package main

import (
	"Polybot/polymarket"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
)

func InitAssets(client *PolymarketClient) {
	var wg sync.WaitGroup

	for marketID := range ActiveMarkets {
		wg.Add(1)
		go func(market string) {
			defer wg.Done()
			resp, err := client.GetClient().GetTrades(map[string]string{"market": market})
			if err != nil {
				log.Printf("init assets: get trades failed for %s: %v", market, err)
				return
			}
			applyTradesToInventory(resp)
		}(marketID)
	}

	wg.Wait()
}

func UpdateAsset(msg []byte) {
	var payload any
	if err := json.Unmarshal(msg, &payload); err != nil {
		return
	}

	switch v := payload.(type) {
	case map[string]any:
		applyAssetTradeEvent(v)
	case []any:
		for _, item := range v {
			if msgMap, ok := item.(map[string]any); ok {
				applyAssetTradeEvent(msgMap)
			}
		}
	}
}

func applyTradesToInventory(payload any) {
	raw, ok := payload.(map[string]any)
	if !ok {
		log.Printf("init assets: invalid trades response: %T", payload)
		return
	}
	data, ok := raw["data"].([]any)
	if !ok {
		log.Printf("init assets: missing trades data: %T", raw["data"])
		return
	}

	for _, item := range data {
		trade, ok := item.(map[string]any)
		if !ok {
			continue
		}

		assetID := fmt.Sprintf("%v", trade["asset_id"])
		if assetID == "" || assetID == "<nil>" {
			continue
		}
		marketID := fmt.Sprintf("%v", trade["market"])

		priceVal, okPrice := parseFloat(trade["price"])
		sizeVal, okSize := parseFloat(trade["size"])
		if !okPrice || !okSize {
			continue
		}
		side := fmt.Sprintf("%v", trade["side"])

		if side == string(polymarket.SideBuy) {
			AddAsset(assetID, marketID, sizeVal, priceVal)
		}
	}
}

func applyAssetTradeEvent(msg map[string]any) {
	eventType := strings.ToLower(fmt.Sprintf("%v", msg["event_type"]))
	if eventType == "" || eventType == "<nil>" {
		eventType = strings.ToLower(fmt.Sprintf("%v", msg["type"]))
	}
	if eventType != "trade" {
		return
	}

	assetID := fmt.Sprintf("%v", msg["asset_id"])
	if assetID == "" || assetID == "<nil>" {
		return
	}
	marketID := fmt.Sprintf("%v", msg["market"])

	priceVal, okPrice := parseFloat(msg["price"])
	sizeVal, okSize := parseFloat(msg["size"])
	if !okPrice || !okSize {
		return
	}

	side := fmt.Sprintf("%v", msg["side"])
	if side != string(polymarket.SideBuy) {
		return
	}

	AddAsset(assetID, marketID, sizeVal, priceVal)
}
