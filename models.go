package main

var Orders = make(map[string]*Order)
var Inventory = make(map[string]*Asset)

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
