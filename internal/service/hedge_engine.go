package service

import (
	"context"
	"math"
	"time"

	"Polybot/internal/domain"
	"Polybot/internal/ports"
)

// HedgeConfig holds parameters for the hedge engine.
type HedgeConfig struct {
	HedgeHurdle float64 // minimum floor improvement to trigger hedge trade
}

// HedgeEngine monitors inventory imbalance and recommends buying the opposite
// side to lock in guaranteed floor profit.
//
// Floor formula: G = min(N_up, N_down) - C_up - C_down
// A hedge trade is recommended if buying the opposite side improves G by more than HedgeHurdle.
type HedgeEngine struct {
	positionSvc *PositionService
	costModel   ports.CostModel
	config      HedgeConfig
}

func NewHedgeEngine(positionSvc *PositionService, costModel ports.CostModel, config HedgeConfig) *HedgeEngine {
	return &HedgeEngine{
		positionSvc: positionSvc,
		costModel:   costModel,
		config:      config,
	}
}

// Evaluate checks whether a hedge trade would improve the guaranteed floor.
// Returns a hedge signal if ΔG > hurdle, otherwise SignalNone.
func (h *HedgeEngine) Evaluate(
	ctx context.Context,
	marketID domain.MarketID,
	quote *domain.MarketQuote,
) (domain.TradeSignal, error) {
	upQty, downQty, upCost, downCost := h.positionSvc.GetInventory(marketID)

	// No position — nothing to hedge
	if upQty == 0 && downQty == 0 {
		return domain.TradeSignal{
			MarketID:   marketID,
			Side:       domain.SignalNone,
			SignalType: "hedge",
			Timestamp:  time.Now(),
		}, nil
	}

	cost, err := h.costModel.EstimateAllInCost(ctx, marketID)
	if err != nil {
		cost = 0.005 // fallback
	}

	// Current guaranteed floor
	currentFloor := math.Min(upQty, downQty) - upCost - downCost

	// Floor improvement if we buy 1 unit of DOWN at ask
	deltaGDown := 0.0
	if quote.Down.Ask > 0 && quote.Down.Ask < 1.0 {
		newFloor := math.Min(upQty, downQty+1) - upCost - downCost - quote.Down.Ask - cost
		deltaGDown = newFloor - currentFloor
	}

	// Floor improvement if we buy 1 unit of UP at ask
	deltaGUp := 0.0
	if quote.Up.Ask > 0 && quote.Up.Ask < 1.0 {
		newFloor := math.Min(upQty+1, downQty) - upCost - downCost - quote.Up.Ask - cost
		deltaGUp = newFloor - currentFloor
	}

	signal := domain.TradeSignal{
		MarketID:        marketID,
		SignalType:      "hedge",
		GuaranteedFloor: currentFloor,
		Timestamp:       time.Now(),
	}

	// Pick the best hedge trade
	switch {
	case deltaGDown > deltaGUp && deltaGDown > h.config.HedgeHurdle:
		signal.Side = domain.SignalHedgeDown
		signal.HedgeEdge = deltaGDown
		signal.Reason = "hedge_buy_down_floor_improvement"
	case deltaGUp > h.config.HedgeHurdle:
		signal.Side = domain.SignalHedgeUp
		signal.HedgeEdge = deltaGUp
		signal.Reason = "hedge_buy_up_floor_improvement"
	default:
		signal.Side = domain.SignalNone
		signal.Reason = "no_hedge_opportunity"
	}

	return signal, nil
}

// ComputeFloor computes the current guaranteed floor for a market position.
func (h *HedgeEngine) ComputeFloor(marketID domain.MarketID) float64 {
	upQty, downQty, upCost, downCost := h.positionSvc.GetInventory(marketID)
	return math.Min(upQty, downQty) - upCost - downCost
}

// ComputeHedgeEdges computes the floor improvement for buying each side.
// Useful for logging without generating a full signal.
func (h *HedgeEngine) ComputeHedgeEdges(
	ctx context.Context,
	marketID domain.MarketID,
	quote *domain.MarketQuote,
) (hedgeEdgeBuyUp, hedgeEdgeBuyDown, floor float64) {
	upQty, downQty, upCost, downCost := h.positionSvc.GetInventory(marketID)
	floor = math.Min(upQty, downQty) - upCost - downCost

	cost, err := h.costModel.EstimateAllInCost(ctx, marketID)
	if err != nil {
		cost = 0.005
	}

	if quote.Down.Ask > 0 && quote.Down.Ask < 1.0 {
		newFloor := math.Min(upQty, downQty+1) - upCost - downCost - quote.Down.Ask - cost
		hedgeEdgeBuyDown = newFloor - floor
	}
	if quote.Up.Ask > 0 && quote.Up.Ask < 1.0 {
		newFloor := math.Min(upQty+1, downQty) - upCost - downCost - quote.Up.Ask - cost
		hedgeEdgeBuyUp = newFloor - floor
	}
	return
}
