package strategy

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"time"

	"Polybot/internal/domain"
	"Polybot/internal/ports"
	"Polybot/internal/service"
)

// FreshnessConfig defines staleness thresholds for data sources.
type FreshnessConfig struct {
	MaxReferenceAge  time.Duration
	MaxQuoteAge      time.Duration
	MaxAllowedSpread float64
	MinTickCount     int
	HedgeAfterPct    float64 // fraction of market elapsed before hedging (0.70 = last 30%)
}

type StrategyRunner struct {
	PricingModel      service.PricingModel
	SignalSvc         *service.SignalService
	HedgeEngine       *service.HedgeEngine
	PersistenceFilter *service.PersistenceFilter
	RiskSvc           *service.RiskService
	ExecSvc           *service.ExecutionService
	PositionSvc       *service.PositionService
	EventRepo         ports.EventRepository
	Clock             ports.Clock
	Freshness         FreshnessConfig
	ImbalanceCfg      service.ImbalancePenaltyConfig
	Logger            *slog.Logger

	lastTradeTime time.Time // cooldown: prevent rapid-fire trades on buffered events
}

func NewStrategyRunner(
	pricing service.PricingModel,
	signal *service.SignalService,
	hedge *service.HedgeEngine,
	persistence *service.PersistenceFilter,
	risk *service.RiskService,
	exec *service.ExecutionService,
	position *service.PositionService,
	eventRepo ports.EventRepository,
	clock ports.Clock,
	freshness FreshnessConfig,
	imbalanceCfg service.ImbalancePenaltyConfig,
	logger *slog.Logger,
) *StrategyRunner {
	return &StrategyRunner{
		PricingModel:      pricing,
		SignalSvc:         signal,
		HedgeEngine:       hedge,
		PersistenceFilter: persistence,
		RiskSvc:           risk,
		ExecSvc:           exec,
		PositionSvc:       position,
		EventRepo:         eventRepo,
		Clock:             clock,
		Freshness:         freshness,
		ImbalanceCfg:      imbalanceCfg,
		Logger:            logger,
	}
}

// EvaluateMarket runs the full pricing->signal->risk->execution pipeline.
//
// Decision priority: hedge trade > directional trade > no trade
//
// refState comes from Chainlink (truth process).
// mktState comes from Polymarket (execution venue).
// These are NEVER mixed -- fair probability uses only Chainlink data.
func (r *StrategyRunner) EvaluateMarket(
	ctx context.Context,
	market *domain.BinaryMarket,
	refState *domain.ReferenceState,
	mktState *domain.MarketState,
	bankrollUSD float64,
) error {
	now := r.Clock.Now()

	// === SAFETY GUARD: trade cooldown (2s) to prevent rapid-fire on buffered events ===
	if !r.lastTradeTime.IsZero() && now.Sub(r.lastTradeTime) < 2*time.Second {
		return nil
	}

	// === SAFETY GUARD: freshness checks ===
	if reason := r.checkFreshness(now, refState, mktState); reason != "" {
		r.Logger.Debug("skipping stale market",
			"market", market.ID,
			"reason", reason,
		)
		return nil
	}

	// === SAFETY GUARD: spread check ===
	if mktState.Spread > r.Freshness.MaxAllowedSpread {
		r.Logger.Debug("spread too wide",
			"market", market.ID,
			"spread", mktState.Spread,
			"max", r.Freshness.MaxAllowedSpread,
		)
		return nil
	}

	// === SAFETY GUARD: quote sanity ===
	if err := r.validateQuotes(mktState); err != nil {
		r.Logger.Debug("quote sanity check failed",
			"market", market.ID,
			"error", err,
		)
		return nil
	}

	// === SAFETY GUARD: jump rejection ===
	if refState.JumpScore > 6.0 {
		r.Logger.Debug("rejecting due to extreme jump",
			"market", market.ID,
			"jump_score", refState.JumpScore,
		)
		return nil
	}

	// === SAFETY GUARD: warm-up — require N ticks for reliable vol estimates ===
	if r.Freshness.MinTickCount > 0 && refState.TickCount < r.Freshness.MinTickCount {
		r.Logger.Debug("waiting for warm-up ticks",
			"market", market.ID,
			"ticks", refState.TickCount,
			"required", r.Freshness.MinTickCount,
		)
		return nil
	}

	remaining := market.EndTime.Sub(now).Seconds()
	if remaining < 0 {
		remaining = 0
	}

	// === PRICING: uses ONLY Chainlink-derived data ===
	pricingInput := domain.PricingInput{
		CurrentPrice:     refState.CurrentPrice,
		PriceToBeat:      market.PriceToBeat,
		RemainingSeconds: remaining,
		RealizedVol1m:    refState.RealizedVol1m,
		RealizedVol5m:    refState.RealizedVol5m,
		JumpScore:        refState.JumpScore,
		Regime:           refState.Regime,
		DriftPerSec:      refState.DriftPerSec,
		DriftTicks:       refState.DriftTicks,
	}

	fv, err := r.PricingModel.FairProbUp(ctx, pricingInput)
	if err != nil {
		return fmt.Errorf("pricing: %w", err)
	}
	fv.MarketID = market.ID

	_ = r.EventRepo.SaveFairValue(ctx, fv)

	// Convert MarketState to MarketQuote for signal/hedge engines
	quote := domain.MarketQuote{
		MarketID:  market.ID,
		Up:        domain.SideQuote{Bid: mktState.UpBid, Ask: mktState.UpAsk},
		Down:      domain.SideQuote{Bid: mktState.DownBid, Ask: mktState.DownAsk},
		Timestamp: mktState.Timestamp,
	}
	_ = r.EventRepo.SaveQuote(ctx, quote)

	// === SELECTOR: hedge > directional > no trade ===
	signal := r.selectSignal(ctx, market, &fv, &quote, remaining)
	_ = r.EventRepo.SaveSignal(ctx, signal)

	if signal.Side == domain.SignalNone {
		return nil
	}

	// === PERSISTENCE: require edge across N consecutive evaluations ===
	if r.PersistenceFilter != nil && !r.PersistenceFilter.Check(signal) {
		return nil
	}

	if !r.RiskSvc.ShouldAllowNewTrade(remaining) {
		r.Logger.Debug("blocked by cutoff", "market", market.ID, "remaining", remaining)
		return nil
	}

	return r.executeSignal(ctx, market, &signal, mktState, refState, bankrollUSD)
}

