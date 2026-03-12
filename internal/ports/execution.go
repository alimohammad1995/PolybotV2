package ports

import (
	"context"

	"Polybot/internal/domain"
)

type ExecutionProvider interface {
	BuyUp(ctx context.Context, marketID domain.MarketID, maxPrice float64, sizeUSD float64) error
	BuyDown(ctx context.Context, marketID domain.MarketID, maxPrice float64, sizeUSD float64) error
	ClosePosition(ctx context.Context, marketID domain.MarketID, side domain.PositionSide) error
}

type CostModel interface {
	EstimateAllInCost(ctx context.Context, marketID domain.MarketID) (float64, error)
}
