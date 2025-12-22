package main

import (
	"Polybot/polymarket"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"
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

	x, _ := json.Marshal(args)
	_ = string(x)
	log.Println(string(x))
	return fmt.Sprintf("%d", time.Now().Nanosecond()), nil

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
	log.Println("cancel orders:", orderIDs)
	return nil

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
	if id := v["orderId"].(string); id != "" {
		return id, true
	}

	return "", false
}
