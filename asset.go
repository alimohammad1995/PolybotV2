package main

import (
	"Polybot/polymarket"
	"log"
	"strings"
	"sync"
)

type assetTradeEvent struct {
	EventType   string                  `json:"event_type"`
	Type        string                  `json:"type"`
	AssetID     string                  `json:"asset_id"`
	Market      string                  `json:"market"`
	Side        string                  `json:"side"`
	MakerOrders []polymarket.MakerOrder `json:"maker_orders"`
}

func InitAssets(client *PolymarketClient) {
	var wg sync.WaitGroup
	funder := client.Me()

	for _, marketID := range GetActiveMarketIDs() {
		wg.Add(1)
		go func(market string) {
			defer wg.Done()
			resp, err := client.GetClient().GetTradesTyped(map[string]string{"market": market})
			if err != nil {
				log.Printf("init assets: get trades failed for %s: %v", market, err)
				return
			}
			applyTradesToInventory(resp.Data, funder)
		}(marketID)
	}

	wg.Wait()
}

func UpdateAsset(msg []byte, funder string) []string {
	if len(msg) == 0 {
		return nil
	}
	var event assetTradeEvent
	if err := decodeJSON(msg, &event); err != nil {
		return nil
	}
	return applyAssetTradeEvent(event, funder)
}

func applyAssetTradeEvent(msg assetTradeEvent, funder string) []string {
	eventType := strings.ToLower(msg.EventType)
	if eventType != "trade" {
		return nil
	}

	return applyMakerOrders(msg.Market, msg.MakerOrders, funder, "inventory update")
}

func applyTradesToInventory(data []polymarket.Trade, funder string) {
	for _, trade := range data {
		applyMakerOrders(trade.Market, trade.MakerOrders, funder, "inventory init")
	}
}

func applyMakerOrders(marketID string, orders []polymarket.MakerOrder, normalizedFunder string, logPrefix string) []string {
	updated := make([]string, 0, len(orders))

	for _, order := range orders {
		if order.MakerAddress != normalizedFunder {
			continue
		}

		assetID := order.AssetID
		priceVal, okPrice := parseFloat(order.Price)
		sizeVal, okSize := parseFloat(order.MatchedAmount)
		if !okPrice || !okSize {
			continue
		}
		side := order.Side

		if side != string(polymarket.SideBuy) {
			continue
		}

		qtyBefore, avgBefore, costBefore := GetAssetPosition(assetID)
		AddAsset(assetID, marketID, sizeVal, priceVal)
		qtyAfter, avgAfter, costAfter := GetAssetPosition(assetID)
		log.Printf(
			"%s: asset=%s market=%s size=%.4f qty=%.4f->%.4f avg=%.4f->%.4f cost=%.4f->%.4f",
			logPrefix,
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

		updated = append(updated, assetID)
	}

	return updated
}
