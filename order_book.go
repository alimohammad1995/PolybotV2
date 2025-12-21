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

type orderBookLevel struct {
	Price json.Number `json:"price"`
	Size  json.Number `json:"size"`
}

type priceChange struct {
	AssetID string      `json:"asset_id"`
	Side    string      `json:"side"`
	Price   json.Number `json:"price"`
	Size    json.Number `json:"size"`
}

type orderBookMessage struct {
	EventType    string           `json:"event_type"`
	AssetID      string           `json:"asset_id"`
	Bids         []orderBookLevel `json:"bids"`
	Asks         []orderBookLevel `json:"asks"`
	PriceChanges []priceChange    `json:"price_changes"`
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
	if len(message) == 0 {
		return nil
	}
	if message[0] == '[' {
		var messages []orderBookMessage
		if err := decodeJSON(message, &messages); err != nil {
			return nil
		}
		assetIDs := make([]string, 0, len(messages)*2)
		for _, msg := range messages {
			if updatedAssetIds := applyOrderBookEvent(msg); len(updatedAssetIds) > 0 {
				assetIDs = append(assetIDs, updatedAssetIds...)
			}
		}
		return assetIDs
	}

	var msg orderBookMessage
	if err := decodeJSON(message, &msg); err != nil {
		return nil
	}
	return applyOrderBookEvent(msg)
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

func applyOrderBookEvent(msg orderBookMessage) []string {
	eventType := msg.EventType
	switch eventType {
	case "book":
		return applyBookSnapshot(msg)
	case "price_change":
		return applyPriceChange(msg)
	}

	return nil
}

func applyBookSnapshot(msg orderBookMessage) []string {
	assetID := msg.AssetID
	if assetID == "" || assetID == "<nil>" {
		return nil
	}

	book := &MarketOrderBook{
		Asks: make([]float64, 101),
		Bids: make([]float64, 101),
	}

	for _, bid := range msg.Bids {
		price, okPrice := parseFloat(bid.Price)
		size, okSize := parseFloat(bid.Size)
		if okPrice && okSize {
			book.Bids[PriceToInt(price)] = size
		}
	}

	for _, ask := range msg.Asks {
		price, okPrice := parseFloat(ask.Price)
		size, okSize := parseFloat(ask.Size)
		if okPrice && okSize {
			book.Asks[PriceToInt(price)] = size
		}
	}

	OrderBook[assetID] = book
	return []string{assetID}
}

func applyPriceChange(msg orderBookMessage) []string {
	if len(msg.PriceChanges) == 0 {
		return nil
	}
	assetIDs := make([]string, 0, len(msg.PriceChanges))

	for _, change := range msg.PriceChanges {
		assetID := change.AssetID
		if assetID == "" || assetID == "<nil>" {
			continue
		}

		side := change.Side
		price, okPrice := parseFloat(change.Price)
		size, okSize := parseFloat(change.Size)
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
