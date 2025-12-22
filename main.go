package main

import (
	"Polybot/polymarket"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
)

const (
	LookAhead = 1
)

func run(market string) {
	gamma := polymarket.NewGammaMarket()
	client, err := NewPolymarketClient()

	if err != nil {
		log.Fatal("Error creating Polymarket client", err)
	}

	//orderExecutor := NewOrderExecutor(client)

	InitAssets(client)
	InitOrders(client)

	userWS := polymarket.NewWebSocketOrderBook(
		polymarket.UserChannel,
		func(msg []byte) {
			UpdateAsset(msg)
		},
	)

	marketWS := polymarket.NewWebSocketOrderBook(
		polymarket.MarketChannel,
		func(msg []byte) {
			assetIds := UpdateOrderBook(msg)
			fmt.Println(assetIds)
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

	if _, ok := marketMapping[market]; !ok {
		log.Fatalf("Invalid market: %s", market)
	}

	run(marketMapping[market])
}
