package main

import (
	"Polybot/polymarket"
	"fmt"
	"log"
	"time"
)

func GetMarketName(market string, index int) (string, int64) {
	now := time.Now().Unix()
	ts := now - now%IntervalSeconds + int64(IntervalSeconds*index)
	return fmt.Sprintf("%s%d", market, ts), ts
}

func Listener(market string, gamma *polymarket.GammaMarket, marketWS *polymarket.WebSocketOrderBook) {
	go func() {
		subscribedList := make([]string, 0, LookAhead*2)
		subscribedMap := make(map[string]bool)

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			newTokenIDs := make([]string, 0, LookAhead*2)

			for i := 0; i < LookAhead; i++ {
				marketName, _ := GetMarketName(market, i)
				marketInfo, err := gamma.GetMarketBySlug(marketName)

				if err != nil {
					continue
				}

				AddMarket(marketInfo)
				SetActiveMarkets(market, gamma)

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
			}
		}
	}()
}

func SetActiveMarkets(market string, gamma *polymarket.GammaMarket) {
	ActiveMarketIDs = make(map[string]bool)

	for i := 0; i < LookAhead; i++ {
		marketName, _ := GetMarketName(market, i)
		marketInfo, err := gamma.GetMarketBySlug(marketName)
		if err != nil {
			continue
		}
		ActiveMarketIDs[marketInfo.MarketID] = true
	}
}
