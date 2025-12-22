package main

import (
	"Polybot/polymarket"
	"fmt"
	"sync"
)

const eps = 1e-9

// TODO use different mutex
var mu = &sync.Mutex{}

var ActiveMarketIDs = make(map[string]bool)

var Orders = make(map[string]*Order)
var MarketToOrderIDs = make(map[string][]string)
var AssetToOrderIDs = make(map[string][]string)
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
	mu.Lock()
	defer mu.Unlock()

	Orders[order.ID] = order

	MarketToOrderIDs[order.MarketID] = append(MarketToOrderIDs[order.MarketID], order.ID)
	AssetToOrderIDs[order.AssetID] = append(AssetToOrderIDs[order.AssetID], order.ID)

	assetPrice := fmt.Sprintf("%s_%d", order.AssetID, order.Price)
	AssetPriceToOrderID[assetPrice] = order.ID
}

func DeleteOrder(orderIDs ...string) {
	mu.Lock()
	defer mu.Unlock()

	for _, orderID := range orderIDs {
		delete(AssetToOrderIDs, Orders[orderID].AssetID)
		delete(MarketToOrderIDs, Orders[orderID].MarketID)

		assetPrice := fmt.Sprintf("%s_%d", Orders[orderID].AssetID, Orders[orderID].Price)
		delete(AssetPriceToOrderID, assetPrice)

		delete(Orders, orderID)
	}
}

func AddAsset(assetID, marketID string, size float64, price float64) {
	mu.Lock()
	defer mu.Unlock()

	asset := Inventory[assetID]
	if asset == nil {
		asset = &Asset{AssetID: assetID, MarketID: marketID}
		Inventory[assetID] = asset
	}

	asset.Update(PriceToInt(price), size)
}

func AddMarket(marketInfo *polymarket.GammaMarketSummary) {
	mu.Lock()
	defer mu.Unlock()

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
	mu.Lock()
	defer mu.Unlock()
	return ActiveMarketIDs[marketID]
}

func GetMarketInfo(marketID string) *polymarket.GammaMarketSummary {
	mu.Lock()
	defer mu.Unlock()
	return MarketIDToMarketInfo[marketID]
}

func GetAssetPosition(assetID string) (qty, avgPrice, cost float64) {
	mu.Lock()
	defer mu.Unlock()
	asset := Inventory[assetID]
	if asset == nil {
		return 0, 0, 0
	}
	return asset.Size, asset.AveragePrice, asset.Size * asset.AveragePrice
}

func GetOrderAtPrice(assetID string, price int) *Order {
	assetPrice := fmt.Sprintf("%s_%d", assetID, price)
	mu.Lock()
	defer mu.Unlock()
	orderID := AssetPriceToOrderID[assetPrice]
	return Orders[orderID]
}

func GetOrderIDByAsset(assetID string) []string {
	mu.Lock()
	defer mu.Unlock()
	return AssetToOrderIDs[assetID]
}

func OrderMatches(orderID string, price int, qty float64) bool {
	mu.Lock()
	defer mu.Unlock()
	order := Orders[orderID]
	if order == nil {
		return false
	}
	remaining := order.OriginalSize - order.MatchedSize
	if remaining < 0 {
		remaining = 0
	}
	return order.Price == price && absFloat(remaining-qty) < eps
}
