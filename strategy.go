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

	upQty, upAvg, upCost := GetAssetPosition(upToken)
	downQty, downAvg, downCost := GetAssetPosition(downToken)

	pairs := math.Min(upQty, downQty)
	neededDownSize := upQty - pairs
	neededUpSize := downQty - pairs

	if upCost+downCost+circuitDelta(timeLeft)*100 <= pairs {
		orderIDs := GetOrderIDsByMarket(marketID)
		if len(orderIDs) > 0 {
			if err := s.executor.CancelOrders(orderIDs); err != nil {
				log.Printf("circuit break cancel failed: market=%s err=%v", marketID, err)
			}
		}
		return
	}

	upBestBidAsk := GetBestBidAsk(upToken)
	downBestBidAsk := GetBestBidAsk(downToken)

	s.syncCompletionOrders(
		marketID,
		upToken,
		downToken,
		neededDownSize,
		neededUpSize,
		timeLeft,
		upAvg,
		downAvg,
		upBestBidAsk[1],
		downBestBidAsk[1],
	)

	if upBestBidAsk[0] == nil || downBestBidAsk[0] == nil {
		return
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

	upPendingQty := GetPendingOrderSize(upToken)
	downPendingQty := GetPendingOrderSize(downToken)

	if upQty+upPendingQty > MaxHoldingSharePerSize {
		allowUp = false
	}
	if downQty+downPendingQty > MaxHoldingSharePerSize {
		allowDown = false
	}

	if upBestBidAsk[0].Price >= downBestBidAsk[0].Price {
		if allowUp {
			s.syncBids(marketID, upToken, upBestBidAsk[0].Price, timeLeft)
		} else {
			s.cancelSide(upToken, OrderTagMaker)
		}

		if allowDown {
			s.syncBids(marketID, downToken, downBestBidAsk[0].Price, timeLeft)
		} else {
			s.cancelSide(downToken, OrderTagMaker)
		}
	} else {
		if allowDown {
			s.syncBids(marketID, downToken, downBestBidAsk[0].Price, timeLeft)
		} else {
			s.cancelSide(downToken, OrderTagMaker)
		}

		if allowUp {
			s.syncBids(marketID, upToken, upBestBidAsk[0].Price, timeLeft)
		} else {
			s.cancelSide(upToken, OrderTagMaker)
		}
	}
}

func (s *Strategy) placeLimitBuy(marketID, tokenID string, price int, qty float64, tag string) string {
	if qty < PolymarketMinimumOrderSize {
		return ""
	}
	fPrice := float64(price) / 100.0
	if fPrice*qty <= PolymarketMinimumOrderValue {
		return ""
	}

	orderID, err := s.executor.BuyLimit(tokenID, fPrice, qty, polymarket.OrderTypeGTC)
	if err != nil {
		log.Printf("place buy failed: market=%s token=%s price=%d qty=%.4f err=%v", marketID, tokenID, price, qty, err)
		return ""
	}

	AddOrder(&Order{
		ID:           orderID,
		MarketID:     marketID,
		AssetID:      tokenID,
		OriginalSize: qty,
		MatchedSize:  0,
		Price:        price,
		Tag:          tag,
	})
	return orderID
}

func (s *Strategy) placeCompletionBuy(marketID, tokenID string, price int, qty float64) string {
	return s.placeLimitBuy(marketID, tokenID, price, qty, OrderTagCompletion)
}

func (s *Strategy) placeMakerBuy(marketID, tokenID string, price int, qty float64) string {
	return s.placeLimitBuy(marketID, tokenID, price, qty, OrderTagMaker)
}

func (s *Strategy) placeMissingBuy(marketID string, tokenID string, neededSize float64, bestAsk *MarketOrder, timeLeft int64, avg float64) string {
	if bestAsk == nil || neededSize < PolymarketMinimumOrderSize {
		return ""
	}

	askPrice, askSize := bestAsk.Price, bestAsk.Size
	if askPrice > maxPriceForMissing(timeLeft, avg) {
		return ""
	}

	if askSize >= neededSize {
		return s.placeCompletionBuy(marketID, tokenID, askPrice, neededSize)

	}

	buySize := math.Min(neededSize-PolymarketMinimumOrderSize, askSize)
	return s.placeCompletionBuy(marketID, tokenID, askPrice, buySize)
}