// selectSignal implements the decision priority: hedge > directional > no trade.
// Hedging is time-gated: only allowed after HedgeAfterPct of the market has elapsed.
func (r *StrategyRunner) selectSignal(
	ctx context.Context,
	market *domain.BinaryMarket,
	fv *domain.FairValue,
	quote *domain.MarketQuote,
	remainingSeconds float64,
) domain.TradeSignal {
	// Priority 1: hedge trade (time-gated — hold directional exposure early)
	if r.HedgeEngine != nil && r.Freshness.HedgeAfterPct > 0 {
		totalDuration := market.EndTime.Sub(market.StartTime).Seconds()
		elapsedPct := 1.0 - remainingSeconds/totalDuration
		if totalDuration > 0 && elapsedPct >= r.Freshness.HedgeAfterPct {
			hedgeSignal, err := r.HedgeEngine.Evaluate(ctx, market.ID, quote)
			if err == nil && hedgeSignal.Side != domain.SignalNone {
				return hedgeSignal
			}
		}
	} else if r.HedgeEngine != nil {
		hedgeSignal, err := r.HedgeEngine.Evaluate(ctx, market.ID, quote)
		if err == nil && hedgeSignal.Side != domain.SignalNone {
			return hedgeSignal
		}
	}

	// Priority 2: directional trade
	penaltyUp, penaltyDown := r.PositionSvc.GetInventoryPenalties(market.ID, r.ImbalanceCfg)
	dirSignal, err := r.SignalSvc.Generate(ctx, fv, quote, penaltyUp, penaltyDown)
	if err != nil {
		r.Logger.Warn("signal generation error", "market", market.ID, "error", err)
		return domain.TradeSignal{
			MarketID:   market.ID,
			Side:       domain.SignalNone,
			SignalType: "directional",
			Timestamp:  time.Now(),
		}
	}
	dirSignal.SignalType = "directional"
	return dirSignal
}

