package ports

import (
	"context"

	"Polybot/internal/domain"
)

type PositionRepository interface {
	SavePosition(ctx context.Context, p domain.Position) error
	GetPosition(ctx context.Context, marketID domain.MarketID, side domain.PositionSide) (domain.Position, error)
	ListOpenPositions(ctx context.Context) ([]domain.Position, error)
	DeletePosition(ctx context.Context, marketID domain.MarketID, side domain.PositionSide) error
}

type EventRepository interface {
	SaveFairValue(ctx context.Context, fv domain.FairValue) error
	SaveSignal(ctx context.Context, signal domain.TradeSignal) error
	SaveQuote(ctx context.Context, quote domain.MarketQuote) error
	SaveReferenceSnapshot(ctx context.Context, snap domain.ReferenceSnapshot) error
	SaveFill(ctx context.Context, fill domain.Fill) error
	SaveSettlement(ctx context.Context, s domain.Settlement) error
}
