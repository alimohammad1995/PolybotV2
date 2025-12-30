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

const (
	PayoutCents = 100
)

// ============================================================================
// Strategy runner (unchanged behavior, but rate-limited per market)
// ============================================================================

type Strategy struct {
	executor *OrderExecutor
	markets  chan string

	pending   map[string]bool
	pendingMu *sync.Mutex
}

func NewStrategy(client *PolymarketClient) *Strategy {
	s := &Strategy{
		executor:  NewOrderExecutor(client),
		markets:   make(chan string, Workers),
		pending:   make(map[string]bool),
		pendingMu: &sync.Mutex{},
	}
	s.Run()
	return s
}

func (s *Strategy) OnUpdate(assetID []string) {
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
	s.pending[marketID] = false
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
	timeLeftSec := int(marketInfo.EndDateTS - now)
	elapsedSec := int(now - marketInfo.StartDateTS)

	if timeLeftSec <= 0 || elapsedSec <= 10 {
		return
	}

	upToken := marketInfo.ClobTokenIDs[0]
	downToken := marketInfo.ClobTokenIDs[1]

	upBestBidAsk := GetBestBidAsk(upToken)
	downBestBidAsk := GetBestBidAsk(downToken)
	if upBestBidAsk[0] == nil || upBestBidAsk[1] == nil || downBestBidAsk[0] == nil || downBestBidAsk[1] == nil {
		return
	}

	if Mode == "local" {
		snapshotManager.Tick(fmt.Sprintf("%s", marketInfo.Slug), upBestBidAsk, downBestBidAsk)
		return
	}

	upQty, upAvg, _ := GetAssetPosition(upToken)
	downQty, downAvg, _ := GetAssetPosition(downToken)

	state := &State{
		upQty:        upQty,
		downQty:      downQty,
		upAvgCents:   upAvg,
		downAvgCents: downAvg,
	}

	book := &OrderBook{
		up: &OrderBookSide{
			bestBid:     upBestBidAsk[0].Price,
			bestBidSize: upBestBidAsk[0].Size,
			bestAsk:     upBestBidAsk[1].Price,
			bestAskSize: upBestBidAsk[1].Size,
		},
		down: &OrderBookSide{
			bestBid:     downBestBidAsk[0].Price,
			bestBidSize: downBestBidAsk[0].Size,
			bestAsk:     downBestBidAsk[1].Price,
			bestAskSize: downBestBidAsk[1].Size,
		},
	}

	openOrders := s.getOpenOrders(marketID, upToken, downToken)
	desiredOrders := TradingDecision(state, book, timeLeftSec, openOrders, upToken, downToken)
	plan := s.reconcileOrders(desiredOrders, openOrders)
	s.executePlan(marketID, upToken, downToken, plan)
}

type State struct {
	upQty        float64
	downQty      float64
	upAvgCents   float64
	downAvgCents float64
}

func (st *State) pnl() float64 {
	minQty := math.Min(st.upQty, st.downQty)
	totalCost := st.upQty*st.upAvgCents + st.downQty*st.downAvgCents
	return minQty*PayoutCents - totalCost
}

func (st *State) net() float64 {
	return st.upQty - st.downQty
}

func (st *State) absNet() float64 {
	return math.Abs(st.net())
}

func (st *State) simulate(side OrderSide, price int, qty float64) float64 {
	newState := st.clone()

	switch side {
	case SideUp:
		newState.upAvgCents = newState.upQty*newState.upAvgCents + float64(price)*qty
		newState.upQty += qty
	case SideDown:
		newState.downAvgCents = newState.downQty*newState.downAvgCents + float64(price)*qty
		newState.downQty += qty
	}

	return st.pnl()
}

func (st *State) clone() *State {
	return &State{
		upQty:        st.upQty,
		downQty:      st.downQty,
		upAvgCents:   st.upAvgCents,
		downAvgCents: st.downAvgCents,
	}
}

type OrderBookSide struct {
	bestBid     int
	bestBidSize float64
	bestAsk     int
	bestAskSize float64
}

type OrderBook struct {
	up   *OrderBookSide
	down *OrderBookSide
}

type OrderSide string

const (
	SideUp   OrderSide = "UP"
	SideDown           = "DOWN"
)

type DesiredOrder struct {
	side  OrderSide
	price int
	size  float64
	tag   string
}

type Plan struct {
	place     []*DesiredOrder
	cancelIDs []string
}

