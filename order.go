package main

import (
	"log"
	"sync"
)

func InitOrders(client *PolymarketClient) {
	var wg sync.WaitGroup

	for marketID := range ActiveMarkets {
		wg.Add(1)
		go func(market string) {
			defer wg.Done()
			resp, err := client.GetClient().GetActiveOrders(map[string]string{"market": market})
			if err != nil {
				log.Printf("init orders: get active orders failed for %s: %v", market, err)
				return
			}
			applyOrders(resp)
		}(marketID)
	}

	wg.Wait()
}

func applyOrders(payload any) {
	raw, ok := payload.(map[string]any)
	if !ok {
		log.Printf("init orders: invalid response: %T", payload)
		return
	}
	data, ok := raw["data"].([]any)
	if !ok {
		log.Printf("init orders: missing orders data: %T", raw["data"])
		return
	}

	for _, item := range data {
		orderMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		orderID := stringFromAny(orderMap["id"])
		if orderID == "" || orderID == "<nil>" {
			continue
		}

		priceVal, okPrice := parseFloat(orderMap["price"])
		origSize, okOrig := parseFloat(orderMap["original_size"])
		matchedSize, okMatched := parseFloat(orderMap["size_matched"])
		if !okPrice || !okOrig || !okMatched {
			continue
		}

		AddOrder(&Order{
			ID:           orderID,
			MarketID:     stringFromAny(orderMap["market"]),
			AssetID:      stringFromAny(orderMap["asset_id"]),
			OriginalSize: origSize,
			MatchedSize:  matchedSize,
			Price:        PriceToInt(priceVal),
		})
	}
}
