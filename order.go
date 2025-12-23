package main

import (
	"Polybot/polymarket"
	"log"
	"sync"
)

type orderEvent struct {
	polymarket.ActiveOrder
	EventType string `json:"event_type"`
}

func InitOrders(client *PolymarketClient) {
	var wg sync.WaitGroup

	for _, marketID := range GetActiveMarketIDs() {
		wg.Add(1)
		go func(market string) {
			defer wg.Done()
			resp, err := client.GetClient().GetActiveOrdersTyped(map[string]string{"market": market})
			if err != nil {
				log.Printf("init orders: get active orders failed for %s: %v", market, err)
				return
			}
			applyOrders(resp.Data)
		}(marketID)
	}

	wg.Wait()
}

/*
UpdateOrder
{"id":"0xc52997cc4facf3df99c19a85aa4ab7bc5b4b236ec76ac5d3ac3066bc91741272", "owner":"a35aea61-d3e2-77b0-ba53-3c4dd029291c", "market":"0x24d2bcfe9fce449c4a1283d1bd4182af2705f784a6803edab699dc1e5b292530", "asset_id":"41649298205647508587657983419737619054577844831472384636513906999914458655929", "side":"BUY", "order_owner":"a35aea61-d3e2-77b0-ba53-3c4dd029291c", "original_size":"5", "size_matched":"0", "price":"0.8", "associate_trades":[], "outcome":"Down", "type":"PLACEMENT", "created_at":"1766491880", "expiration":"0", "order_type":"GTC", "status":"LIVE", "maker_address":"0x795c3aAA5fd42EA3b3543A8F6958F0bC8b4F8171", "timestamp":"1766491880835", "event_type":"order"}
*/
func UpdateOrder(msg []byte) []string {
	if len(msg) == 0 {
		return nil
	}
	var event orderEvent
	if err := decodeJSON(msg, &event); err != nil {
		return nil
	}
	return applyOrderEvent(event)
}

func applyOrderEvent(order orderEvent) []string {
	if order.EventType != "order" {
		return nil
	}
	if order.Type != "UPDATE" {
		return nil
	}

	applyOrders([]polymarket.ActiveOrder{order.ActiveOrder})
	return []string{order.AssetID}
}

func applyOrders(data []polymarket.ActiveOrder) {
	for _, order := range data {
		orderID := order.ID
		if orderID == "" || orderID == "<nil>" {
			continue
		}

		priceVal, okPrice := parseFloat(order.Price)
		origSize, okOrig := parseFloat(order.OriginalSize)
		matchedSize, okMatched := parseFloat(order.SizeMatched)
		if !okPrice || !okOrig || !okMatched {
			continue
		}

		AddOrder(&Order{
			ID:           orderID,
			MarketID:     order.Market,
			AssetID:      order.AssetID,
			OriginalSize: origSize,
			MatchedSize:  matchedSize,
			Price:        PriceToInt(priceVal),
		})
		log.Printf(
			"order init: id=%s market=%s asset=%s price=%.4f size=%.4f matched=%.4f",
			orderID,
			order.Market,
			order.AssetID,
			priceVal,
			origSize,
			matchedSize,
		)
	}
}
