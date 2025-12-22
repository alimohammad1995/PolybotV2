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
	client   *PolymarketClient
	executor *OrderExecutor
	markets  chan string
	stateMu  sync.Mutex
	states   map[string]*MarketState
}

type MarketState struct {
	mu          sync.Mutex
	pausedUntil int64
	ordersYes   []string
	ordersNo    []string
}

func NewStrategy(client *PolymarketClient) *Strategy {
	strategy := &Strategy{
		client:   client,
		executor: NewOrderExecutor(client),
		markets:  make(chan string, Workers),
		states:   make(map[string]*MarketState),
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
	if marketInfo == nil || len(marketInfo.ClobTokenIDs) < 2 {
		return
	}

	state := s.getState(marketID)
	now := time.Now().Unix()
	tleft := marketInfo.EndDateTS - now
	if tleft <= 0 {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	if now < state.pausedUntil {
		return
	}

	yesToken := marketInfo.ClobTokenIDs[0]
	noToken := marketInfo.ClobTokenIDs[1]

	yesQty, yesAvg, _ := GetAssetPosition(yesToken)
	noQty, noAvg, _ := GetAssetPosition(noToken)

	pairs := math.Min(yesQty, noQty)
	unmatchedYes := yesQty - pairs
	unmatchedNo := noQty - pairs

	askYes := bestAsk(yesToken)
	askNo := bestAsk(noToken)

	if unmatchedYes > 0 {
		maxNo := maxPriceForMissing("NO", tleft, yesAvg, noAvg)
		if validAsk(askNo) && askNo <= maxNo {
			qty := minFloat(unmatchedYes, availableAtAsk(noToken, askNo))
			if qty > 0 {
				s.placeLimitBuy(marketID, noToken, askNo, qty)
			}
		}
	}

	if unmatchedNo > 0 {
		maxYes := maxPriceForMissing("YES", tleft, yesAvg, noAvg)
		if validAsk(askYes) && askYes <= maxYes {
			qty := minFloat(unmatchedNo, availableAtAsk(yesToken, askYes))
			if qty > 0 {
				s.placeLimitBuy(marketID, yesToken, askYes, qty)
			}
		}
	}

	allowYes := true
	allowNo := true

	if unmatchedYes >= MaxUnmatched {
		allowYes = false
	}
	if unmatchedNo >= MaxUnmatched {
		allowNo = false
	}

	if !canStartNewUnmatched(tleft) {
		if unmatchedYes > 0 {
			allowYes = false
			allowNo = true
		} else if unmatchedNo > 0 {
			allowNo = false
			allowYes = true
		} else {
			allowYes = false
			allowNo = false
		}
	}

	baseYes := bestBid(yesToken)
	baseNo := bestBid(noToken)

	if allowYes && validBaseBid(baseYes) {
		for level := 0; level < LadderLevels; level++ {
			price := baseYes - level*LadderStep
			qty := LevelSize[level]
			if price >= MinPrice && quoteDepthAllowed("YES", price, tleft) {
				s.ensureBid(state, "YES", level, marketID, yesToken, price, qty)
			} else {
				s.cancelLevel(state, "YES", level)
			}
		}
	} else {
		s.cancelSide(state, "YES")
	}

	if allowNo && validBaseBid(baseNo) {
		for level := 0; level < LadderLevels; level++ {
			price := baseNo - level*LadderStep
			qty := LevelSize[level]
			if price >= MinPrice && quoteDepthAllowed("NO", price, tleft) {
				s.ensureBid(state, "NO", level, marketID, noToken, price, qty)
			} else {
				s.cancelLevel(state, "NO", level)
			}
		}
	} else {
		s.cancelSide(state, "NO")
	}
}

func (s *Strategy) getState(marketID string) *MarketState {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	state := s.states[marketID]
	if state == nil {
		state = &MarketState{
			ordersYes: make([]string, LadderLevels),
			ordersNo:  make([]string, LadderLevels),
		}
		s.states[marketID] = state
	}
	return state
}

func (s *Strategy) placeLimitBuy(marketID, tokenID string, price int, qty float64) string {
	if qty <= 0 {
		return ""
	}
	resp, err := s.executor.BuyLimit(tokenID, float64(price)/100.0, qty, polymarket.OrderTypeGTC)
	if err != nil {
		log.Printf("place buy failed: market=%s token=%s price=%d qty=%.4f err=%v", marketID, tokenID, price, qty, err)
		return ""
	}

	orderID := orderIDFromResponse(resp)
	if orderID != "" {
		AddOrder(&Order{
			ID:           orderID,
			MarketID:     marketID,
			AssetID:      tokenID,
			OriginalSize: qty,
			MatchedSize:  0,
			Price:        price,
		})
	}
	return orderID
}

func (s *Strategy) ensureBid(state *MarketState, outcome string, level int, marketID, tokenID string, price int, qty float64) {
	orderID := s.orderIDAtLevel(state, outcome, level)
	if orderID != "" && OrderMatches(orderID, price, qty) {
		return
	}
	if orderID != "" {
		s.cancelOrder(orderID)
	}

	newID := s.placeLimitBuy(marketID, tokenID, price, qty)
	s.setOrderIDAtLevel(state, outcome, level, newID)
}

func (s *Strategy) cancelLevel(state *MarketState, outcome string, level int) {
	orderID := s.orderIDAtLevel(state, outcome, level)
	if orderID == "" {
		return
	}
	s.cancelOrder(orderID)
	s.setOrderIDAtLevel(state, outcome, level, "")
}

func (s *Strategy) cancelSide(state *MarketState, outcome string) {
	for level := 0; level < LadderLevels; level++ {
		s.cancelLevel(state, outcome, level)
	}
}

func (s *Strategy) cancelOrder(orderID string) {
	if orderID == "" {
		return
	}
	if _, err := s.client.client.CancelOrders([]string{orderID}); err != nil {
		log.Printf("cancel order failed: orderID=%s err=%v", orderID, err)
		return
	}
	DeleteOrder(orderID)
}

func (s *Strategy) orderIDAtLevel(state *MarketState, outcome string, level int) string {
	if outcome == "YES" {
		return state.ordersYes[level]
	}
	return state.ordersNo[level]
}

func (s *Strategy) setOrderIDAtLevel(state *MarketState, outcome string, level int, orderID string) {
	if outcome == "YES" {
		state.ordersYes[level] = orderID
		return
	}
	state.ordersNo[level] = orderID
}

func orderIDFromResponse(resp any) string {
	if resp == nil {
		return ""
	}
	switch v := resp.(type) {
	case map[string]any:
		if id := sanitizeID(stringFromAny(v["id"])); id != "" {
			return id
		}
		if id := sanitizeID(stringFromAny(v["order_id"])); id != "" {
			return id
		}
		if id := sanitizeID(stringFromAny(v["orderID"])); id != "" {
			return id
		}
		if order, ok := v["order"].(map[string]any); ok {
			if id := sanitizeID(stringFromAny(order["id"])); id != "" {
				return id
			}
		}
	}
	return ""
}

func sanitizeID(id string) string {
	if id == "" || id == "<nil>" {
		return ""
	}
	return id
}

func discountTarget(tleft int64) float64 {
	switch {
	case tleft > 5*60:
		return DBase
	case tleft > 2*60:
		return DMid
	case tleft > 60:
		return DLate
	default:
		return DFinal
	}
}

func maxPriceForMissing(missingOutcome string, tleft int64, avgYes, avgNo float64) int {
	limit := float64(MaxPrice) - discountTarget(tleft)
	var max float64
	if missingOutcome == "NO" {
		max = limit - avgYes
	} else {
		max = limit - avgNo
	}
	return int(math.Floor(max))
}

func canStartNewUnmatched(tleft int64) bool {
	return tleft > StopNewUnmatchedSec
}

func quoteDepthAllowed(outcome string, price int, tleft int64) bool {
	return true
}

func bestBid(tokenID string) int {
	book := OrderBook[tokenID]
	if book == nil {
		return -1
	}
	return book.BestBid
}

func bestAsk(tokenID string) int {
	book := OrderBook[tokenID]
	if book == nil {
		return -1
	}
	if book.BestAsk < 0 || book.BestAsk >= len(book.Asks) {
		return -1
	}
	return book.BestAsk
}

func availableAtAsk(tokenID string, price int) float64 {
	book := OrderBook[tokenID]
	if book == nil || price < 0 || price >= len(book.Asks) {
		return 0
	}
	return book.Asks[price]
}

func validAsk(price int) bool {
	return price >= MinPrice && price <= MaxPrice
}

func validBaseBid(price int) bool {
	return price >= MinPrice && price < MaxBaseBid
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
