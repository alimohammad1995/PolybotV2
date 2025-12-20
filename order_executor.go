package main

import "Polybot/polymarket"

type OrderExecutor struct {
	client *PolymarketClient
}

func NewOrderExecutor(client *PolymarketClient) *OrderExecutor {
	return &OrderExecutor{client: client}
}

func (e *OrderExecutor) BuyLimit(tokenID string, price, size float64, orderType polymarket.OrderType) (any, error) {
	args := polymarket.OrderArgs{
		TokenID:    tokenID,
		Price:      price,
		Size:       size,
		Side:       polymarket.SideBuy,
		FeeRateBps: 0,
		Expiration: 0,
		Nonce:      0,
	}
	order, err := e.client.client.CreateOrder(args, nil)
	if err != nil {
		return nil, err
	}
	if orderType == "" {
		orderType = polymarket.OrderTypeGTC
	}
	return e.client.client.PostOrder(order, orderType)
}
