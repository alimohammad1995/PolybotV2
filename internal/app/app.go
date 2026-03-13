package app

import (
	"context"
	"log/slog"
	"time"

	"Polybot/internal/domain"
	"Polybot/internal/infra/tracker"
	"Polybot/internal/ports"
	"Polybot/internal/service"
	"Polybot/internal/strategy"
)

type App struct {
	Config         *AppConfig
	Registry       *service.MarketRegistry
	RefAnalytics   *service.ReferenceAnalyticsService
	Resampler      *service.Resampler
	PositionSvc    *service.PositionService
	Runner         *strategy.StrategyRunner
	MarketData     ports.MarketDataProvider
	RefPriceStream ports.ReferencePriceProvider
	PriceTracker   *tracker.PriceTracker
	FillListener   ports.FillListener // nil in paper mode
	Logger         *slog.Logger
}

type AppConfig struct {
	BankrollUSD float64
	Asset       string // single asset (BTC, ETH, etc.)
	Mode        string
}

func (a *App) Run(ctx context.Context) error {
	a.Logger.Info("starting polybot",
		"mode", a.Config.Mode,
		"asset", a.Config.Asset,
		"bankroll", a.Config.BankrollUSD,
	)

	// Bootstrap: load existing positions from Polymarket API (live mode only).
	// This must happen after market lifecycle resolves the first market so that
	// token IDs are in the registry. We start it after a short delay below.
	if a.FillListener != nil {
		go func() {
			// Wait briefly for market lifecycle to populate the registry
			time.Sleep(8 * time.Second)
			if err := a.FillListener.LoadPositionsFromAPI(ctx); err != nil {
				a.Logger.Warn("failed to bootstrap positions from API", "error", err)
			}
		}()
	}

	repriceCh := make(chan domain.RepriceEvent, 1024)

	// Stream Chainlink reference prices for our asset
	go a.streamChainlinkTicks(ctx, a.Config.Asset, repriceCh)

	// Market lifecycle: resolve market, stream quotes via WS, roll on expiry
	go a.marketLifecycle(ctx, repriceCh)

	// Stream Polymarket quotes via WebSocket
	go a.streamPolymarketQuotes(ctx, repriceCh)

	// Periodic reprice ticker (catch time decay)
	go a.timeDecayTicker(ctx, repriceCh)

	// Price tracker: log model vs market prices every second
	go a.PriceTracker.Run(ctx)

	// Fill listener: subscribe to Polymarket user WS for trade confirmations (live mode)
	if a.FillListener != nil {
		go a.FillListener.Run(ctx)
	}

	// Single reprice loop — one market, one asset, CPU-bound evaluation
	a.repriceLoop(ctx, repriceCh)

	a.Logger.Info("shutting down")
	return nil
}

// marketLifecycle resolves the current market, polls quotes, and rolls to the next
// market when the current one expires.
func (a *App) marketLifecycle(ctx context.Context, repriceCh chan<- domain.RepriceEvent) {
	for {
		// Resolve current market
		markets, err := a.MarketData.GetActiveMarkets(ctx)
		if err != nil || len(markets) == 0 {
			a.Logger.Warn("waiting for market to become available", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				continue
			}
		}

		market := markets[0]

		// Set PriceToBeat from Chainlink at market start time
		snap, err := a.RefPriceStream.GetPriceAtTime(ctx, market.Asset, market.StartTime)
		if err != nil {
			a.Logger.Warn("failed to fetch price at market start, using current",
				"asset", market.Asset, "start_time", market.StartTime, "error", err)
			// Fallback: use current price from analytics
			if refState, ok := a.RefAnalytics.GetState(market.Asset); ok && refState.CurrentPrice > 0 {
				market.PriceToBeat = refState.CurrentPrice
			}
		} else {
			market.PriceToBeat = snap.Price
			a.Logger.Info("price to beat set from chainlink",
				"asset", market.Asset,
				"start_time", market.StartTime.Format(time.RFC3339),
				"price_to_beat", snap.Price,
			)
		}

		a.Registry.SetMarket(market)
		a.Logger.Info("active market",
			"id", market.ID,
			"slug", market.Slug,
			"asset", market.Asset,
			"price_to_beat", market.PriceToBeat,
			"end_time", market.EndTime.Format(time.RFC3339),
			"up_token", market.UpTokenID,
			"down_token", market.DownTokenID,
		)

		// Wait until market expires or context is cancelled
		a.waitForExpiry(ctx, market)

		// Clean up expired market
		a.Registry.RemoveMarket(market.ID)
		a.Logger.Info("market expired, rolling to next", "expired_slug", market.Slug)
	}
}

