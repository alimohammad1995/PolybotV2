package ports

import (
	"context"

	"Polybot/internal/domain"
)

// OrderResult is returned by BuyUp/BuyDown with the exchange order ID.
// For paper mode, OrderID is empty and Filled is true (simulated instant fill).
type OrderResult struct {
	OrderID string
	Filled  bool    // true if FOK fill confirmed synchronously
	Price   float64 // actual fill price (if known)
	Size    float64 // actual fill size in tokens (if known)
}

type ExecutionProvider interface {
	BuyUp(ctx context.Context, marketID domain.MarketID, maxPrice float64, sizeUSD float64) (OrderResult, error)
	BuyDown(ctx context.Context, marketID domain.MarketID, maxPrice float64, sizeUSD float64) (OrderResult, error)
	ClosePosition(ctx context.Context, marketID domain.MarketID, side domain.PositionSide) error
}

type CostModel interface {
	EstimateAllInCost(ctx context.Context, marketID domain.MarketID) (float64, error)
}

// FillListener listens for confirmed trade fills and updates positions.
// In live mode this connects to the Polymarket user WebSocket.
// Run blocks until ctx is cancelled.
type FillListener interface {
	Run(ctx context.Context)
	LoadPositionsFromAPI(ctx context.Context) error
}
