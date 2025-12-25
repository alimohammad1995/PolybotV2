package main

import (
	"Polybot/polymarket"
	"log"
	"math"
	"sync"
	"time"
)

type Strategy2 struct {
	executor  *OrderExecutor
	markets   chan string
	pending   map[string]bool
	pendingMu *sync.Mutex
}

func NewStrategy2(client *PolymarketClient) *Strategy {
	strategy := &Strategy{
		executor:  NewOrderExecutor(client),
		markets:   make(chan string, Workers),
		pending:   make(map[string]bool),
		pendingMu: &sync.Mutex{},
	}
	strategy.Run()
	return strategy
}

func (s *Strategy2) OnUpdate(assetID []string) {
	s.enqueueMarketsFromAssets(assetID)
}

func (s *Strategy2) Run() {
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

func (s *Strategy2) enqueueMarketsFromAssets(assetIDs []string) {
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

func (s *Strategy2) enqueueMarket(marketID string) {
	s.pendingMu.Lock()
	if s.pending[marketID] {
		s.pendingMu.Unlock()
		return
	}
	s.pending[marketID] = true
	s.pendingMu.Unlock()
	s.markets <- marketID
}

func (s *Strategy2) markDone(marketID string) {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	delete(s.pending, marketID)
}

func (s *Strategy2) handle(marketID string) {
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

	if pairs*100 >= (upCost+downCost)+circuitDelta(timeLeft)*100 {
		s.cancelSide(upToken, OrderTagMaker)
		s.cancelSide(downToken, OrderTagMaker)
		return
	}

	upBestBidAsk := GetBestBidAsk(upToken)
	downBestBidAsk := GetBestBidAsk(downToken)

	s.syncCompletionOrders(
		marketID,
		timeLeft,

		downToken,
		downQty,
		downAvg,
		downBestBidAsk[1],

		upToken,
		upQty,
		upAvg,
		upBestBidAsk[1],
	)

	if upBestBidAsk[0] == nil || downBestBidAsk[0] == nil {
		return
	}

	allowUpOrders := neededDownSize < MaxUnmatched
	allowDownOrders := neededUpSize < MaxUnmatched
	allowOrders := allowUpOrders && allowDownOrders

	if timeLeft <= StopNewUnmatchedSec {
		if neededDownSize > eps {
			allowUpOrders = false
		} else if neededUpSize > eps {
			allowDownOrders = false
		} else {
			allowUpOrders = false
			allowDownOrders = false
		}
	}

	if !allowOrders {
		s.cancelSide(downToken, OrderTagMaker)
		s.cancelSide(upToken, OrderTagMaker)
		return
	}

	if !allowDownOrders {
		s.cancelSide(downToken, OrderTagMaker)
	}
	if !allowUpOrders {
		s.cancelSide(upToken, OrderTagMaker)
	}
	if !allowUpOrders && !allowDownOrders {
		return
	}

	if upBestBidAsk[0].Price >= downBestBidAsk[0].Price {
		if allowUpOrders {
			s.syncBids(marketID, upToken, upBestBidAsk[0].Price, timeLeft)
		}

		if allowDownOrders {
			s.syncBids(marketID, downToken, downBestBidAsk[0].Price, timeLeft)
		}
	} else {
		if allowDownOrders {
			s.syncBids(marketID, downToken, downBestBidAsk[0].Price, timeLeft)
		}

		if allowUpOrders {
			s.syncBids(marketID, upToken, upBestBidAsk[0].Price, timeLeft)
		}
	}
}

func (s *Strategy2) placeLimitBuy(marketID, tokenID string, price int, qty float64, tag string) string {
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

func (s *Strategy2) placeCompletionBuy(marketID, tokenID string, price int, qty float64) string {
	return s.placeLimitBuy(marketID, tokenID, price, qty, OrderTagCompletion)
}

func (s *Strategy2) placeMakerBuy(marketID, tokenID string, price int, qty float64) string {
	return s.placeLimitBuy(marketID, tokenID, price, qty, OrderTagMaker)
}

func (s *Strategy2) cancelSide(tokenID, tag string) error {
	orderIds := GetOrderIDsByAssetAndTag(tokenID, tag)
	if len(orderIds) == 0 {
		return nil
	}
	if err := s.executor.CancelOrders(orderIds, "side"); err != nil {
		log.Printf("cancel order failed: orderID=%s err=%v", orderIds, err)
		return err
	}
	return nil
}

func (s *Strategy2) syncBids(marketID, tokenID string, highestBid int, timeLeft int64) {
	if highestBid < MinPrice {
		s.cancelSide(tokenID, OrderTagMaker)
		return
	}

	levelSize := getLevels()
	desired := make(map[int]float64, len(levelSize))
	for i, size := range levelSize {
		price := calculatePriceForBidding(highestBid, i, timeLeft)
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
		if err := s.executor.CancelOrders(cancels, "sync"); err != nil {
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

func (s *Strategy2) syncCompletionOrders(
	marketID string,
	timeLeft int64,

	downToken string,
	downQty float64,
	downAvg float64,
	downBestAsk *MarketOrder,

	upToken string,
	upQty float64,
	upAvg float64,
	upBestAsk *MarketOrder,
) {
	if math.Abs(downQty-upQty) < PolymarketMinimumOrderSize {
		return
	}

	totalCost := downQty*downAvg + upQty*upAvg

	if downQty > upQty {
		neededUpSize := downQty - upQty

		maxNewPriceFloat := (profitDelta(timeLeft)*downQty - totalCost) / neededUpSize
		maxNewPriceFloat = math.Ceil(maxNewPriceFloat)
		maxNewPrice := int(maxNewPriceFloat)

		upDesired := upBestAsk != nil && upBestAsk.Price <= maxNewPrice

		s.syncCompletionSide(marketID, upToken, upDesired, maxNewPrice, neededUpSize)
	} else {
		neededDownSize := upQty - downQty
		maxNewPriceFloat := (profitDelta(timeLeft)*upQty - totalCost) / neededDownSize
		maxNewPriceFloat = math.Ceil(maxNewPriceFloat)
		maxNewPrice := int(maxNewPriceFloat)

		downDesired := downBestAsk != nil && downBestAsk.Price >= maxNewPrice

		s.syncCompletionSide(marketID, downToken, downDesired, maxNewPrice, neededDownSize)
	}
}

func (s *Strategy2) syncCompletionSide(marketID string, tokenID string, desired bool, desiredPrice int, desiredQty float64) {
	existing := GetOrdersByAssetAndTag(tokenID, OrderTagCompletion)
	cancels := make([]string, 0, len(existing))
	keepOrder := ""

	desiredQty = math.Ceil(desiredQty)

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
		if err := s.executor.CancelOrders(cancels, "completion"); err != nil {
			log.Printf("cancel completion order failed: orderID=%s err=%v", cancels, err)
			return
		}
	}

	if keepOrder == "" && desired {
		s.placeCompletionBuy(marketID, tokenID, desiredPrice, desiredQty)
	}
}

func calculatePriceForBidding(highestBid int, index int, timeLeft int64) int {
	stepper := 1

	switch {
	case timeLeft > 10*60:
		stepper = 3
	case timeLeft > 5*60:
		stepper = 2
	}

	return highestBid - (highestBid % stepper) - index*stepper
}

var Level2 = []float64{10, 10, 10, 10}

func getLevels() []float64 {
	return Level2
}

func profitDelta(timeLeft int64) float64 {
	delta := DLoss

	switch {
	case timeLeft > 10*60:
		delta = DBase
	case timeLeft > 5*60:
		delta = DLate
	case timeLeft > 2*60:
		delta = DFinal
	}

	return float64(MaxPrice - delta)
}

func circuitDelta(timeLeft int64) float64 {
	switch {
	case timeLeft > 10*60:
		return 2
	case timeLeft > 5*60:
		return 1
	}

	return 0
}

func quoteDepthAllowed(tokenID string, price int, tleft int64) bool {
	return true
}
