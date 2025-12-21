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

func GetTop(tokenID string) []*Order {
	res := make([]*Order, 2)
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
		res[0] = &Order{Price: bestBid, Size: book.Bids[bestBid]}
	}
	if bestAsk < 101 {
		res[1] = &Order{Price: bestAsk, Size: book.Asks[bestAsk]}
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

	if OrderBook == nil {
		OrderBook = make(map[string]*OrderBookModel)
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
	eventType := fmt.Sprintf("%v", msg["event_type"])
	switch eventType {
	case "book":
		return applyBookSnapshot(msg)
	case "price_change":
		return applyPriceChange(msg)
	}

	return nil
}

func applyBookSnapshot(msg map[string]any) []string {
	assetID := fmt.Sprintf("%v", msg["asset_id"])
	if assetID == "" || assetID == "<nil>" {
		return nil
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
			continue
		}

		if side == string(polymarket.SideBuy) {
			book.Bids[floatToInt(price)] = size
		} else if side == string(polymarket.SideSell) {
			book.Asks[floatToInt(price)] = size
		}

		assetIDs = append(assetIDs, assetID)
	}

	return assetIDs
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
