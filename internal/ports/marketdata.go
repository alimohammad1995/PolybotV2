package ports

import (
	"context"
	"time"

	"Polybot/internal/domain"
)

type MarketDataProvider interface {
	GetActiveMarkets(ctx context.Context) ([]domain.BinaryMarket, error)
	GetQuote(ctx context.Context, marketID domain.MarketID) (domain.MarketQuote, error)
	SubscribeQuotes(ctx context.Context) (<-chan domain.MarketQuote, error)
}

type ReferencePriceProvider interface {
	GetLatestPrice(ctx context.Context, asset string) (domain.ReferenceSnapshot, error)
	GetPriceAtTime(ctx context.Context, asset string, ts time.Time) (domain.ReferenceSnapshot, error)
	SubscribePrices(ctx context.Context, asset string) (<-chan domain.ReferenceSnapshot, error)
}
