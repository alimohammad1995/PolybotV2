package main

import (
	"Polybot/polymarket"
	"errors"
	"log"
)

type OrderExecutor struct {
	client *PolymarketClient
}

func NewOrderExecutor(client *PolymarketClient) *OrderExecutor {
	return &OrderExecutor{client: client}
}

func (e *OrderExecutor) BuyLimit(tokenID string, price, size float64, orderType polymarket.OrderType) (string, error) {
	args := polymarket.OrderArgs{
		TokenID: tokenID,
		Price:   price,
		Size:    size,
		Side:    polymarket.SideBuy,
	}

	log.Printf("order submit: side=buy token=%s price=%.4f size=%.4f type=%s", tokenID, price, size, orderType)

	order, err := e.client.client.CreateOrder(args, nil)
	if err != nil {
		return "", err
	}
	if orderType == "" {
		orderType = polymarket.OrderTypeGTC
	}
	res, err := e.client.client.PostOrder(order, orderType)
	if err != nil {
		return "", err
	}
	orderID, success := parseOrderID(res)
	if !success {
		return "", errors.New("order failed")
	}
	return orderID, nil
}

func (e *OrderExecutor) CancelOrders(orderIDs []string) error {
	if len(orderIDs) == 0 {
		return nil
	}

	log.Printf("order cancel: count=%d ids=%v", len(orderIDs), orderIDs)
	_, err := e.client.client.CancelOrders(orderIDs)
	return err
}

func parseOrderID(resp any) (string, bool) {
	if resp == nil {
		return "", false
	}
	v := resp.(map[string]any)

	success, _ := v["success"].(bool)
	if !success {
		return "", false
	}
	if id := v["orderID"].(string); id != "" {
		return id, true
	}

	return "", false
}
