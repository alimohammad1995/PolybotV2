package strategy

import (
	"context"
	"fmt"
	"log/slog"
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
	Logger            *slog.Logger
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
		r.Logger.Warn("quote sanity check failed",
			"market", market.ID,
			"error", err,
		)
		return nil
	}

	// === SAFETY GUARD: jump rejection ===
	if refState.JumpScore > 6.0 {
		r.Logger.Info("rejecting due to extreme jump",
			"market", market.ID,
			"jump_score", refState.JumpScore,
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
	signal := r.selectSignal(ctx, market, &fv, &quote)
	_ = r.EventRepo.SaveSignal(ctx, signal)

	r.Logger.Info("evaluated",
		"asset", market.Asset,
		"ref", refState.CurrentPrice,
		"beat", market.PriceToBeat,
		"remaining", remaining,
		"regime", refState.Regime,
		"vol1m", refState.RealizedVol1m,
		"p_raw", fv.ProbRaw,
		"p_cal", fv.ProbCalibrated,
		"p_lo", fv.ProbUpLower,
		"p_hi", fv.ProbUpUpper,
		"up_ask", mktState.UpAsk,
		"down_ask", mktState.DownAsk,
		"signal", signal.Side,
		"signal_type", signal.SignalType,
	)

	if signal.Side == domain.SignalNone {
		return nil
	}

	// === PERSISTENCE: require edge across N consecutive evaluations ===
	if r.PersistenceFilter != nil && !r.PersistenceFilter.Check(signal) {
		r.Logger.Debug("signal not persistent enough",
			"market", market.ID,
			"side", signal.Side,
			"type", signal.SignalType,
		)
		return nil
	}

	if !r.RiskSvc.ShouldAllowNewTrade(remaining) {
		r.Logger.Info("blocked by cutoff", "market", market.ID, "remaining", remaining)
		return nil
	}

	return r.executeSignal(ctx, market, &signal, mktState, refState, bankrollUSD)
}

// selectSignal implements the decision priority: hedge > directional > no trade.
func (r *StrategyRunner) selectSignal(
	ctx context.Context,
	market *domain.BinaryMarket,
	fv *domain.FairValue,
	quote *domain.MarketQuote,
) domain.TradeSignal {
	// Priority 1: hedge trade (monetize existing inventory)
	if r.HedgeEngine != nil {
		hedgeSignal, err := r.HedgeEngine.Evaluate(ctx, market.ID, quote)
		if err == nil && hedgeSignal.Side != domain.SignalNone {
			return hedgeSignal
		}
	}

	// Priority 2: directional trade
	inventoryPenalty := r.PositionSvc.GetInventoryPenalty(market.ID)
	dirSignal, err := r.SignalSvc.Generate(ctx, fv, quote, inventoryPenalty)
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
	var positionSide domain.PositionSide

	switch signal.Side {
	case domain.SignalBuyUp, domain.SignalHedgeUp:
		edge = signal.EdgeBuyUp
		if signal.SignalType == "hedge" {
			edge = signal.HedgeEdge
		}
		maxPrice = mktState.UpAsk
		positionSide = domain.PositionUp
	case domain.SignalBuyDown, domain.SignalHedgeDown:
		edge = signal.EdgeBuyDown
		if signal.SignalType == "hedge" {
			edge = signal.HedgeEdge
		}
		maxPrice = mktState.DownAsk
		positionSide = domain.PositionDown
	default:
		return nil
	}

	sizeUSD := r.RiskSvc.ComputeTargetSizeUSD(edge, bankrollUSD, currentExposure, maxPrice)
	if sizeUSD <= 0 {
		return nil
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

	// In paper mode (Filled=true, no OrderID), record position immediately.
	// In live mode, the fill listener updates positions from confirmed WS events.
	if result.Filled && result.OrderID == "" {
		fillPrice := maxPrice
		if result.Price > 0 {
			fillPrice = result.Price
		}
		return r.PositionSvc.RecordPosition(ctx, domain.Position{
			MarketID:         market.ID,
			Side:             positionSide,
			AvgEntryPrice:    fillPrice,
			Quantity:         sizeUSD / fillPrice,
			NotionalUSD:      sizeUSD,
			OpenedAt:         time.Now(),
			HoldToSettlement: true,
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
