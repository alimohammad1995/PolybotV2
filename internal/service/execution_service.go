package service

import (
	"context"

	"Polybot/internal/domain"
	"Polybot/internal/ports"
)

type ExecutionService struct {
	Provider ports.ExecutionProvider
}

func NewExecutionService(provider ports.ExecutionProvider) *ExecutionService {
	return &ExecutionService{Provider: provider}
}

func (e *ExecutionService) Execute(ctx context.Context, req domain.ExecutionRequest) error {
	switch req.Side {
	case domain.SignalBuyUp, domain.SignalHedgeUp:
		return e.Provider.BuyUp(ctx, req.MarketID, req.MaxPrice, req.SizeUSD)
	case domain.SignalBuyDown, domain.SignalHedgeDown:
		return e.Provider.BuyDown(ctx, req.MarketID, req.MaxPrice, req.SizeUSD)
	default:
		return nil
	}
}
