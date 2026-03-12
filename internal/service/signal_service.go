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
	inventoryPenalty float64,
) (domain.TradeSignal, error) {
	cost, err := s.CostModel.EstimateAllInCost(ctx, quote.MarketID)
	if err != nil {
		return domain.TradeSignal{}, err
	}

	hurdle := s.Config.BaseHurdle + inventoryPenalty

	edgeBuyUp := fv.ProbUpLower - quote.Up.Ask - cost
	edgeBuyDown := (1.0 - fv.ProbUpUpper) - quote.Down.Ask - cost

	signal := domain.TradeSignal{
		MarketID:        quote.MarketID,
		EdgeBuyUp:       edgeBuyUp,
		EdgeBuyDown:     edgeBuyDown,
		EffectiveHurdle: hurdle,
		Timestamp:       time.Now(),
	}

	switch {
	case edgeBuyUp > edgeBuyDown && edgeBuyUp > hurdle:
		signal.Side = domain.SignalBuyUp
		signal.Reason = "buy_up_edge_exceeds_hurdle"
	case edgeBuyDown > edgeBuyUp && edgeBuyDown > hurdle:
		signal.Side = domain.SignalBuyDown
		signal.Reason = "buy_down_edge_exceeds_hurdle"
	default:
		signal.Side = domain.SignalNone
		signal.Reason = "no_trade"
	}

	return signal, nil
}
