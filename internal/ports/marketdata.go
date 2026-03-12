package ports

import (
	"context"

	"Polybot/internal/domain"
)

type MarketDataProvider interface {
	GetActiveMarkets(ctx context.Context) ([]domain.BinaryMarket, error)
	GetQuote(ctx context.Context, marketID domain.MarketID) (domain.MarketQuote, error)
	SubscribeQuotes(ctx context.Context, marketIDs []domain.MarketID) (<-chan domain.MarketQuote, error)
}

type ReferencePriceProvider interface {
	GetLatestPrice(ctx context.Context, asset string) (domain.ReferenceSnapshot, error)
	SubscribePrices(ctx context.Context, asset string) (<-chan domain.ReferenceSnapshot, error)
}
