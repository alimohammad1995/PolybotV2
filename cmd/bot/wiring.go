package main

import (
	"log/slog"
	"os"
	"strings"
	"time"

	"Polybot/internal/app"
	"Polybot/internal/config"
	infraChainlink "Polybot/internal/infra/chainlink"
	"Polybot/internal/infra/polymarket"
	"Polybot/internal/infra/storage"
	"Polybot/internal/infra/tracker"
	"Polybot/internal/model"
	"Polybot/internal/ports"
	"Polybot/internal/service"
	"Polybot/internal/strategy"
)

func buildApp(cfg *config.Config, refStream *infraChainlink.Stream, logger *slog.Logger) *app.App {
	eventRepo := storage.NewInMemoryEventRepo(logger)
	positionRepo := storage.NewInMemoryPositionRepo()

	registry := service.NewMarketRegistry()
	refAnalytics := service.NewReferenceAnalyticsService(5000, cfg.ResampleIntervalMs)
	positionSvc := service.NewPositionService(positionRepo)

	// Resampler: converts irregular Chainlink ticks to fixed-interval grid
	resampler := service.NewResampler(time.Duration(cfg.ResampleIntervalMs) * time.Millisecond)
	logger.Info("resampler configured", "interval_ms", cfg.ResampleIntervalMs)

	pricingModel := buildPricingModel(cfg, refAnalytics, logger)

	costModel := &fixedCostModel{cost: 0.005}

	signalSvc := service.NewSignalService(
		costModel,
		service.SignalConfig{
			BaseHurdle: cfg.BaseHurdle,
			MaxSizeUSD: cfg.MaxPositionUSDPerMarket,
		},
	)

	// Hedge engine: monitors inventory, buys opposite side to improve guaranteed floor
	hedgeEngine := service.NewHedgeEngine(positionSvc, costModel, service.HedgeConfig{
		HedgeHurdle: cfg.HedgeHurdle,
	})

	// Persistence filter: require edge across N consecutive evaluations
	persistenceFilter := service.NewPersistenceFilter(cfg.PersistenceCount, cfg.HedgePersistenceCount)

	riskSvc := service.NewRiskService(service.RiskConfig{
		MaxPositionUSDPerMarket: cfg.MaxPositionUSDPerMarket,
		MaxTotalExposureUSD:     cfg.MaxTotalExposureUSD,
		NoNewTradeCutoffSecs:    cfg.NoNewTradeCutoffSecs,
		FractionalKelly:         cfg.FractionalKelly,
		MinTradeSizeUSD:         cfg.MinTradeSizeUSD,
		MinTradeShares:          cfg.MinTradeShares,
	})

	clobClient := buildClobClient(cfg, logger)
	execProvider := buildExecutionProvider(cfg, clobClient, registry, logger)
	execSvc := service.NewExecutionService(execProvider)

	runner := strategy.NewStrategyRunner(
		pricingModel,
		signalSvc,
		hedgeEngine,
		persistenceFilter,
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

	// Fill listener: in live mode, subscribes to Polymarket user WS for trade confirmations.
	// In paper mode, fills are simulated immediately — no listener needed.
	var fillListener ports.FillListener
	if cfg.Mode == "live" {
		fillListener = polymarket.NewFillListener(clobClient, registry, positionSvc, logger)
		logger.Info("fill listener enabled (live mode)")
	}

	return &app.App{
		Config: &app.AppConfig{
			BankrollUSD: cfg.BankrollUSD,
			Asset:       asset,
			Mode:        cfg.Mode,
		},
		Registry:       registry,
		RefAnalytics:   refAnalytics,
		Resampler:      resampler,
		PositionSvc:    positionSvc,
		Runner:         runner,
		MarketData:     marketData,
		RefPriceStream: refStream,
		PriceTracker:   tracker.NewPriceTracker(registry, refAnalytics, pricingModel, positionSvc, hedgeEngine, "logs", logger),
		FillListener:   fillListener,
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

	m := model.NewDynamicGaussianModel(0.001, cfg.DefaultModelUncertainty)

	// Load isotonic calibration if configured
	if cfg.CalibrationFile != "" {
		calMap, err := model.NewCalibrationMapFromFile(cfg.CalibrationFile)
		if err != nil {
			logger.Warn("failed to load calibration file, using uncalibrated model",
				"file", cfg.CalibrationFile, "error", err)
		} else {
			m.Calibration = calMap
			logger.Info("loaded isotonic calibration", "file", cfg.CalibrationFile)
		}
	}

	logger.Info("using dynamic gaussian model (Chainlink vol-driven)")
	return m
}
