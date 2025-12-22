package main

import (
	"Polybot/polymarket"
	"log"
	"math"
	"time"
)

const Workers = 5

type Strategy struct {
	client   *PolymarketClient
	executor *OrderExecutor
	markets  chan string
}

func NewStrategy(client *PolymarketClient) *Strategy {
	strategy := &Strategy{
		client:   client,
		executor: NewOrderExecutor(client),
		markets:  make(chan string, Workers),
	}
	strategy.Run()
	return strategy
}

func (s *Strategy) OnOrderBookUpdate(assetID []string) {
	marketsToCheck := map[string]bool{}
	for _, id := range assetID {
		if marketID, ok := TokenToMarketID[id]; ok {
			marketsToCheck[marketID] = true
			s.markets <- marketID
		}
	}

}

func (s *Strategy) OnAssetUpdate(assetID []string) {
	marketsToCheck := map[string]bool{}
	for _, id := range assetID {
		if marketID, ok := TokenToMarketID[id]; ok {
			marketsToCheck[marketID] = true
			s.markets <- marketID
		}
	}
}

func (s *Strategy) Run() {
	for i := 0; i < Workers; i++ {
		go func() {
			for {
				market := <-s.markets
				s.handle(market)
			}
		}()
	}
}

func (s *Strategy) handle(marketID string) {
	if !IsActiveMarket(marketID) {
		return
	}

	marketInfo := GetMarketInfo(marketID)
	if marketInfo == nil {
		return
	}

	now := time.Now().Unix()
	timeLeft := marketInfo.EndDateTS - now
	if timeLeft <= 0 {
		return
	}

	upToken := marketInfo.ClobTokenIDs[0]
	downToken := marketInfo.ClobTokenIDs[1]

	upQty, upAvg, _ := GetAssetPosition(upToken)
	downQty, downAvg, _ := GetAssetPosition(downToken)

	pairs := math.Min(upQty, downQty)
	neededDownSize := upQty - pairs
	neededUpSize := downQty - pairs

	upBestBidAsk := GetBestBidAsk(upToken)
	downBestBidAsk := GetBestBidAsk(downToken)

	if neededDownSize >= PolymarketMinimumOrderSize && downBestBidAsk[1] != nil {
		askDownPrice, askDownSize := downBestBidAsk[1].Price, downBestBidAsk[1].Size

		if askDownPrice <= maxPriceForMissing(DOWN, timeLeft, upAvg, downAvg) {
			if askDownSize >= neededDownSize {
				s.placeLimitBuy(marketID, downToken, askDownPrice, neededDownSize)
			} else {
				buySize := math.Min(neededDownSize-PolymarketMinimumOrderSize, askDownSize)
				s.placeLimitBuy(marketID, downToken, askDownPrice, buySize)
			}
		}
	}

	if neededUpSize > 0 && upBestBidAsk[1] != nil {
		askUpPrice, askUpSize := upBestBidAsk[1].Price, upBestBidAsk[1].Size

		if askUpPrice <= maxPriceForMissing(UP, timeLeft, upAvg, downAvg) {
			if askUpSize >= neededUpSize {
				s.placeLimitBuy(marketID, downToken, askUpPrice, neededUpSize)
			} else {
				buySize := math.Min(neededUpSize-PolymarketMinimumOrderSize, askUpSize)
				s.placeLimitBuy(marketID, downToken, askUpPrice, buySize)
			}
		}
	}

	allowUp := neededDownSize <= MaxUnmatched
	allowDown := neededUpSize <= MaxUnmatched

	if timeLeft <= StopNewUnmatchedSec {
		if neededDownSize > 0 {
			allowUp = false
		} else if neededUpSize > 0 {
			allowDown = false
		} else {
			allowUp = false
			allowDown = false
		}
	}

	if !allowUp && !allowDown {
		return
	}

	if allowUp && upBestBidAsk[0] != nil {
		upHighestBidPrice := upBestBidAsk[0].Price
		for i, size := range LevelSize {
			price := upHighestBidPrice - i
			if price >= MinPrice && quoteDepthAllowed(upToken, price, timeLeft) {
				s.ensureBid(marketID, upToken, price, size)
			} else {
				s.cancelLevel(upToken, price)
			}
		}
	} else {
		s.cancelSide(upToken)
	}

	if allowDown && downBestBidAsk[0] != nil {
		downHighestBidPrice := downBestBidAsk[0].Price

		for i, size := range LevelSize {
			price := downHighestBidPrice - i
			if price >= MinPrice && quoteDepthAllowed(downToken, price, timeLeft) {
				s.ensureBid(marketID, downToken, price, size)
			} else {
				s.cancelLevel(downToken, price)
			}
		}
	} else {
		s.cancelSide(downToken)
	}
}

func (s *Strategy) placeLimitBuy(marketID, tokenID string, price int, qty float64) {
	if qty < PolymarketMinimumOrderSize {
		return
	}
	orderID, err := s.executor.BuyLimit(tokenID, float64(price)/100.0, qty, polymarket.OrderTypeGTC)
	if err != nil {
		log.Printf("place buy failed: market=%s token=%s price=%d qty=%.4f err=%v", marketID, tokenID, price, qty, err)
		return
	}

	AddOrder(&Order{
		ID:           orderID,
		MarketID:     marketID,
		AssetID:      tokenID,
		OriginalSize: qty,
		MatchedSize:  0,
		Price:        price,
	})
}

func (s *Strategy) ensureBid(marketID, tokenID string, price int, qty float64) {
	order := GetOrderAtPrice(tokenID, price)
	if order != nil && order.OriginalSize < qty {
		s.cancelOrder(order.ID)
	}

	s.placeLimitBuy(marketID, tokenID, price, qty)
}

func (s *Strategy) cancelLevel(tokenID string, price int) {
	order := GetOrderAtPrice(tokenID, price)
	if order == nil {
		return
	}
	s.cancelOrder(order.ID)
}

func (s *Strategy) cancelSide(tokenID string) {
	orderIds := GetOrderIDByAsset(tokenID)
	if err := s.executor.CancelOrders(orderIds); err != nil {
		log.Printf("cancel order failed: orderID=%s err=%v", orderIds, err)
		return
	}
	DeleteOrder(orderIds...)
}

func (s *Strategy) cancelOrder(orderID string) {
	if err := s.executor.CancelOrders([]string{orderID}); err != nil {
		log.Printf("cancel order failed: orderID=%s err=%v", orderID, err)
		return
	}
	DeleteOrder(orderID)
}

func discountTarget(tleft int64) int {
	switch {
	case tleft > 10*60:
		return DBase
	case tleft > 2*60:
		return DLate
	default:
		return DFinal
	}
}

func maxPriceForMissing(side Side, timeLeft int64, avgUp, avgDown float64) int {
	limit := MaxPrice - discountTarget(timeLeft)

	switch side {
	case DOWN:
		return limit - int(math.Ceil(avgUp))
	case UP:
		return limit - int(math.Ceil(avgDown))
	}

	return 0
}

func quoteDepthAllowed(tokenID string, price int, tleft int64) bool {
	return true
}