func (a *App) waitForExpiry(ctx context.Context, market domain.BinaryMarket) {
	remaining := time.Until(market.EndTime)
	if remaining <= 0 {
		return
	}
	timer := time.NewTimer(remaining)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// streamPolymarketQuotes subscribes to the Polymarket WebSocket and feeds quotes into repriceCh.
func (a *App) streamPolymarketQuotes(ctx context.Context, repriceCh chan<- domain.RepriceEvent) {
	quoteCh, err := a.MarketData.SubscribeQuotes(ctx)
	if err != nil {
		a.Logger.Error("failed to subscribe to polymarket quotes", "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case quote, ok := <-quoteCh:
			if !ok {
				return
			}
			a.Registry.SetQuote(quote)
			select {
			case repriceCh <- domain.RepriceEvent{MarketID: quote.MarketID, Reason: "polymarket_ws"}:
			default:
			}
		}
	}
}

// streamChainlinkTicks processes Chainlink price feed for our asset.
// Raw ticks are fed through the Resampler which emits fixed-interval ticks
// to RefAnalytics for cleaner vol computation.
func (a *App) streamChainlinkTicks(ctx context.Context, asset string, repriceCh chan<- domain.RepriceEvent) {
	// Register resampler subscriber: resampled ticks → RefAnalytics + reprice events
	if a.Resampler != nil {
		a.Resampler.Subscribe(func(tick domain.ResampledTick) {
			a.RefAnalytics.OnResampledTick(tick)
			for _, marketID := range a.Registry.ListMarketIDsForAsset(tick.Asset) {
				select {
				case repriceCh <- domain.RepriceEvent{MarketID: marketID, Reason: "chainlink_tick"}:
				default:
				}
			}
		})
	}

	ch, err := a.RefPriceStream.SubscribePrices(ctx, asset)
	if err != nil {
		a.Logger.Error("failed to subscribe to chainlink", "asset", asset, "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case snap, ok := <-ch:
			if !ok {
				return
			}

			tick := domain.ChainlinkTick{
				Asset:     snap.Asset,
				Price:     snap.Price,
				Timestamp: snap.Timestamp,
			}

			if a.Resampler != nil {
				// Feed through resampler — it will emit to RefAnalytics on grid boundaries
				a.Resampler.OnRawTick(tick)
			} else {
				// No resampler — direct feed (backward compat)
				a.RefAnalytics.OnTick(tick)
				for _, marketID := range a.Registry.ListMarketIDsForAsset(asset) {
					select {
					case repriceCh <- domain.RepriceEvent{MarketID: marketID, Reason: "chainlink_tick"}:
					default:
					}
				}
			}
		}
	}
}

func (a *App) timeDecayTicker(ctx context.Context, repriceCh chan<- domain.RepriceEvent) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, m := range a.Registry.ListMarkets() {
				select {
				case repriceCh <- domain.RepriceEvent{MarketID: m.ID, Reason: "time_decay"}:
				default:
				}
			}
		}
	}
}

func (a *App) repriceLoop(ctx context.Context, repriceCh <-chan domain.RepriceEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-repriceCh:
			if !ok {
				return
			}

			market, ok := a.Registry.GetMarket(evt.MarketID)
			if !ok {
				continue
			}

			refState, ok := a.RefAnalytics.GetState(market.Asset)
			if !ok {
				continue
			}

			// Late-bind PriceToBeat if it wasn't available at market creation
			if market.PriceToBeat == 0 && refState.CurrentPrice > 0 {
				market.PriceToBeat = refState.CurrentPrice
				a.Registry.SetMarket(market)
				a.Logger.Info("price to beat set (late bind)",
					"asset", market.Asset,
					"price_to_beat", refState.CurrentPrice,
				)
			}

			quote, ok := a.Registry.GetQuote(evt.MarketID)
			if !ok {
				continue
			}

			mktState := domain.MarketState{
				MarketID:    market.ID,
				PriceToBeat: market.PriceToBeat,
				UpBid:       quote.Up.Bid,
				UpAsk:       quote.Up.Ask,
				DownBid:     quote.Down.Bid,
				DownAsk:     quote.Down.Ask,
				Spread:      (quote.Up.Ask - quote.Up.Bid + quote.Down.Ask - quote.Down.Bid) / 2.0,
				Timestamp:   quote.Timestamp,
			}

			err := a.Runner.EvaluateMarket(ctx, &market, &refState, &mktState, a.Config.BankrollUSD)
			if err != nil {
				a.Logger.Error("evaluation error",
					"market", evt.MarketID,
					"reason", evt.Reason,
					"error", err,
				)
			}
		}
	}
}
