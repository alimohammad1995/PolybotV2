package main

import (
	"Polybot/polymarket"
	"fmt"
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

	s.placeMissingBuy(marketID, downToken, neededDownSize, downBestBidAsk[1], timeLeft, upAvg)
	s.placeMissingBuy(marketID, downToken, neededUpSize, upBestBidAsk[1], timeLeft, downAvg)

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
	if fPrice*qty <= PolymarketMinimumOrderValue {
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

func (s *Strategy) placeMissingBuy(marketID string, tokenID string, neededSize float64, bestAsk *MarketOrder, timeLeft int64, avg float64) {
	if bestAsk == nil || neededSize < PolymarketMinimumOrderSize {
		return
	}

	askPrice, askSize := bestAsk.Price, bestAsk.Size
	if askPrice > maxPriceForMissing(timeLeft, avg) {
		return
	}

	if askSize >= neededSize {
		s.placeLimitBuy(marketID, tokenID, askPrice, neededSize)
		return
	}

	buySize := math.Min(neededSize-PolymarketMinimumOrderSize, askSize)
	s.placeLimitBuy(marketID, tokenID, askPrice, buySize)
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

	levelSize := getLevels(highestBid)
	fmt.Println(levelSize)
	desired := make(map[int]float64, len(levelSize))
	for i, size := range levelSize {
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

	fmt.Println(desired, existing)

	for price, size := range desired {
		order := existing[price]
		if order != nil && math.Abs(order.OriginalSize-size) < eps {
			continue
		}
		s.placeLimitBuy(marketID, tokenID, price, size)
	}
}

var Level1 = []float64{5, 5, 5, 5}
var Level2 = []float64{10, 10, 10, 10}
var Level3 = []float64{20, 20, 20, 20}

func getLevels(price int) []float64 {
	actualPrice := intAbs(MaxPrice/2 - price)

	switch {
	case actualPrice <= 10:
		return Level1
	case actualPrice <= 30:
		return Level2
	default:
		return Level3
	}
}

func discountTarget(timeLeft int64) int {
	switch {
	case timeLeft > 10*60:
		return DBase
	case timeLeft > 5*60:
		return DLate
	case timeLeft > 2*60:
		return DFinal
	}

	return DLoss
}

func maxPriceForMissing(timeLeft int64, avg float64) int {
	return MaxPrice - discountTarget(timeLeft) - int(math.Ceil(avg))
}

func quoteDepthAllowed(tokenID string, price int, tleft int64) bool {
	return true
}
