package main

import (
	"Polybot/polymarket"
	"log"
	"math"
	"sync"
	"time"
)

const Workers = 5

type Strategy struct {
	executor  *OrderExecutor
	markets   chan string
	pending   map[string]bool
	pendingMu *sync.Mutex
}

func NewStrategy(client *PolymarketClient) *Strategy {
	strategy := &Strategy{
		executor:  NewOrderExecutor(client),
		markets:   make(chan string, Workers),
		pending:   make(map[string]bool),
		pendingMu: &sync.Mutex{},
	}
	strategy.Run()
	return strategy
}

func (s *Strategy) OnOrderBookUpdate(assetID []string) {
	s.enqueueMarketsFromAssets(assetID)
}

func (s *Strategy) OnAssetUpdate(assetID []string) {
	s.enqueueMarketsFromAssets(assetID)
}

func (s *Strategy) Run() {
	for i := 0; i < Workers; i++ {
		go func() {
			for {
				market := <-s.markets
				s.handle(market)
				s.markDone(market)
			}
		}()
	}
}

func (s *Strategy) enqueueMarketsFromAssets(assetIDs []string) {
	if len(assetIDs) == 0 {
		return
	}
	marketsToCheck := make(map[string]bool)
	for _, id := range assetIDs {
		if marketID, ok := GetMarketIDByToken(id); ok {
			marketsToCheck[marketID] = true
		}
	}
	for marketID := range marketsToCheck {
		s.enqueueMarket(marketID)
	}
}

func (s *Strategy) enqueueMarket(marketID string) {
	s.pendingMu.Lock()
	if s.pending[marketID] {
		s.pendingMu.Unlock()
		return
	}
	s.pending[marketID] = true
	s.pendingMu.Unlock()
	s.markets <- marketID
}

func (s *Strategy) markDone(marketID string) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	delete(s.pending, marketID)
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
	timeStarted := now - marketInfo.StartDateTS
	if timeLeft <= 0 {
		return
	}

	if timeStarted >= 0 && timeStarted <= MinimumStartWaitingSec {
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

	if upBestBidAsk[0] == nil || downBestBidAsk[0] == nil {
		return
	}

	allowUp := neededDownSize <= MaxUnmatched && upQty <= MaxSharePerSize
	allowDown := neededUpSize <= MaxUnmatched && downQty <= MaxSharePerSize

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

	if upBestBidAsk[0].Price >= downBestBidAsk[0].Price {
		if allowUp {
			s.syncBids(marketID, upToken, upBestBidAsk[0].Price, timeLeft)
		} else {
			s.cancelSide(upToken)
		}

		if allowDown {
			s.syncBids(marketID, downToken, downBestBidAsk[0].Price, timeLeft)
		} else {
			s.cancelSide(downToken)
		}
	} else {
		if allowDown {
			s.syncBids(marketID, downToken, downBestBidAsk[0].Price, timeLeft)
		} else {
			s.cancelSide(downToken)
		}

		if allowUp {
			s.syncBids(marketID, upToken, upBestBidAsk[0].Price, timeLeft)
		} else {
			s.cancelSide(upToken)
		}
	}
}

func (s *Strategy) placeLimitBuy(marketID, tokenID string, price int, qty float64) {
	if qty < PolymarketMinimumOrderSize {
		return
	}
	fPrice := float64(price) / 100.0
	if fPrice <= PolymarketMinimumOrderValue {
		return
	}

	orderID, err := s.executor.BuyLimit(tokenID, fPrice, qty, polymarket.OrderTypeGTC)
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

func (s *Strategy) cancelSide(tokenID string) error {
	orderIds := GetOrderIDsByAsset(tokenID)
	if len(orderIds) == 0 {
		return nil
	}
	if err := s.executor.CancelOrders(orderIds); err != nil {
		log.Printf("cancel order failed: orderID=%s err=%v", orderIds, err)
		return err
	}
	DeleteOrder(orderIds...)
	return nil
}

func (s *Strategy) syncBids(marketID, tokenID string, highestBid int, timeLeft int64) {
	if highestBid < MinPrice {
		s.cancelSide(tokenID)
		return
	}

	desired := make(map[int]float64, len(LevelSize))
	for i, size := range LevelSize {
		price := highestBid - i
		if price >= MinPrice && quoteDepthAllowed(tokenID, price, timeLeft) {
			desired[price] = size
		}
	}
	if len(desired) == 0 {
		s.cancelSide(tokenID)
		return
	}

	existing := GetOrdersByAsset(tokenID)
	cancels := make([]string, 0, len(existing))
	for price, order := range existing {
		desiredSize, ok := desired[price]
		if !ok || math.Abs(order.OriginalSize-desiredSize) >= eps {
			cancels = append(cancels, order.ID)
		}
	}

	if len(cancels) > 0 {
		if err := s.executor.CancelOrders(cancels); err != nil {
			log.Printf("cancel order failed: orderID=%s err=%v", cancels, err)
			return
		}
		DeleteOrder(cancels...)
	}

	for price, size := range desired {
		order := existing[price]
		if order != nil && math.Abs(order.OriginalSize-size) < eps {
			continue
		}
		s.placeLimitBuy(marketID, tokenID, price, size)
	}
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
