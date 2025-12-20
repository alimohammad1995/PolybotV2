package main

import (
	"Polybot/polymarket"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
)

var OrderBook map[string]*OrderBookModel

type Order struct {
	Price int
	Size  float64
}

type OrderBookModel struct {
	Asks []float64
	Bids []float64
}

func GetTop(tokenID string) []Order {
	res := make([][]Order, 2)

}

func UpdateOrderBook(message []byte) {
	var payload any
	if err := json.Unmarshal(message, &payload); err != nil {
		return
	}

	if OrderBook == nil {
		OrderBook = make(map[string]*OrderBookModel)
	}

	switch v := payload.(type) {
	case map[string]any:
		applyOrderBookEvent(v)
	case []any:
		for _, item := range v {
			if msg, ok := item.(map[string]any); ok {
				applyOrderBookEvent(msg)
			}
		}
	}
}

func DeleteOrderBook(assetIDs []string) {
	for _, id := range assetIDs {
		delete(OrderBook, id)
	}
}

func PrintOrderBook() {
	for tokenID, book := range OrderBook {
		log.Println(tokenID)
		log.Println("ASKS: ", book.Asks)
		log.Println("BIDS: ", book.Bids)
	}
}

func applyOrderBookEvent(msg map[string]any) {
	eventType := fmt.Sprintf("%v", msg["event_type"])
	switch eventType {
	case "book":
		applyBookSnapshot(msg)
	case "price_change":
		applyPriceChange(msg)
	}
}

func applyBookSnapshot(msg map[string]any) {
	assetID := fmt.Sprintf("%v", msg["asset_id"])
	if assetID == "" || assetID == "<nil>" {
		return
	}

	book := &OrderBookModel{
		Asks: make([]float64, 101),
		Bids: make([]float64, 101),
	}

	if bids, ok := msg["bids"].([]any); ok {
		for _, bid := range bids {
			if bidMap, ok := bid.(map[string]any); ok {
				price, okPrice := parseFloat(bidMap["price"])
				size, okSize := parseFloat(bidMap["size"])
				if okPrice && okSize {
					book.Bids[floatToInt(price)] = size
				}
			}
		}
	}

	if asks, ok := msg["asks"].([]any); ok {
		for _, ask := range asks {
			if askMap, ok := ask.(map[string]any); ok {
				price, okPrice := parseFloat(askMap["price"])
				size, okSize := parseFloat(askMap["size"])
				if okPrice && okSize {
					book.Asks[floatToInt(price)] = size
				}
			}
		}
	}

	OrderBook[assetID] = book
}

func applyPriceChange(msg map[string]any) {
	priceChanges, ok := msg["price_changes"].([]any)
	if !ok {
		return
	}

	for _, change := range priceChanges {
		changeMap, ok := change.(map[string]any)
		if !ok {
			continue
		}

		assetID := fmt.Sprintf("%v", changeMap["asset_id"])
		if assetID == "" || assetID == "<nil>" {
			continue
		}

		side := fmt.Sprintf("%v", changeMap["side"])
		price, okPrice := parseFloat(changeMap["price"])
		size, okSize := parseFloat(changeMap["size"])
		if !okPrice || !okSize {
			continue
		}

		book, ok := OrderBook[assetID]
		if !ok {
			return
		}

		if side == string(polymarket.SideBuy) {
			book.Bids[floatToInt(price)] = size
		} else if side == string(polymarket.SideSell) {
			book.Asks[floatToInt(price)] = size
		}
	}
}

func parseFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	default:
		f, err := strconv.ParseFloat(fmt.Sprintf("%v", value), 64)
		return f, err == nil
	}
}

func floatToInt(value float64) int {
	return int(value * 100)
}