func (s *Strategy) reconcileOrders(desired []*DesiredOrder, openOrdersByTag map[OrderSide][]*Order) *Plan {
	desiresBySidePrice := make(map[string]bool, len(desired))
	openOrdersBySidePrice := make(map[string]bool, len(openOrdersByTag))

	for _, d := range desired {
		desiresBySidePrice[fmt.Sprintf("%s_%d", d.side, d.price)] = true
	}

	for side, orders := range openOrdersByTag {
		for _, o := range orders {
			openOrdersBySidePrice[fmt.Sprintf("%s_%d", side, o.Price)] = true
		}
	}

	cancelIDs := make([]string, 0, 20)
	for side, orders := range openOrdersByTag {
		for _, o := range orders {
			if !desiresBySidePrice[fmt.Sprintf("%s_%d", side, o.Price)] {
				cancelIDs = append(cancelIDs, o.ID)
			}
		}
	}

	newDesires := make([]*DesiredOrder, 0, len(desired))
	for _, d := range desired {
		if !openOrdersBySidePrice[fmt.Sprintf("%s_%d", d.side, d.price)] {
			newDesires = append(newDesires, d)
		}
	}

	return &Plan{place: newDesires, cancelIDs: cancelIDs}
}

func (s *Strategy) executePlan(marketID, upToken, downToken string, plan *Plan) {
	if err := s.executor.CancelOrders(plan.cancelIDs, "canceling"); err != nil {
		log.Printf("cancel order failed: market=%s err=%v", marketID, err)
	}

	marketIDs := make([]string, 0, len(plan.place))
	tokenIDs := make([]string, 0, len(plan.place))
	prices := make([]int, 0, len(plan.place))
	sizes := make([]float64, 0, len(plan.place))
	tags := make([]string, 0, len(plan.place))

	for _, order := range plan.place {
		tokenID := downToken
		if order.side == SideUp {
			tokenID = upToken
		}

		if order.price < 1 || order.price >= PayoutCents {
			continue
		}

		size := normalizeOrderSize(order.size)
		if size < PolymarketMinimumOrderSize {
			continue
		}

		marketIDs = append(marketIDs, marketID)
		tokenIDs = append(tokenIDs, tokenID)
		prices = append(prices, order.price)
		sizes = append(sizes, size)
		tags = append(tags, order.tag)
	}

	s.placeLimitBuy(marketIDs, tokenIDs, prices, sizes, tags)
}

func (s *Strategy) placeLimitBuy(marketID, tokenID []string, price []int, qty []float64, tag []string) {
	if len(marketID) == 0 {
		return
	}

	pricesFloat := make([]float64, len(price))
	for i, p := range price {
		pricesFloat[i] = float64(p) / 100.0
	}

	log.Printf("order submit: side=buy token=%s price=%d size=%.4f tag=%s", tokenID, price, qty, tag)

	orderIDs, err := s.executor.BuyLimits(tokenID, pricesFloat, qty, polymarket.OrderTypeGTC)
	if err != nil {
		log.Printf("place buy failed: market=%s token=%s price=%d qty=%.4f err=%v", marketID, tokenID, price, qty, err)
		return
	}

	for i, orderID := range orderIDs {
		AddOrder(&Order{
			ID:           orderID,
			MarketID:     marketID[i],
			AssetID:      tokenID[i],
			OriginalSize: qty[i],
			MatchedSize:  0,
			Price:        price[i],
			Tag:          tag[i],
		})
	}
}

func (s *Strategy) getOpenOrders(marketID string, upToken, downToken string) map[OrderSide][]*Order {
	ordersMu.RLock()
	defer ordersMu.RUnlock()

	set := MarketToOrderIDs[marketID]
	if len(set) == 0 {
		return map[OrderSide][]*Order{}
	}
	out := make(map[OrderSide][]*Order, len(set))
	for id := range set {
		o := Orders[id]
		if o.Remaining() <= 0.1 {
			continue
		}

		if o.AssetID == upToken {
			out[SideUp] = append(out[SideUp], o)
		} else if o.AssetID == downToken {
			out[SideDown] = append(out[SideDown], o)
		}
	}
	return out
}

func normalizeOrderSize(size float64) float64 {
	if size <= 0 {
		return 0
	}
	if size < PolymarketMinimumOrderSize {
		return PolymarketMinimumOrderSize
	}
	return size
}
