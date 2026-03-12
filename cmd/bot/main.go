package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"Polybot/internal/config"
	infraChainlink "Polybot/internal/infra/chainlink"
	infraLogger "Polybot/internal/infra/logger"
)

// Chainlink Data Streams feed IDs (mainnet v3 feeds)
var chainlinkFeeds = map[string]string{
	"BTC": "0x00039d9e45394f473ab1f050a1b963e6b05351e52d71e507509ada0c95ed75b8",
	"XRP": "0x0003c16c6aed42294f5cb4741f6e59ba2d728f0eae2eb9e6d3f555808c59fc45",
	"ETH": "0x000362205e10b3a147d02792eccee483dca6c7b44ecce7012cb8c6e0b68b3ae9",
	"SOL": "0x0003b778d3f6b2ac4991302b89cb313f99a42467d6c9c5f96f57c29c0d2bc24f",
}

func main() {
	logger := infraLogger.New()

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		logger.Info("received shutdown signal")
		cancel()
	}()

	// Initialize Chainlink — only register the feed for our asset
	refStream, err := infraChainlink.NewStream(infraChainlink.StreamConfig{
		ApiKey:    cfg.ChainlinkUserID,
		ApiSecret: cfg.ChainlinkSecret,
		RestURL:   cfg.ChainlinkRestURL,
		WsURL:     cfg.ChainlinkWSURL,
	}, logger)
	if err != nil {
		logger.Error("failed to create chainlink stream", "error", err)
		os.Exit(1)
	}

	asset := strings.ToUpper(cfg.Market)
	feedID, ok := chainlinkFeeds[asset]
	if !ok {
		logger.Error("no chainlink feed for asset", "asset", asset)
		os.Exit(1)
	}
	if err := refStream.RegisterFeedFromString(asset, feedID); err != nil {
		logger.Error("failed to register feed", "asset", asset, "error", err)
		os.Exit(1)
	}

	if cfg.Mode == "debug" || os.Getenv("DEBUG_CHAINLINK") == "1" {
		runChainlinkDebug(ctx, refStream, logger)
		return
	}

	logger.Info("bot config", "market", cfg.Market, "interval", cfg.Interval, "mode", cfg.Mode)

	application := buildApp(cfg, refStream, logger)
	if err := application.Run(ctx); err != nil {
		logger.Error("application error", "error", err)
		os.Exit(1)
	}
}
