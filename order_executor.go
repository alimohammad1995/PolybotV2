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
	args := &polymarket.OrderArgs{
		TokenID: tokenID,
		Price:   price,
		Size:    size,
		Side:    polymarket.SideBuy,
	}

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

func (e *OrderExecutor) BuyLimits(tokenIDs []string, prices, sizes []float64, orderType polymarket.OrderType) ([]string, error) {
	if len(tokenIDs) == 0 {
		return nil, nil
	}
	if orderType == "" {
		orderType = polymarket.OrderTypeGTC
	}

	postArgs := make([]polymarket.PostOrdersArgs, 0, len(tokenIDs))
	for i, tokenID := range tokenIDs {
		args := &polymarket.OrderArgs{
			TokenID: tokenID,
			Price:   prices[i],
			Size:    sizes[i],
			Side:    polymarket.SideBuy,
		}
		order, err := e.client.client.CreateOrder(args, nil)
		if err != nil {
			return nil, err
		}
		postArgs = append(postArgs, polymarket.PostOrdersArgs{
			Order:     order,
			OrderType: orderType,
		})
	}

	res, err := e.client.client.PostOrders(postArgs)
	if err != nil {
		return nil, err
	}
	orderIDs, success := parseOrderIDs(res)
	if !success {
		return nil, errors.New("orders failed")
	}
	return orderIDs, nil
}

func (e *OrderExecutor) CancelOrders(orderIDs []string, because string) error {
	if len(orderIDs) == 0 {
		return nil
	}

	log.Printf("order cancel %s: count=%d ids=%v", because, len(orderIDs), orderIDs)
	_, err := e.client.client.CancelOrders(orderIDs)

	if err == nil {
		DeleteOrder(orderIDs...)
	}

	return err
}

func (e *OrderExecutor) CancelAllOrders(marketID string) error {
	orderIDs := GetOrderIDsByMarket(marketID, "")
	_, err := e.client.client.CancelOrders(orderIDs)
	if err == nil {
		DeleteOrder(orderIDs...)
	}
	return err
}

func parseOrderID(resp any) (string, bool) {
	if resp == nil {
		return "", false
	}
	v, ok := resp.(map[string]any)
	if !ok {
		return "", false
	}

	success, _ := v["success"].(bool)
	if !success {
		return "", false
	}
	if id := v["orderID"].(string); id != "" {
		return id, true
	}

	return "", false
}

func parseOrderIDs(resp any) ([]string, bool) {
	if resp == nil {
		return nil, false
	}
	v := resp.([]any)
	orderIDs := make([]string, 0, len(v))
	for _, item := range v {
		if id, ok := parseOrderID(item); ok {
			orderIDs = append(orderIDs, id)
		}
	}
	return orderIDs, len(orderIDs) > 0
}
