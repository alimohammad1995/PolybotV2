package main

import (
	"Polybot/polymarket"
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
	Tag          string
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

	if existingOrder, ok := Orders[order.ID]; ok {
		if order.Tag == "" {
			order.Tag = existingOrder.Tag
		}
	}

	Orders[order.ID] = order

	if _, ok := MarketToOrderIDs[order.MarketID]; !ok {
		MarketToOrderIDs[order.MarketID] = make(map[string]bool)
	}
	MarketToOrderIDs[order.MarketID][order.ID] = true

	if _, ok := AssetToOrderIDs[order.AssetID]; !ok {
		AssetToOrderIDs[order.AssetID] = make(map[string]bool)
	}
	AssetToOrderIDs[order.AssetID][order.ID] = true
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
	UpdateAssetTracker(assetID, marketID, size, PriceToInt(price))
}

func UpdateAssetTracker(assetID, marketID string, size float64, price int) {
	info := GetMarketInfo(marketID)
	if assetID == info.ClobTokenIDs[0] {
		GetTracker(marketID).OnFillUp(price, size)
	} else {
		GetTracker(marketID).OnFillDown(price, size)
	}
}

func AddMarket(marketInfo *polymarket.GammaMarketSummary) {
	if marketInfo == nil {
		return
	}
	marketMu.Lock()
	defer marketMu.Unlock()

	MarketToMarketID[marketInfo.Slug] = marketInfo.MarketID
	MarketIDToMarketInfo[marketInfo.MarketID] = marketInfo

	tokenUp := marketInfo.ClobTokenIDs[0]
	tokenDown := marketInfo.ClobTokenIDs[1]

	TokenToMarketID[tokenUp] = marketInfo.MarketID
	TokenToMarketID[tokenDown] = marketInfo.MarketID

	TokenToTokenRival[tokenUp] = tokenDown
	TokenToTokenRival[tokenDown] = tokenUp
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

func GetOrderIDsByAssetAndTag(assetID, tag string) []string {
	ordersMu.RLock()
	defer ordersMu.RUnlock()
	set := AssetToOrderIDs[assetID]
	if len(set) == 0 {
		return nil
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		if tag == "" || Orders[id].Tag == tag {
			ids = append(ids, id)
		}
	}
	return ids
}

func GetOrdersByAssetAndTag(assetID, tag string) map[int]*Order {
	ordersMu.RLock()
	defer ordersMu.RUnlock()
	set := AssetToOrderIDs[assetID]
	if len(set) == 0 {
		return nil
	}
	out := make(map[int]*Order, len(set))
	for id := range set {
		order := Orders[id]
		if tag == "" || order.Tag == tag {
			out[order.Price] = order
		}
	}
	return out
}

func GetOrderIDsByMarket(marketID string, tag string) []string {
	ordersMu.RLock()
	defer ordersMu.RUnlock()
	set := MarketToOrderIDs[marketID]
	if len(set) == 0 {
		return nil
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		order := Orders[id]
		if tag == "" || order.Tag == tag {
			ids = append(ids, id)
		}
	}
	return ids
}

func GetPendingOrderSize(assetID string, tag string) float64 {
	ordersMu.RLock()
	defer ordersMu.RUnlock()
	set := AssetToOrderIDs[assetID]
	if len(set) == 0 {
		return 0
	}
	size := 0.0
	for id := range set {
		if tag != "" && Orders[id].Tag != tag {
			continue
		}
		size += Orders[id].OriginalSize - Orders[id].MatchedSize
	}
	return size
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
