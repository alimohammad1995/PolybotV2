package main

import (
	"Polybot/polymarket"
	"encoding/json"
	"log"
	"strconv"
)

// OrderBook TokenID => MarketOrderBook
var OrderBook = make(map[string]*MarketOrderBook)

type MarketOrder struct {
	Price int
	Size  float64
}

type MarketOrderBook struct {
	Asks []float64
	Bids []float64
}

func GetTop(tokenID string) []*MarketOrder {
	res := make([]*MarketOrder, 2)
	book, ok := OrderBook[tokenID]
	if !ok || book == nil {
		return res
	}

	bestBid := -1
	bestAsk := 101

	for i := 0; i < len(book.Bids); i++ {
		bidIndex := i
		askIndex := len(book.Asks) - i - 1

		if book.Bids[bidIndex] > 0 {
			bestBid = bidIndex
		}
		if book.Asks[askIndex] > 0 {
			bestAsk = askIndex
		}
	}

	if bestBid >= 0 {
		res[0] = &MarketOrder{Price: bestBid, Size: book.Bids[bestBid]}
	}
	if bestAsk < 101 {
		res[1] = &MarketOrder{Price: bestAsk, Size: book.Asks[bestAsk]}
	}

	return res
}

func PrintTop(tokenID string) {
	orders := GetTop(tokenID)
	jsonOrders, _ := json.Marshal(orders)
	log.Println(tokenID, "=>", string(jsonOrders))
}

func UpdateOrderBook(message []byte) []string {
	var payload any
	if err := json.Unmarshal(message, &payload); err != nil {
		return nil
	}

	switch v := payload.(type) {
	case map[string]any:
		return applyOrderBookEvent(v)
	case []any:
		assetIDs := make([]string, 0, len(v)*2)

		for _, item := range v {
			if msg, ok := item.(map[string]any); ok {
				if updatedAssetIds := applyOrderBookEvent(msg); len(updatedAssetIds) > 0 {
					assetIDs = append(assetIDs, updatedAssetIds...)
				}
			}
		}
		return assetIDs
	}

	return nil
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

func applyOrderBookEvent(msg map[string]any) []string {
	eventType := stringFromAny(msg["event_type"])
	switch eventType {
	case "book":
		return applyBookSnapshot(msg)
	case "price_change":
		return applyPriceChange(msg)
	}

	return nil
}

func applyBookSnapshot(msg map[string]any) []string {
	assetID := stringFromAny(msg["asset_id"])
	if assetID == "" || assetID == "<nil>" {
		return nil
	}

	book := &MarketOrderBook{
		Asks: make([]float64, 101),
		Bids: make([]float64, 101),
	}

	if bids, ok := msg["bids"].([]any); ok {
		for _, bid := range bids {
			if bidMap, ok := bid.(map[string]any); ok {
				price, okPrice := parseFloat(bidMap["price"])
				size, okSize := parseFloat(bidMap["size"])
				if okPrice && okSize {
					book.Bids[PriceToInt(price)] = size
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
					book.Asks[PriceToInt(price)] = size
				}
			}
		}
	}

	OrderBook[assetID] = book
	return []string{assetID}
}

func applyPriceChange(msg map[string]any) []string {
	priceChanges, ok := msg["price_changes"].([]any)
	if !ok {
		return nil
	}
	assetIDs := make([]string, 0, len(priceChanges))

	for _, change := range priceChanges {
		changeMap, ok := change.(map[string]any)
		if !ok {
			continue
		}

		assetID := stringFromAny(changeMap["asset_id"])
		if assetID == "" || assetID == "<nil>" {
			continue
		}

		side := stringFromAny(changeMap["side"])
		price, okPrice := parseFloat(changeMap["price"])
		size, okSize := parseFloat(changeMap["size"])
		if !okPrice || !okSize {
			continue
		}

		book, ok := OrderBook[assetID]
		if !ok {
			continue
		}

		if side == string(polymarket.SideBuy) {
			book.Bids[PriceToInt(price)] = size
		} else if side == string(polymarket.SideSell) {
			book.Asks[PriceToInt(price)] = size
		}

		assetIDs = append(assetIDs, assetID)
	}

	return assetIDs
}

func parseFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func PriceToInt(value float64) int {
	return int(value * 100)
}
