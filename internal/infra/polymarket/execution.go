package polymarket

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"Polybot/internal/domain"
	"Polybot/internal/service"
)

// ExecutionProvider implements ports.ExecutionProvider using the Polymarket CLOB.
type ExecutionProvider struct {
	client   *ClobClient
	registry *service.MarketRegistry
	logger   *slog.Logger
}

func NewExecutionProvider(client *ClobClient, registry *service.MarketRegistry, logger *slog.Logger) *ExecutionProvider {
	return &ExecutionProvider{
		client:   client,
		registry: registry,
		logger:   logger,
	}
}

func (e *ExecutionProvider) BuyUp(_ context.Context, marketID domain.MarketID, maxPrice float64, sizeUSD float64) error {
	market, ok := e.registry.GetMarket(marketID)
	if !ok {
		return fmt.Errorf("market %s not found in registry", marketID)
	}
	if market.UpTokenID == "" {
		return fmt.Errorf("market %s has no UpTokenID", marketID)
	}

	return e.placeMarketBuy(market.UpTokenID, maxPrice, sizeUSD, marketID, "UP")
}

func (e *ExecutionProvider) BuyDown(_ context.Context, marketID domain.MarketID, maxPrice float64, sizeUSD float64) error {
	market, ok := e.registry.GetMarket(marketID)
	if !ok {
		return fmt.Errorf("market %s not found in registry", marketID)
	}
	if market.DownTokenID == "" {
		return fmt.Errorf("market %s has no DownTokenID", marketID)
	}

	return e.placeMarketBuy(market.DownTokenID, maxPrice, sizeUSD, marketID, "DOWN")
}

func (e *ExecutionProvider) ClosePosition(_ context.Context, marketID domain.MarketID, side domain.PositionSide) error {
	market, ok := e.registry.GetMarket(marketID)
	if !ok {
		return fmt.Errorf("market %s not found in registry", marketID)
	}

	var tokenID string
	if side == domain.PositionUp {
		tokenID = market.UpTokenID
	} else {
		tokenID = market.DownTokenID
	}
	if tokenID == "" {
		return fmt.Errorf("market %s has no token ID for side %s", marketID, side)
	}

	// Sell to close — use a market sell at minimum price to fill immediately
	order, err := e.client.CreateMarketOrder(MarketOrderArgs{
		TokenID:   tokenID,
		Amount:    0, // TODO: look up position size from position service
		Side:      SideSell,
		OrderType: OrderTypeFOK,
	}, nil)
	if err != nil {
		return fmt.Errorf("create close order for %s %s: %w", marketID, side, err)
	}

	resp, err := e.client.PostOrder(&order, OrderTypeFOK)
	if err != nil {
		return fmt.Errorf("post close order for %s %s: %w", marketID, side, err)
	}

	e.logger.Info("[LIVE] close position",
		"market", marketID,
		"side", side,
		"token_id", tokenID,
		"response", fmt.Sprintf("%v", resp),
	)
	return nil
}

func (e *ExecutionProvider) placeMarketBuy(tokenID string, maxPrice float64, sizeUSD float64, marketID domain.MarketID, sideLabel string) error {
	order, err := e.client.CreateMarketOrder(MarketOrderArgs{
		TokenID:   tokenID,
		Amount:    sizeUSD,
		Side:      SideBuy,
		Price:     maxPrice,
		OrderType: OrderTypeFOK,
	}, nil)
	if err != nil {
		return fmt.Errorf("create %s order for %s: %w", sideLabel, marketID, err)
	}

	resp, err := e.client.PostOrder(&order, OrderTypeFOK)
	if err != nil {
		return fmt.Errorf("post %s order for %s: %w", sideLabel, marketID, err)
	}

	e.logger.Info("[LIVE] buy",
		"market", marketID,
		"side", sideLabel,
		"token_id", tokenID,
		"max_price", maxPrice,
		"size_usd", sizeUSD,
		"response", fmt.Sprintf("%v", resp),
		"timestamp", time.Now().Format(time.RFC3339),
	)
	return nil
}
