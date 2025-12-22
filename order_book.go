package main

import (
	"Polybot/polymarket"
	"encoding/json"
	"log"
	"sync"
)

var orderMu = &sync.RWMutex{}

// orderBook TokenID => MarketOrderBook
var orderBook = make(map[string]*MarketOrderBook)

type MarketOrder struct {
	Price int
	Size  float64
}

type MarketOrderBook struct {
	Asks    []float64 // Price => Size
	Bids    []float64 // Price => Size
	BestBid int       // Price
	BestAsk int       // Price
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

func GetBestBidAsk(tokenID string) []*MarketOrder {
	orderMu.RLock()
	defer orderMu.RUnlock()

	res := make([]*MarketOrder, 2)
	book, ok := orderBook[tokenID]
	if !ok || book == nil {
		return res
	}

	if book.BestBid >= 1 && book.BestBid <= 99 {
		res[0] = &MarketOrder{Price: book.BestBid, Size: book.Bids[book.BestBid]}
	}
	if book.BestAsk >= 1 && book.BestAsk <= 99 {
		res[1] = &MarketOrder{Price: book.BestAsk, Size: book.Asks[book.BestAsk]}
	}

	return res
}

func PrintTop(tokenID string) {
	orders := GetBestBidAsk(tokenID)
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
		orderMu.Lock()
		for _, msg := range messages {
			applyOrderBookEventLocked(msg, &assetIDs)
		}
		orderMu.Unlock()
		return assetIDs
	}

	var msg orderBookMessage
	if err := decodeJSON(message, &msg); err != nil {
		return nil
	}
	assetIDs := make([]string, 0, 2)
	orderMu.Lock()
	applyOrderBookEventLocked(msg, &assetIDs)
	orderMu.Unlock()
	return assetIDs
}

func DeleteOrderBook(assetIDs []string) {
	orderMu.Lock()
	defer orderMu.Unlock()

	for _, id := range assetIDs {
		delete(orderBook, id)
	}
}

func applyOrderBookEventLocked(msg orderBookMessage, assetIDs *[]string) {
	switch msg.EventType {
	case "book":
		if updated := applyBookSnapshotLocked(msg); len(updated) > 0 {
			*assetIDs = append(*assetIDs, updated...)
		}
	case "price_change":
		if updated := applyPriceChangeLocked(msg); len(updated) > 0 {
			*assetIDs = append(*assetIDs, updated...)
		}
	}
}

func applyBookSnapshotLocked(msg orderBookMessage) []string {
	assetID := msg.AssetID
	if assetID == "" || assetID == "<nil>" {
		return nil
	}

	book := &MarketOrderBook{
		Asks:    make([]float64, 101),
		Bids:    make([]float64, 101),
		BestBid: -1,
		BestAsk: 101,
	}

	for _, bid := range msg.Bids {
		priceIndex, okPrice := priceIndexFromString(bid.Price.String())
		size, okSize := parseFloat(bid.Size)
		if okPrice && okSize {
			book.Bids[priceIndex] = size
			if size > 0 && priceIndex > book.BestBid {
				book.BestBid = priceIndex
			}
		}
	}

	for _, ask := range msg.Asks {
		priceIndex, okPrice := priceIndexFromString(ask.Price.String())
		size, okSize := parseFloat(ask.Size)
		if okPrice && okSize {
			book.Asks[priceIndex] = size
			if size > 0 && (book.BestAsk < 0 || priceIndex < book.BestAsk) {
				book.BestAsk = priceIndex
			}
		}
	}

	orderBook[assetID] = book
	return []string{assetID}
}

func applyPriceChangeLocked(msg orderBookMessage) []string {
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
		priceIndex, okPrice := priceIndexFromString(change.Price.String())
		size, okSize := parseFloat(change.Size)
		if !okPrice || !okSize {
			continue
		}

		book, ok := orderBook[assetID]
		if !ok {
			continue
		}

		if side == string(polymarket.SideBuy) {
			book.Bids[priceIndex] = size
			if size > 0 {
				if priceIndex > book.BestBid {
					book.BestBid = priceIndex
				}
			} else if priceIndex == book.BestBid {
				book.BestBid = scanBestBid(book)
			}
		} else if side == string(polymarket.SideSell) {
			book.Asks[priceIndex] = size
			if size > 0 {
				if book.BestAsk < 0 || priceIndex < book.BestAsk {
					book.BestAsk = priceIndex
				}
			} else if priceIndex == book.BestAsk {
				book.BestAsk = scanBestAsk(book)
			}
		}

		assetIDs = append(assetIDs, assetID)
	}

	return assetIDs
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