func (r *StrategyRunner) executeSignal(
	ctx context.Context,
	market *domain.BinaryMarket,
	signal *domain.TradeSignal,
	mktState *domain.MarketState,
	refState *domain.ReferenceState,
	bankrollUSD float64,
) error {
	currentExposure := r.PositionSvc.GetExposureForMarket(market.ID)

	var edge float64
	var maxPrice float64
	var buyingUp bool

	switch signal.Side {
	case domain.SignalBuyUp, domain.SignalHedgeUp:
		edge = signal.EdgeBuyUp
		if signal.SignalType == "hedge" {
			edge = signal.HedgeEdge
		}
		maxPrice = mktState.UpAsk
		buyingUp = true
	case domain.SignalBuyDown, domain.SignalHedgeDown:
		edge = signal.EdgeBuyDown
		if signal.SignalType == "hedge" {
			edge = signal.HedgeEdge
		}
		maxPrice = mktState.DownAsk
		buyingUp = false
	default:
		return nil
	}

	// Compute imbalance ratio for Kelly decay
	upQty, downQty, upCost, downCost := r.PositionSvc.GetInventory(market.ID)
	total := upQty + downQty
	imbalance := math.Abs(upQty - downQty)
	imbalanceRatio := 0.0
	if total > 0 {
		imbalanceRatio = imbalance / total
	}
	increasingImbalance := (buyingUp && upQty > downQty) || (!buyingUp && downQty > upQty)

	sizeUSD := r.RiskSvc.ComputeTargetSizeUSD(edge, bankrollUSD, currentExposure, maxPrice, imbalanceRatio, increasingImbalance)
	if sizeUSD <= 0 {
		return nil
	}

	// Pre-trade risk gates: imbalance, floor, worst-case loss
	if maxPrice > 0 {
		proposedShares := math.Floor(sizeUSD / maxPrice)
		if reason := r.RiskSvc.PreTradeCheck(buyingUp, proposedShares, maxPrice, upQty, downQty, upCost, downCost); reason != "" {
			r.Logger.Debug("trade blocked by risk gate",
				"market", market.ID,
				"side", signal.Side,
				"reason", reason,
			)
			return nil
		}
	}

	// Hedge trades: cap at the number of shares needed to balance inventory.
	// The hedge edge is per-share ΔG, but only valid up to |Nu - Nd| shares.
	if signal.SignalType == "hedge" && maxPrice > 0 {
		upQty, downQty, _, _ := r.PositionSvc.GetInventory(market.ID)
		var balanceShares float64
		switch signal.Side {
		case domain.SignalHedgeUp:
			balanceShares = downQty - upQty
		case domain.SignalHedgeDown:
			balanceShares = upQty - downQty
		}
		if balanceShares <= 0 {
			return nil
		}
		maxHedgeUSD := balanceShares * maxPrice
		if sizeUSD > maxHedgeUSD {
			sizeUSD = math.Floor(balanceShares) * maxPrice
		}
		if sizeUSD <= 0 {
			return nil
		}
	}

	r.Logger.Info("executing",
		"market", market.ID,
		"side", signal.Side,
		"type", signal.SignalType,
		"max_price", maxPrice,
		"size_usd", sizeUSD,
		"edge", edge,
		"regime", refState.Regime,
	)

	result, err := r.ExecSvc.Execute(ctx, domain.ExecutionRequest{
		MarketID: market.ID,
		Side:     signal.Side,
		MaxPrice: maxPrice,
		SizeUSD:  sizeUSD,
		Reason:   signal.Reason,
	})
	if err != nil {
		return fmt.Errorf("execution: %w", err)
	}

	r.lastTradeTime = time.Now()

	// In paper mode (Filled=true, no OrderID), record position immediately.
	// In live mode, the fill listener updates positions from confirmed WS events.
	if result.Filled && result.OrderID == "" {
		fillPrice := maxPrice
		if result.Price > 0 {
			fillPrice = result.Price
		}
		return r.PositionSvc.AccumulateFill(ctx, domain.Fill{
			MarketID:  market.ID,
			Side:      signal.Side,
			Price:     fillPrice,
			SizeUSD:   sizeUSD,
			Timestamp: time.Now(),
		})
	}

	r.Logger.Info("order submitted, awaiting fill confirmation",
		"market", market.ID,
		"order_id", result.OrderID,
		"filled", result.Filled,
	)
	return nil
}

// OnMarketUpdate is a backward-compatible wrapper that constructs
// ReferenceState and MarketState from simple arguments.
func (r *StrategyRunner) OnMarketUpdate(
	ctx context.Context,
	market domain.BinaryMarket,
	quote domain.MarketQuote,
	refPrice float64,
	bankrollUSD float64,
) error {
	refState := domain.ReferenceState{
		Asset:        market.Asset,
		CurrentPrice: refPrice,
		LastUpdate:   r.Clock.Now(),
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
	return r.EvaluateMarket(ctx, &market, &refState, &mktState, bankrollUSD)
}

func (r *StrategyRunner) checkFreshness(now time.Time, ref *domain.ReferenceState, mkt *domain.MarketState) string {
	if r.Freshness.MaxReferenceAge > 0 && !ref.LastUpdate.IsZero() {
		if now.Sub(ref.LastUpdate) > r.Freshness.MaxReferenceAge {
			return "stale_chainlink"
		}
	}
	if r.Freshness.MaxQuoteAge > 0 && !mkt.Timestamp.IsZero() {
		if now.Sub(mkt.Timestamp) > r.Freshness.MaxQuoteAge {
			return "stale_polymarket_quote"
		}
	}
	return ""
}

func (r *StrategyRunner) validateQuotes(mkt *domain.MarketState) error {
	if mkt.UpAsk <= 0 || mkt.UpAsk > 1.0 {
		return fmt.Errorf("invalid up ask: %f", mkt.UpAsk)
	}
	if mkt.DownAsk <= 0 || mkt.DownAsk > 1.0 {
		return fmt.Errorf("invalid down ask: %f", mkt.DownAsk)
	}
	if mkt.UpBid < 0 || mkt.UpBid >= mkt.UpAsk {
		return fmt.Errorf("invalid up bid/ask: bid=%f ask=%f", mkt.UpBid, mkt.UpAsk)
	}
	if mkt.DownBid < 0 || mkt.DownBid >= mkt.DownAsk {
		return fmt.Errorf("invalid down bid/ask: bid=%f ask=%f", mkt.DownBid, mkt.DownAsk)
	}
	return nil
}
