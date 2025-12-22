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

	for _, marketID := range GetActiveMarketIDs() {
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
			qtyBefore, avgBefore, costBefore := GetAssetPosition(assetID)
			AddAsset(assetID, marketID, sizeVal, priceVal)
			qtyAfter, avgAfter, costAfter := GetAssetPosition(assetID)
			log.Printf(
				"inventory init: asset=%s market=%s size=%.4f qty=%.4f->%.4f avg=%.4f->%.4f cost=%.4f->%.4f",
				assetID,
				marketID,
				sizeVal,
				qtyBefore,
				qtyAfter,
				avgBefore,
				avgAfter,
				costBefore,
				costAfter,
			)
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

	qtyBefore, avgBefore, costBefore := GetAssetPosition(assetID)
	AddAsset(assetID, marketID, sizeVal, priceVal)
	qtyAfter, avgAfter, costAfter := GetAssetPosition(assetID)
	log.Printf(
		"inventory update: asset=%s market=%s size=%.4f qty=%.4f->%.4f avg=%.4f->%.4f cost=%.4f->%.4f",
		assetID,
		marketID,
		sizeVal,
		qtyBefore,
		qtyAfter,
		avgBefore,
		avgAfter,
		costBefore,
		costAfter,
	)

	return []string{assetID}
}
