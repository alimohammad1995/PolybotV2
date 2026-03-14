package service

import (
	"context"
	"time"

	"Polybot/internal/domain"
	"Polybot/internal/ports"
)

type SignalConfig struct {
	BaseHurdle float64
	MaxSizeUSD float64
}

type SignalService struct {
	CostModel ports.CostModel
	Config    SignalConfig
}

func NewSignalService(costModel ports.CostModel, cfg SignalConfig) *SignalService {
	return &SignalService{
		CostModel: costModel,
		Config:    cfg,
	}
}

func (s *SignalService) Generate(
	ctx context.Context,
	fv *domain.FairValue,
	quote *domain.MarketQuote,
	penaltyBuyUp, penaltyBuyDown float64,
) (domain.TradeSignal, error) {
	cost, err := s.CostModel.EstimateAllInCost(ctx, quote.MarketID)
	if err != nil {
		return domain.TradeSignal{}, err
	}

	hurdleUp := s.Config.BaseHurdle + penaltyBuyUp
	hurdleDown := s.Config.BaseHurdle + penaltyBuyDown

	edgeBuyUp := fv.ProbUpLower - quote.Up.Ask - cost
	edgeBuyDown := (1.0 - fv.ProbUpUpper) - quote.Down.Ask - cost

	signal := domain.TradeSignal{
		MarketID:    quote.MarketID,
		EdgeBuyUp:   edgeBuyUp,
		EdgeBuyDown: edgeBuyDown,
		Timestamp:   time.Now(),
	}

	// Compare each side's edge against its own hurdle
	upPass := edgeBuyUp > hurdleUp
	downPass := edgeBuyDown > hurdleDown

	switch {
	case upPass && downPass:
		// Both pass — pick the one with more edge above its hurdle
		if (edgeBuyUp - hurdleUp) > (edgeBuyDown - hurdleDown) {
			signal.Side = domain.SignalBuyUp
			signal.EffectiveHurdle = hurdleUp
			signal.Reason = "buy_up_edge_exceeds_hurdle"
		} else {
			signal.Side = domain.SignalBuyDown
			signal.EffectiveHurdle = hurdleDown
			signal.Reason = "buy_down_edge_exceeds_hurdle"
		}
	case upPass:
		signal.Side = domain.SignalBuyUp
		signal.EffectiveHurdle = hurdleUp
		signal.Reason = "buy_up_edge_exceeds_hurdle"
	case downPass:
		signal.Side = domain.SignalBuyDown
		signal.EffectiveHurdle = hurdleDown
		signal.Reason = "buy_down_edge_exceeds_hurdle"
	default:
		signal.Side = domain.SignalNone
		signal.EffectiveHurdle = hurdleUp // arbitrary — no trade
		signal.Reason = "no_trade"
	}

	return signal, nil
}
