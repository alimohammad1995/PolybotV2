package main

import (
	"Polybot/polymarket"
	"log"
	"sync"
)

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
	}
}
