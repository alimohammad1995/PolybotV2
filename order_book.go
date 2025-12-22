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
	Asks    []float64
	Bids    []float64
	bestBid int
	bestAsk int
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

	if book.bestBid >= 0 {
		res[0] = &MarketOrder{Price: book.bestBid, Size: book.Bids[book.bestBid]}
	}
	if book.bestAsk >= 0 && book.bestAsk < len(book.Asks) {
		res[1] = &MarketOrder{Price: book.bestAsk, Size: book.Asks[book.bestAsk]}
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
		Asks:    make([]float64, 101),
		Bids:    make([]float64, 101),
		bestBid: -1,
		bestAsk: 101,
	}

	for _, bid := range msg.Bids {
		priceIndex, okPrice := parsePriceIndex(bid.Price)
		size, okSize := parseFloat(bid.Size)
		if okPrice && okSize {
			book.Bids[priceIndex] = size
			if size > 0 && priceIndex > book.bestBid {
				book.bestBid = priceIndex
			}
		}
	}

	for _, ask := range msg.Asks {
		priceIndex, okPrice := parsePriceIndex(ask.Price)
		size, okSize := parseFloat(ask.Size)
		if okPrice && okSize {
			book.Asks[priceIndex] = size
			if size > 0 && (book.bestAsk < 0 || priceIndex < book.bestAsk) {
				book.bestAsk = priceIndex
			}
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
		priceIndex, okPrice := parsePriceIndex(change.Price)
		size, okSize := parseFloat(change.Size)
		if !okPrice || !okSize {
			continue
		}

		book, ok := OrderBook[assetID]
		if !ok {
			continue
		}

		if side == string(polymarket.SideBuy) {
			book.Bids[priceIndex] = size
			if size > 0 {
				if priceIndex > book.bestBid {
					book.bestBid = priceIndex
				}
			} else if priceIndex == book.bestBid {
				book.bestBid = scanBestBid(book)
			}
		} else if side == string(polymarket.SideSell) {
			book.Asks[priceIndex] = size
			if size > 0 {
				if book.bestAsk < 0 || priceIndex < book.bestAsk {
					book.bestAsk = priceIndex
				}
			} else if priceIndex == book.bestAsk {
				book.bestAsk = scanBestAsk(book)
			}
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

func parsePriceIndex(value any) (int, bool) {
	switch v := value.(type) {
	case json.Number:
		return priceIndexFromString(v.String())
	case string:
		return priceIndexFromString(v)
	case float64:
		return PriceToInt(v), true
	case float32:
		return PriceToInt(float64(v)), true
	default:
		return 0, false
	}
}

func priceIndexFromString(value string) (int, bool) {
	if value == "" || value == "<nil>" {
		return 0, false
	}
	intPart := 0
	frac := 0
	fracDigits := 0
	i := 0
	n := len(value)
	for i < n && value[i] != '.' {
		ch := value[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		intPart = intPart*10 + int(ch-'0')
		i++
	}
	if i == n {
		return intPart * 100, true
	}
	if value[i] != '.' {
		return 0, false
	}
	i++
	for i < n && fracDigits < 2 {
		ch := value[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		frac = frac*10 + int(ch-'0')
		fracDigits++
		i++
	}
	if fracDigits == 1 {
		frac *= 10
	}
	return intPart*100 + frac, true
}

func PriceToInt(value float64) int {
	return int(value * 100)
}

func scanBestBid(book *MarketOrderBook) int {
	for i := len(book.Bids) - 1; i >= 0; i-- {
		if book.Bids[i] > 0 {
			return i
		}
	}
	return -1
}

func scanBestAsk(book *MarketOrderBook) int {
	for i := 0; i < len(book.Asks); i++ {
		if book.Asks[i] > 0 {
			return i
		}
	}
	return len(book.Asks)
}
