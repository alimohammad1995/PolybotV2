package main

import (
	"Polybot/polymarket"
	"log"
	"os"
	"time"

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

	NewOrderExecutor(client)

	userWS := polymarket.NewWebSocketOrderBook(
		polymarket.UserChannel,
		func(msg []byte) {
		},
	)

	marketWS := polymarket.NewWebSocketOrderBook(
		polymarket.MarketChannel,
		func(msg []byte) {
			UpdateOrderBook(msg)
		},
	)

	userWS.RunAsync(map[string]any{"markets": []string{}, "type": polymarket.UserChannel, "auth": client.GetCreds()})
	marketWS.RunAsync(nil)

	subscribedList := make([]string, 0, LookAhead*2)
	subscribedMap := make(map[string]bool)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		PrintOrderBook()

		newTokenIDs := make([]string, 0, LookAhead*2)

		for i := 0; i < LookAhead; i++ {
			marketName, _ := GetMarketName(market, i)
			marketInfo, err := gamma.GetMarketBySlug(marketName)

			if err != nil {
				continue
			}

			tokenYes := marketInfo.ClobTokenIDs[0]
			tokenNo := marketInfo.ClobTokenIDs[1]

			if !subscribedMap[tokenYes] {
				newTokenIDs = append(newTokenIDs, tokenYes)
			}
			if !subscribedMap[tokenNo] {
				newTokenIDs = append(newTokenIDs, tokenNo)
			}
		}

		if len(newTokenIDs) == 0 {
			continue
		}

		log.Println("Subscribing to token IDs", newTokenIDs)
		if err := marketWS.SubscribeToTokenIDs(newTokenIDs); err != nil {
			log.Println("Error subscribing to token IDs", err, newTokenIDs)
			continue
		}

		for _, tokenID := range newTokenIDs {
			subscribedMap[tokenID] = true
		}
		subscribedList = append(subscribedList, newTokenIDs...)

		if len(subscribedList) > 2*LookAhead {
			unsubscribeList := subscribedList[:len(subscribedList)-LookAhead*2]
			log.Println("Unsubscribing to token IDs", unsubscribeList)
			if err := marketWS.UnsubscribeToTokenIDs(unsubscribeList); err != nil {
				log.Println("Error unsubscribing to token IDs", err, unsubscribeList)
			}
			subscribedList = subscribedList[len(subscribedList)-LookAhead*2:]
			DeleteOrderBook(unsubscribeList)
		}
	}
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
