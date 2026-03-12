package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	infraChainlink "Polybot/internal/infra/chainlink"
)

func runChainlinkDebug(ctx context.Context, refStream *infraChainlink.Stream, logger *slog.Logger) {
	logger.Info("=== CHAINLINK DEBUG MODE ===")

	if err := refStream.DiscoverFeeds(ctx); err != nil {
		logger.Warn("feed discovery failed (may not be supported)", "error", err)
	}

	// Fetch and stream all registered feeds
	for _, asset := range refStream.RegisteredAssets() {
		snap, err := refStream.FetchLatestReport(ctx, asset)
		if err != nil {
			logger.Error("REST fetch failed", "asset", asset, "error", err)
		} else {
			logger.Info("REST price",
				"asset", snap.Asset,
				"price", fmt.Sprintf("%.2f", snap.Price),
				"timestamp", snap.Timestamp.Format(time.RFC3339),
			)
		}
	}

	for _, asset := range refStream.RegisteredAssets() {
		asset := asset
		ch, err := refStream.SubscribePrices(ctx, asset)
		if err != nil {
			logger.Error("failed to subscribe", "asset", asset, "error", err)
			continue
		}
		go func() {
			for snap := range ch {
				logger.Info("WS price",
					"asset", snap.Asset,
					"price", fmt.Sprintf("%.6f", snap.Price),
					"timestamp", snap.Timestamp.Format(time.RFC3339Nano),
				)
			}
		}()
	}

	logger.Info("streaming prices... press Ctrl+C to stop")
	<-ctx.Done()
	logger.Info("debug mode shutdown")
}
