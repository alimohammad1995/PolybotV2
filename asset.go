package main

import (
	"Polybot/polymarket"
	"encoding/json"
	"log"
	"strings"
	"sync"
)

type assetTradeEvent struct {
	EventType string      `json:"event_type"`
	Type      string      `json:"type"`
	AssetID   string      `json:"asset_id"`
	Market    string      `json:"market"`
	Price     json.Number `json:"price"`
	Size      json.Number `json:"size"`
	Side      string      `json:"side"`
}

func InitAssets(client *PolymarketClient) {
	var wg sync.WaitGroup

	for marketID := range ActiveMarketIDs {
		wg.Add(1)
		go func(market string) {
			defer wg.Done()
			resp, err := client.GetClient().GetTradesTyped(map[string]string{"market": market})
			if err != nil {
				log.Printf("init assets: get trades failed for %s: %v", market, err)
				return
			}
			applyTradesToInventory(resp.Data)
		}(marketID)
	}

	wg.Wait()
}

func UpdateAsset(msg []byte) []string {
	if len(msg) == 0 {
		return nil
	}
	var event assetTradeEvent
	if err := decodeJSON(msg, &event); err != nil {
		return nil
	}
	return applyAssetTradeEvent(event)
}

func applyTradesToInventory(data []polymarket.Trade) {
	for _, trade := range data {
		assetID := trade.AssetID
		if assetID == "" || assetID == "<nil>" {
			continue
		}
		marketID := trade.Market

		priceVal, okPrice := parseFloat(trade.Price)
		sizeVal, okSize := parseFloat(trade.Size)
		if !okPrice || !okSize {
			continue
		}
		side := trade.Side

		if side == string(polymarket.SideBuy) {
			AddAsset(assetID, marketID, sizeVal, priceVal)
		}
	}
}

func applyAssetTradeEvent(msg assetTradeEvent) []string {
	eventType := strings.ToLower(msg.EventType)
	if eventType == "" || eventType == "<nil>" {
		eventType = strings.ToLower(msg.Type)
	}
	if eventType != "trade" {
		return nil
	}

	assetID := msg.AssetID
	if assetID == "" || assetID == "<nil>" {
		return nil
	}
	marketID := msg.Market

	priceVal, okPrice := parseFloat(msg.Price)
	sizeVal, okSize := parseFloat(msg.Size)
	if !okPrice || !okSize {
		return nil
	}

	side := msg.Side
	if side != string(polymarket.SideBuy) {
		return nil
	}

	AddAsset(assetID, marketID, sizeVal, priceVal)
	return []string{assetID}
}
