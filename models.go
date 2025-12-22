package main

import (
	"Polybot/polymarket"
	"fmt"
	"sync"
)

const eps = 1e-9

var (
	ordersMu    = &sync.RWMutex{}
	inventoryMu = &sync.RWMutex{}
	marketMu    = &sync.RWMutex{}
	activeMu    = &sync.RWMutex{}
)

var ActiveMarketIDs = make(map[string]bool)

var Orders = make(map[string]*Order)
var MarketToOrderIDs = make(map[string]map[string]bool)
var AssetToOrderIDs = make(map[string]map[string]bool)
var AssetPriceToOrderID = make(map[string]string)

var Inventory = make(map[string]*Asset)

var MarketToMarketID = make(map[string]string)
var MarketIDToMarketInfo = make(map[string]*polymarket.GammaMarketSummary)
var TokenToMarketID = make(map[string]string)
var TokenToTokenRival = make(map[string]string)

type Order struct {
	ID           string
	MarketID     string
	AssetID      string
	OriginalSize float64
	MatchedSize  float64
	Price        int
}

type Asset struct {
	AssetID      string
	MarketID     string
	Size         float64
	AveragePrice float64
}

func (asset *Asset) Update(price int, quantity float64) {
	asset.AveragePrice = (asset.AveragePrice*asset.Size + float64(price)*quantity) / (asset.Size + quantity)
	asset.Size += quantity
}

func AddOrder(order *Order) {
	if order == nil || order.ID == "" {
		return
	}
	ordersMu.Lock()
	defer ordersMu.Unlock()

	Orders[order.ID] = order

	if _, ok := MarketToOrderIDs[order.MarketID]; !ok {
		MarketToOrderIDs[order.MarketID] = make(map[string]bool)
	}
	MarketToOrderIDs[order.MarketID][order.ID] = true

	if _, ok := AssetToOrderIDs[order.AssetID]; !ok {
		AssetToOrderIDs[order.AssetID] = make(map[string]bool)
	}
	AssetToOrderIDs[order.AssetID][order.ID] = true

	assetPrice := fmt.Sprintf("%s_%d", order.AssetID, order.Price)
	AssetPriceToOrderID[assetPrice] = order.ID
}

func DeleteOrder(orderIDs ...string) {
	if len(orderIDs) == 0 {
		return
	}
	ordersMu.Lock()
	defer ordersMu.Unlock()

	for _, orderID := range orderIDs {
		order := Orders[orderID]
		if order == nil {
			continue
		}

		if assetSet, ok := AssetToOrderIDs[order.AssetID]; ok {
			delete(assetSet, orderID)
		}
		if marketSet, ok := MarketToOrderIDs[order.MarketID]; ok {
			delete(marketSet, orderID)
		}

		assetPrice := fmt.Sprintf("%s_%d", order.AssetID, order.Price)
		if AssetPriceToOrderID[assetPrice] == orderID {
			delete(AssetPriceToOrderID, assetPrice)
		}

		delete(Orders, orderID)
	}
}

func AddAsset(assetID, marketID string, size float64, price float64) {
	inventoryMu.Lock()
	defer inventoryMu.Unlock()

	asset := Inventory[assetID]
	if asset == nil {
		asset = &Asset{AssetID: assetID, MarketID: marketID}
		Inventory[assetID] = asset
	}

	asset.Update(PriceToInt(price), size)
}

func AddMarket(marketInfo *polymarket.GammaMarketSummary) {
	if marketInfo == nil {
		return
	}
	marketMu.Lock()
	defer marketMu.Unlock()

	MarketToMarketID[marketInfo.Slug] = marketInfo.MarketID
	MarketIDToMarketInfo[marketInfo.MarketID] = marketInfo

	tokenYes := marketInfo.ClobTokenIDs[0]
	tokenNo := marketInfo.ClobTokenIDs[1]

	TokenToMarketID[tokenYes] = marketInfo.MarketID
	TokenToMarketID[tokenNo] = marketInfo.MarketID

	TokenToTokenRival[tokenYes] = tokenNo
	TokenToTokenRival[tokenNo] = tokenYes
}

func IsActiveMarket(marketID string) bool {
	activeMu.RLock()
	defer activeMu.RUnlock()
	return ActiveMarketIDs[marketID]
}

func GetMarketInfo(marketID string) *polymarket.GammaMarketSummary {
	marketMu.RLock()
	defer marketMu.RUnlock()
	return MarketIDToMarketInfo[marketID]
}

func GetAssetPosition(assetID string) (qty, avgPrice, cost float64) {
	inventoryMu.RLock()
	defer inventoryMu.RUnlock()
	asset := Inventory[assetID]
	if asset == nil {
		return 0, 0, 0
	}
	return asset.Size, asset.AveragePrice, asset.Size * asset.AveragePrice
}

func GetOrderAtPrice(assetID string, price int) *Order {
	assetPrice := fmt.Sprintf("%s_%d", assetID, price)
	ordersMu.RLock()
	defer ordersMu.RUnlock()
	orderID := AssetPriceToOrderID[assetPrice]
	return Orders[orderID]
}

func GetOrderIDsByAsset(assetID string) []string {
	ordersMu.RLock()
	defer ordersMu.RUnlock()
	set := AssetToOrderIDs[assetID]
	if len(set) == 0 {
		return nil
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	return ids
}

func GetOrdersByAsset(assetID string) map[int]*Order {
	ordersMu.RLock()
	defer ordersMu.RUnlock()
	set := AssetToOrderIDs[assetID]
	if len(set) == 0 {
		return nil
	}
	out := make(map[int]*Order, len(set))
	for id := range set {
		order := Orders[id]
		if order != nil {
			out[order.Price] = order
		}
	}
	return out
}

func GetMarketIDByToken(tokenID string) (string, bool) {
	marketMu.RLock()
	defer marketMu.RUnlock()
	marketID, ok := TokenToMarketID[tokenID]
	return marketID, ok
}

func SetActiveMarketsMap(active map[string]bool) {
	activeMu.Lock()
	defer activeMu.Unlock()
	ActiveMarketIDs = active
}

func GetActiveMarketIDs() []string {
	activeMu.RLock()
	defer activeMu.RUnlock()
	ids := make([]string, 0, len(ActiveMarketIDs))
	for id := range ActiveMarketIDs {
		ids = append(ids, id)
	}
	return ids
}
