package main

import (
	"context"
	"log/slog"

	"Polybot/internal/domain"
)

// fixedCostModel returns a constant cost estimate.
type fixedCostModel struct {
	cost float64
}

func (f *fixedCostModel) EstimateAllInCost(_ context.Context, _ domain.MarketID) (float64, error) {
	return f.cost, nil
}

// paperExecutionProvider logs trades without executing.
type paperExecutionProvider struct {
	logger *slog.Logger
}

func (p *paperExecutionProvider) BuyUp(_ context.Context, marketID domain.MarketID, maxPrice float64, sizeUSD float64) error {
	p.logger.Info("[PAPER] buy UP", "market", marketID, "max_price", maxPrice, "size_usd", sizeUSD)
	return nil
}

func (p *paperExecutionProvider) BuyDown(_ context.Context, marketID domain.MarketID, maxPrice float64, sizeUSD float64) error {
	p.logger.Info("[PAPER] buy DOWN", "market", marketID, "max_price", maxPrice, "size_usd", sizeUSD)
	return nil
}

func (p *paperExecutionProvider) ClosePosition(_ context.Context, marketID domain.MarketID, side domain.PositionSide) error {
	p.logger.Info("[PAPER] close position", "market", marketID, "side", side)
	return nil
}

// stubMarketDataProvider is a placeholder until Polymarket adapter is complete.
type stubMarketDataProvider struct{}

func (s *stubMarketDataProvider) GetActiveMarkets(_ context.Context) ([]domain.BinaryMarket, error) {
	return nil, nil
}

func (s *stubMarketDataProvider) GetQuote(_ context.Context, _ domain.MarketID) (domain.MarketQuote, error) {
	return domain.MarketQuote{}, nil
}

func (s *stubMarketDataProvider) SubscribeQuotes(_ context.Context) (<-chan domain.MarketQuote, error) {
	ch := make(chan domain.MarketQuote)
	return ch, nil
}