func (s *Strategy) cancelSide(tokenID, tag string) error {
	orderIds := GetOrderIDsByAssetAndTag(tokenID, tag)
	if len(orderIds) == 0 {
		return nil
	}
	if err := s.executor.CancelOrders(orderIds); err != nil {
		log.Printf("cancel order failed: orderID=%s err=%v", orderIds, err)
		return err
	}
	return nil
}

func (s *Strategy) syncBids(marketID, tokenID string, highestBid int, timeLeft int64) {
	if highestBid < MinPrice {
		s.cancelSide(tokenID, OrderTagMaker)
		return
	}

	levelSize := getLevels(highestBid)
	desired := make(map[int]float64, len(levelSize))
	for i, size := range levelSize {
		price := highestBid - i
		if price >= MinPrice && quoteDepthAllowed(tokenID, price, timeLeft) {
			desired[price] = size
		}
	}
	if len(desired) == 0 {
		s.cancelSide(tokenID, OrderTagMaker)
		return
	}

	existing := GetOrdersByAssetAndTag(tokenID, OrderTagMaker)
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
	}

	for price, size := range desired {
		order := existing[price]
		if order != nil && math.Abs(order.OriginalSize-size) < eps {
			continue
		}
		s.placeMakerBuy(marketID, tokenID, price, size)
	}
}

func (s *Strategy) syncCompletionOrders(
	marketID string,
	upToken string,
	downToken string,
	unmatchedUp float64,
	unmatchedDown float64,
	timeLeft int64,
	upAvg float64,
	downAvg float64,
	upBestAsk *MarketOrder,
	downBestAsk *MarketOrder,
) {
	upPrice, upQty, upDesired := desiredCompletionOrder(unmatchedDown, upBestAsk, timeLeft, upAvg)
	downPrice, downQty, downDesired := desiredCompletionOrder(unmatchedUp, downBestAsk, timeLeft, downAvg)

	s.syncCompletionSide(marketID, upToken, upDesired, upPrice, upQty)
	s.syncCompletionSide(marketID, downToken, downDesired, downPrice, downQty)
}

func desiredCompletionOrder(unmatched float64, bestAsk *MarketOrder, timeLeft int64, avg float64) (int, float64, bool) {
	if unmatched <= 0 || bestAsk == nil {
		return 0, 0, false
	}
	maxPrice := maxPriceForMissing(timeLeft, avg)
	if bestAsk.Price > maxPrice {
		return 0, 0, false
	}
	return bestAsk.Price, math.Min(unmatched, bestAsk.Size), true
}

func (s *Strategy) syncCompletionSide(marketID string, tokenID string, desired bool, desiredPrice int, desiredQty float64) {
	existing := GetOrdersByAssetAndTag(tokenID, OrderTagCompletion)
	cancels := make([]string, 0, len(existing))
	keepOrder := ""

	for _, order := range existing {
		if !desired || order.Price != desiredPrice || order.OriginalSize != desiredQty {
			cancels = append(cancels, order.ID)
			continue
		}

		if keepOrder == "" {
			keepOrder = order.ID
		} else {
			cancels = append(cancels, order.ID)
		}
	}

	if len(cancels) > 0 {
		if err := s.executor.CancelOrders(cancels); err != nil {
			log.Printf("cancel completion order failed: orderID=%s err=%v", cancels, err)
			return
		}
	}

	if keepOrder == "" && desired {
		s.placeCompletionBuy(marketID, tokenID, desiredPrice, desiredQty)
	}
}

var Level1 = []float64{5, 5, 5, 5}
var Level2 = []float64{10, 10, 10, 10}
var Level3 = []float64{20, 20, 20, 20}

func getLevels(price int) []float64 {
	return Level2

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

func circuitDelta(timeLeft int64) float64 {
	switch {
	case timeLeft > 10*60:
		return 5
	case timeLeft > 5*60:
		return 3
	}

	return 0
}

func maxPriceForMissing(timeLeft int64, avg float64) int {
	return MaxPrice - discountTarget(timeLeft) - int(math.Ceil(avg))
}

func quoteDepthAllowed(tokenID string, price int, tleft int64) bool {
	return true
}
