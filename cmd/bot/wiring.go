package main

import (
	"log/slog"
	"os"
	"strings"

	"Polybot/internal/app"
	"Polybot/internal/config"
	infraChainlink "Polybot/internal/infra/chainlink"
	"Polybot/internal/infra/polymarket"
	"Polybot/internal/infra/storage"
	"Polybot/internal/model"
	"Polybot/internal/ports"
	"Polybot/internal/service"
	"Polybot/internal/strategy"
)

func buildApp(cfg *config.Config, refStream *infraChainlink.Stream, logger *slog.Logger) *app.App {
	eventRepo := storage.NewInMemoryEventRepo(logger)
	positionRepo := storage.NewInMemoryPositionRepo()

	registry := service.NewMarketRegistry()
	refAnalytics := service.NewReferenceAnalyticsService(5000)
	positionSvc := service.NewPositionService(positionRepo)

	pricingModel := buildPricingModel(cfg, refAnalytics, logger)

	signalSvc := service.NewSignalService(
		&fixedCostModel{cost: 0.005},
		service.SignalConfig{
			BaseHurdle: cfg.BaseHurdle,
			MaxSizeUSD: cfg.MaxPositionUSDPerMarket,
		},
	)

	riskSvc := service.NewRiskService(service.RiskConfig{
		MaxPositionUSDPerMarket: cfg.MaxPositionUSDPerMarket,
		MaxTotalExposureUSD:     cfg.MaxTotalExposureUSD,
		NoNewTradeCutoffSecs:    cfg.NoNewTradeCutoffSecs,
		FractionalKelly:         cfg.FractionalKelly,
		MinTradeSizeUSD:         cfg.MinTradeSizeUSD,
	})

	clobClient := buildClobClient(cfg, logger)
	execProvider := buildExecutionProvider(cfg, clobClient, registry, logger)
	execSvc := service.NewExecutionService(execProvider)

	runner := strategy.NewStrategyRunner(
		pricingModel,
		signalSvc,
		riskSvc,
		execSvc,
		positionSvc,
		eventRepo,
		ports.SystemClock{},
		strategy.FreshnessConfig{
			MaxReferenceAge:  cfg.MaxReferenceAge,
			MaxQuoteAge:      cfg.MaxQuoteAge,
			MaxAllowedSpread: cfg.MaxAllowedSpread,
		},
		logger,
	)

	asset := strings.ToUpper(cfg.Market)
	marketData := polymarket.NewMarketProvider(clobClient, cfg.Market, cfg.Interval, logger)

	return &app.App{
		Config: &app.AppConfig{
			BankrollUSD: cfg.BankrollUSD,
			WorkerCount: cfg.WorkerCount,
			Asset:       asset,
			Mode:        cfg.Mode,
		},
		Registry:       registry,
		RefAnalytics:   refAnalytics,
		PositionSvc:    positionSvc,
		Runner:         runner,
		MarketData:     marketData,
		RefPriceStream: refStream,
		Logger:         logger,
	}
}

func buildClobClient(cfg *config.Config, logger *slog.Logger) *polymarket.ClobClient {
	client, err := polymarket.NewClobClient(cfg.PrivateKey, polymarket.SignatureEOA, cfg.FunderAddress)
	if err != nil {
		logger.Error("failed to create polymarket client", "error", err)
		os.Exit(1)
	}
	creds, err := client.CreateOrDeriveAPICreds(0)
	if err != nil {
		logger.Error("failed to derive polymarket API creds", "error", err)
		os.Exit(1)
	}
	client.SetAPICreds(creds)
	logger.Info("polymarket client ready", "address", client.Address())
	return client
}

func buildExecutionProvider(cfg *config.Config, client *polymarket.ClobClient, registry *service.MarketRegistry, logger *slog.Logger) ports.ExecutionProvider {
	if cfg.Mode == "live" {
		logger.Info("live mode — real execution enabled")
		return polymarket.NewExecutionProvider(client, registry, logger)
	}
	logger.Info("paper mode — simulated execution")
	return &paperExecutionProvider{logger: logger}
}

func buildPricingModel(cfg *config.Config, refAnalytics *service.ReferenceAnalyticsService, logger *slog.Logger) service.PricingModel {
	if cfg.ModelParamsFile != "" {
		source, err := model.NewFileBasedMixtureParamSource(cfg.ModelParamsFile)
		if err != nil {
			logger.Warn("failed to load model params, falling back to dynamic gaussian",
				"file", cfg.ModelParamsFile, "error", err)
		} else {
			logger.Info("loaded calibrated mixture params", "file", cfg.ModelParamsFile)
			return model.NewMixtureModel("", source)
		}
	}

	logger.Info("using dynamic gaussian model (Chainlink vol-driven)")
	return model.NewDynamicGaussianModel(0.001, cfg.DefaultModelUncertainty)
}
