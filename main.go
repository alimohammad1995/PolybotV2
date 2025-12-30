package main

import (
	"Polybot/polymarket"
	"log"
	"os"

	"github.com/joho/godotenv"
)

var Mode string

const (
	LookAhead = 1
)

func run(market string) {
	gamma := polymarket.NewGammaMarket()
	client, err := NewPolymarketClient()

	if err != nil {
		log.Fatal("Error creating Polymarket client", err)
	}

	SetActiveMarkets(market, gamma)

	strategy := NewStrategy(client)
	snapshotManager.Run()

	InitAssets(client)
	InitOrders(client)

	userWS := polymarket.NewWebSocketOrderBook(
		polymarket.UserChannel,
		func(msg []byte) {
			tradeAssetIds := UpdateAsset(msg, client.Me())
			orderAssetIds := UpdateOrder(msg)

			assetIds := make([]string, 0, len(tradeAssetIds)+len(orderAssetIds))
			if len(tradeAssetIds) > 0 {
				assetIds = append(assetIds, tradeAssetIds...)
			}
			if len(orderAssetIds) > 0 {
				assetIds = append(assetIds, orderAssetIds...)
			}

			strategy.OnUpdate(assetIds)
		},
	)

	marketWS := polymarket.NewWebSocketOrderBook(
		polymarket.MarketChannel,
		func(msg []byte) {
			assetIds := UpdateOrderBook(msg)
			strategy.OnUpdate(assetIds)
		},
	)

	userWS.RunAsync(map[string]any{"markets": []string{}, "type": polymarket.UserChannel, "auth": client.GetCreds()})
	marketWS.RunAsync(nil)

	Listener(market, gamma, marketWS)

	select {}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	marketMapping := map[string]string{
		"btc": BtcMarketName,
		"eth": EthMarketName,
		"sol": SolMarketName,
		"xrp": XrpMarketName,
	}

	market := os.Getenv("MARKET")
	if market == "" {
		log.Fatal("MARKET env var is required")
	}

	Mode = os.Getenv("MODE")

	if _, ok := marketMapping[market]; !ok {
		log.Fatalf("Invalid market: %s", market)
	}

	run(marketMapping[market])
}
