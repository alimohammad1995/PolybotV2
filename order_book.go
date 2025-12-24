package main

import (
	"Polybot/polymarket"
	"encoding/json"
	"fmt"
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

func PrintBestBidAsk(prefix, tokenID string) {
	bestBidAsk := GetBestBidAsk(tokenID)
	if len(bestBidAsk) == 2 {
		fmt.Printf("%s => Best Bid: (%v, %v), Best Ask: (%v, %v)\n", prefix, bestBidAsk[0].Price, bestBidAsk[0].Size, bestBidAsk[1].Price, bestBidAsk[1].Size)
	}
}

/*
UpdateOrderBook
[{"market":"0x0936baff284871821b1ab5c64d33d67f117392dba34d461fe04360df893dfde4","asset_id":"25245433628208476987364023432306599125597948324625578977436890641930158259698","timestamp":"1766481546487","hash":"8c7f21df4ea57ce24228402e20a307358e55a0fd","bids":[{"price":"0.01","size":"3769.8"},{"price":"0.02","size":"2492.3"},{"price":"0.03","size":"2333.7"},{"price":"0.04","size":"2143.9"},{"price":"0.05","size":"2287.4"},{"price":"0.06","size":"2226"},{"price":"0.07","size":"2205"},{"price":"0.08","size":"2184.7"},{"price":"0.09","size":"2070.9"},{"price":"0.1","size":"2074.4"},{"price":"0.11","size":"63.3"},{"price":"0.12","size":"128.98"},{"price":"0.13","size":"126.9"},{"price":"0.14","size":"125.1"},{"price":"0.15","size":"110.19"},{"price":"0.16","size":"109.69"},{"price":"0.17","size":"109.19"},{"price":"0.18","size":"108.8"},{"price":"0.19","size":"108.4"},{"price":"0.2","size":"108.1"},{"price":"0.21","size":"145.21"},{"price":"0.22","size":"129.6"},{"price":"0.23","size":"107.3"},{"price":"0.24","size":"107.1"},{"price":"0.25","size":"106.9"},{"price":"0.26","size":"106.8"},{"price":"0.27","size":"106.6"},{"price":"0.28","size":"106.4"},{"price":"0.29","size":"106.3"},{"price":"0.3","size":"106.2"},{"price":"0.31","size":"106.1"},{"price":"0.32","size":"106"},{"price":"0.33","size":"105.9"},{"price":"0.34","size":"105.8"},{"price":"0.35","size":"255.7"},{"price":"0.36","size":"255.6"},{"price":"0.37","size":"255.6"},{"price":"0.38","size":"255.5"},{"price":"0.39","size":"255.5"},{"price":"0.4","size":"265.4"},{"price":"0.41","size":"180.4"},{"price":"0.42","size":"175"},{"price":"0.43","size":"323"},{"price":"0.44","size":"356.92"},{"price":"0.45","size":"238.92"},{"price":"0.46","size":"304.72"},{"price":"0.47","size":"162"},{"price":"0.48","size":"172.92"},{"price":"0.49","size":"60"}],"asks":[{"price":"0.99","size":"3641.8"},{"price":"0.98","size":"2452.3"},{"price":"0.97","size":"2333.7"},{"price":"0.96","size":"2193.9"},{"price":"0.95","size":"2287.4"},{"price":"0.94","size":"2226"},{"price":"0.93","size":"2205"},{"price":"0.92","size":"2184.7"},{"price":"0.91","size":"2065.9"},{"price":"0.9","size":"2074.4"},{"price":"0.89","size":"113.3"},{"price":"0.88","size":"128.98"},{"price":"0.87","size":"276.9"},{"price":"0.86","size":"275.1"},{"price":"0.85","size":"260.19"},{"price":"0.84","size":"259.69"},{"price":"0.83","size":"259.19"},{"price":"0.82","size":"258.8"},{"price":"0.81","size":"258.4"},{"price":"0.8","size":"258.1"},{"price":"0.79","size":"265.8"},{"price":"0.78","size":"257.6"},{"price":"0.77","size":"257.3"},{"price":"0.76","size":"263.35"},{"price":"0.75","size":"256.9"},{"price":"0.74","size":"256.8"},{"price":"0.73","size":"206.6"},{"price":"0.72","size":"256.4"},{"price":"0.71","size":"256.3"},{"price":"0.7","size":"266.2"},{"price":"0.69","size":"276.09"},{"price":"0.68","size":"266"},{"price":"0.67","size":"180.9"},{"price":"0.66","size":"255.8"},{"price":"0.65","size":"180.7"},{"price":"0.64","size":"255.6"},{"price":"0.63","size":"255.6"},{"price":"0.62","size":"255.5"},{"price":"0.61","size":"244.5"},{"price":"0.6","size":"260.4"},{"price":"0.59","size":"255.4"},{"price":"0.58","size":"255.3"},{"price":"0.57","size":"255.3"},{"price":"0.56","size":"305.3"},{"price":"0.55","size":"741.22"},{"price":"0.54","size":"454.12"},{"price":"0.53","size":"320.82"},{"price":"0.52","size":"180"},{"price":"0.51","size":"182.92"},{"price":"0.5","size":"16"}],"event_type":"book","last_trade_price":"0.500"},{"market":"0x0936baff284871821b1ab5c64d33d67f117392dba34d461fe04360df893dfde4","asset_id":"20168250870314270898487593595943592687458358996170785976290704639743788087191","timestamp":"1766481546487","hash":"a86a52c77278ed12e78225aeab19a3138f9418c4","bids":[{"price":"0.01","size":"3641.8"},{"price":"0.02","size":"2452.3"},{"price":"0.03","size":"2333.7"},{"price":"0.04","size":"2193.9"},{"price":"0.05","size":"2287.4"},{"price":"0.06","size":"2226"},{"price":"0.07","size":"2205"},{"price":"0.08","size":"2184.7"},{"price":"0.09","size":"2065.9"},{"price":"0.1","size":"2074.4"},{"price":"0.11","size":"113.3"},{"price":"0.12","size":"128.98"},{"price":"0.13","size":"276.9"},{"price":"0.14","size":"275.1"},{"price":"0.15","size":"260.19"},{"price":"0.16","size":"259.69"},{"price":"0.17","size":"259.19"},{"price":"0.18","size":"258.8"},{"price":"0.19","size":"258.4"},{"price":"0.2","size":"258.1"},{"price":"0.21","size":"265.8"},{"price":"0.22","size":"257.6"},{"price":"0.23","size":"257.3"},{"price":"0.24","size":"263.35"},{"price":"0.25","size":"256.9"},{"price":"0.26","size":"256.8"},{"price":"0.27","size":"206.6"},{"price":"0.28","size":"256.4"},{"price":"0.29","size":"256.3"},{"price":"0.3","size":"266.2"},{"price":"0.31","size":"276.09"},{"price":"0.32","size":"266"},{"price":"0.33","size":"180.9"},{"price":"0.34","size":"255.8"},{"price":"0.35","size":"180.7"},{"price":"0.36","size":"255.6"},{"price":"0.37","size":"255.6"},{"price":"0.38","size":"255.5"},{"price":"0.39","size":"244.5"},{"price":"0.4","size":"260.4"},{"price":"0.41","size":"255.4"},{"price":"0.42","size":"255.3"},{"price":"0.43","size":"255.3"},{"price":"0.44","size":"305.3"},{"price":"0.45","size":"741.22"},{"price":"0.46","size":"454.12"},{"price":"0.47","size":"320.82"},{"price":"0.48","size":"180"},{"price":"0.49","size":"182.92"},{"price":"0.5","size":"16"}],"asks":[{"price":"0.99","size":"3769.8"},{"price":"0.98","size":"2492.3"},{"price":"0.97","size":"2333.7"},{"price":"0.96","size":"2143.9"},{"price":"0.95","size":"2287.4"},{"price":"0.94","size":"2226"},{"price":"0.93","size":"2205"},{"price":"0.92","size":"2184.7"},{"price":"0.91","size":"2070.9"},{"price":"0.9","size":"2074.4"},{"price":"0.89","size":"63.3"},{"price":"0.88","size":"128.98"},{"price":"0.87","size":"126.9"},{"price":"0.86","size":"125.1"},{"price":"0.85","size":"110.19"},{"price":"0.84","size":"109.69"},{"price":"0.83","size":"109.19"},{"price":"0.82","size":"108.8"},{"price":"0.81","size":"108.4"},{"price":"0.8","size":"108.1"},{"price":"0.79","size":"145.21"},{"price":"0.78","size":"129.6"},{"price":"0.77","size":"107.3"},{"price":"0.76","size":"107.1"},{"price":"0.75","size":"106.9"},{"price":"0.74","size":"106.8"},{"price":"0.73","size":"106.6"},{"price":"0.72","size":"106.4"},{"price":"0.71","size":"106.3"},{"price":"0.7","size":"106.2"},{"price":"0.69","size":"106.1"},{"price":"0.68","size":"106"},{"price":"0.67","size":"105.9"},{"price":"0.66","size":"105.8"},{"price":"0.65","size":"255.7"},{"price":"0.64","size":"255.6"},{"price":"0.63","size":"255.6"},{"price":"0.62","size":"255.5"},{"price":"0.61","size":"255.5"},{"price":"0.6","size":"265.4"},{"price":"0.59","size":"180.4"},{"price":"0.58","size":"175"},{"price":"0.57","size":"323"},{"price":"0.56","size":"356.92"},{"price":"0.55","size":"238.92"},{"price":"0.54","size":"304.72"},{"price":"0.53","size":"162"},{"price":"0.52","size":"172.92"},{"price":"0.51","size":"60"}],"event_type":"book","last_trade_price":"0.500"}]
{"market":"0x0936baff284871821b1ab5c64d33d67f117392dba34d461fe04360df893dfde4", "price_changes":[{"asset_id":"25245433628208476987364023432306599125597948324625578977436890641930158259698", "price":"0.49", "size":"55", "side":"BUY", "hash":"bad813f84a3e7be740b95924fed984639e6798bc", "best_bid":"0.49", "best_ask":"0.5"}, {"asset_id":"20168250870314270898487593595943592687458358996170785976290704639743788087191", "price":"0.51", "size":"55", "side":"SELL", "hash":"eef80d141ea160c72c8644fd33c4e7cac54728fb", "best_bid":"0.5", "best_ask":"0.51"}], "timestamp":"1766481546776", "event_type":"price_change"}
{"market":"0x0936baff284871821b1ab5c64d33d67f117392dba34d461fe04360df893dfde4", "asset_id":"20168250870314270898487593595943592687458358996170785976290704639743788087191", "bids":[{"price":"0.01", "size":"3641.8"}, {"price":"0.02", "size":"2452.3"}, {"price":"0.03", "size":"2333.7"}, {"price":"0.04", "size":"2193.9"}, {"price":"0.05", "size":"2287.4"}, {"price":"0.06", "size":"2226"}, {"price":"0.07", "size":"2205"}, {"price":"0.08", "size":"2184.7"}, {"price":"0.09", "size":"2065.9"}, {"price":"0.1", "size":"2074.4"}, {"price":"0.11", "size":"113.3"}, {"price":"0.12", "size":"128.98"}, {"price":"0.13", "size":"276.9"}, {"price":"0.14", "size":"275.1"}, {"price":"0.15", "size":"260.19"}, {"price":"0.16", "size":"259.69"}, {"price":"0.17", "size":"259.19"}, {"price":"0.18", "size":"258.8"}, {"price":"0.19", "size":"258.4"}, {"price":"0.2", "size":"258.1"}, {"price":"0.21", "size":"265.8"}, {"price":"0.22", "size":"257.6"}, {"price":"0.23", "size":"257.3"}, {"price":"0.24", "size":"263.35"}, {"price":"0.25", "size":"256.9"}, {"price":"0.26", "size":"256.8"}, {"price":"0.27", "size":"206.6"}, {"price":"0.28", "size":"256.4"}, {"price":"0.29", "size":"256.3"}, {"price":"0.3", "size":"266.2"}, {"price":"0.31", "size":"276.09"}, {"price":"0.32", "size":"266"}, {"price":"0.33", "size":"180.9"}, {"price":"0.34", "size":"255.8"}, {"price":"0.35", "size":"180.7"}, {"price":"0.36", "size":"255.6"}, {"price":"0.37", "size":"255.6"}, {"price":"0.38", "size":"255.5"}, {"price":"0.39", "size":"244.5"}, {"price":"0.4", "size":"260.4"}, {"price":"0.41", "size":"255.4"}, {"price":"0.42", "size":"255.3"}, {"price":"0.43", "size":"255.3"}, {"price":"0.44", "size":"305.3"}, {"price":"0.45", "size":"741.22"}, {"price":"0.46", "size":"454.12"}, {"price":"0.47", "size":"352.82"}, {"price":"0.48", "size":"212"}, {"price":"0.49", "size":"150.92"}], "asks":[{"price":"0.99", "size":"3769.8"}, {"price":"0.98", "size":"2492.3"}, {"price":"0.97", "size":"2333.7"}, {"price":"0.96", "size":"2143.9"}, {"price":"0.95", "size":"2287.4"}, {"price":"0.94", "size":"2226"}, {"price":"0.93", "size":"2205"}, {"price":"0.92", "size":"2184.7"}, {"price":"0.91", "size":"2070.9"}, {"price":"0.9", "size":"2074.4"}, {"price":"0.89", "size":"63.3"}, {"price":"0.88", "size":"128.98"}, {"price":"0.87", "size":"126.9"}, {"price":"0.86", "size":"125.1"}, {"price":"0.85", "size":"110.19"}, {"price":"0.84", "size":"109.69"}, {"price":"0.83", "size":"109.19"}, {"price":"0.82", "size":"108.8"}, {"price":"0.81", "size":"108.4"}, {"price":"0.8", "size":"108.1"}, {"price":"0.79", "size":"145.21"}, {"price":"0.78", "size":"129.6"}, {"price":"0.77", "size":"107.3"}, {"price":"0.76", "size":"107.1"}, {"price":"0.75", "size":"106.9"}, {"price":"0.74", "size":"106.8"}, {"price":"0.73", "size":"106.6"}, {"price":"0.72", "size":"106.4"}, {"price":"0.71", "size":"106.3"}, {"price":"0.7", "size":"106.2"}, {"price":"0.69", "size":"106.1"}, {"price":"0.68", "size":"106"}, {"price":"0.67", "size":"105.9"}, {"price":"0.66", "size":"105.8"}, {"price":"0.65", "size":"255.7"}, {"price":"0.64", "size":"255.6"}, {"price":"0.63", "size":"255.6"}, {"price":"0.62", "size":"255.5"}, {"price":"0.61", "size":"255.5"}, {"price":"0.6", "size":"265.4"}, {"price":"0.59", "size":"180.4"}, {"price":"0.58", "size":"235"}, {"price":"0.57", "size":"255"}, {"price":"0.56", "size":"356.92"}, {"price":"0.55", "size":"206.92"}, {"price":"0.54", "size":"314.72"}, {"price":"0.53", "size":"162"}, {"price":"0.52", "size":"204.92"}, {"price":"0.51", "size":"50"}, {"price":"0.5", "size":"28.84"}], "hash":"7d979de7586e893d33493d738300ab3f7027c661", "timestamp":"1766481547845", "event_type":"book"}
*/
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
