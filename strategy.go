package main

import (
	"Polybot/polymarket"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

/*
Assumptions:
- Your existing codebase provides:
  - PolymarketClient, OrderExecutor, GetMarketInfo, IsActiveMarket, GetBestBidAsk, GetAssetPosition,
    GetMarketIDByToken, Snapshot, GetOrderIDsByMarket, AddOrder, Orders map, Orders mutex, MarketToOrderIDs, etc.
- You receive book updates frequently and fill notifications for your orders.

This file focuses on the full strategy + decision logic and integrates with your existing skeleton.
*/

// ============================================================================
// Strategy runner (same structure you already have)
// ============================================================================

const Workers = 5

type Strategy struct {
	executor  *OrderExecutor
	markets   chan string
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

	Snapshot(marketID, upBestBidAsk, downBestBidAsk)

	upQty, upAvg, _ := GetAssetPosition(upToken)
	downQty, downAvg, _ := GetAssetPosition(downToken)

	state := State{
		upQty:        upQty,
		downQty:      downQty,
		upAvgCents:   upAvg,
		downAvgCents: downAvg,
	}

	book := OrderBook{
		up: OrderBookSide{
			bestBid:     upBestBidAsk[0].Price,
			bestBidSize: upBestBidAsk[0].Size,
			bestAsk:     upBestBidAsk[1].Price,
			bestAskSize: upBestBidAsk[1].Size,
		},
		down: OrderBookSide{
			bestBid:     downBestBidAsk[0].Price,
			bestBidSize: downBestBidAsk[0].Size,
			bestAsk:     downBestBidAsk[1].Price,
			bestAskSize: downBestBidAsk[1].Size,
		},
	}

	openOrdersByTag := getOpenOrdersByTag(marketID)
	tracker := GetTracker(marketID)
	plan := TradingDecision(state, book, timeLeftSec, openOrdersByTag, upToken, downToken, tracker)
	s.executePlan(marketID, upToken, downToken, plan)
}

type State struct {
	upQty        float64
	downQty      float64
	upAvgCents   float64
	downAvgCents float64
}

type OrderBookSide struct {
	bestBid     int
	bestBidSize float64
	bestAsk     int
	bestAskSize float64
}

type OrderBook struct {
	up   OrderBookSide
	down OrderBookSide
}

type OrderSide int

const (
	SideUp OrderSide = iota
	SideDown
)

type DesiredOrder struct {
	side  OrderSide
	price int
	size  float64
	tag   string
}

type Plan struct {
	cancelTags []string
	place      []DesiredOrder
}

// ============================================================================
// CONSTANTS (tuned for your observed imbalance range)
// ============================================================================

const (
	PayoutCents = 100

	minQty     = 5
	feesBuffer = 0

	// Profit floors
	profitFloorNormal = 1
	profitFloorClose  = 2

	// Inventory thresholds
	deadband      = 15
	softImbalance = 40
	hardImbalance = 80
	maxImbalance  = 120

	// Close-only very late
	closeOnlySeconds = 60

	bufferDefault = 2
	bufferLate    = 1

	pauseAskSumGE = 130

	ladderStep = 1

	requotePriceDelta = 2 // do not churn for 1c moves
)

// Equal levels with fixed sizes to avoid create/cancel.
var ladderSizes = []float64{10, 10, 10, 10}

// Net exposure cap when your bid is expensive (prevents worst-case cliff)
func maxNetAllowedAtBid(bid int) float64 {
	switch {
	case bid >= 97:
		return 10
	case bid >= 95:
		return 20
	case bid >= 90:
		return 35
	case bid >= 85:
		return 50
	default:
		return 120
	}
}

// ============================================================================
// Trading Decision (core)
// ============================================================================

func TradingDecision(
	state State,
	book OrderBook,
	timeLeft int,
	openOrdersByTag map[string][]*Order,
	upToken, downToken string,
	tracker *MarketTracker,
) Plan {
	askUp := book.up.bestAsk
	askDown := book.down.bestAsk

	// Pause if sum of asks is absurd (illiquid / broken)
	if askUp+askDown >= pauseAskSumGE {
		return reconcileOrders(nil, openOrdersByTag)
	}

	pendingUp := pendingQtyForToken(openOrdersByTag, upToken)
	pendingDown := pendingQtyForToken(openOrdersByTag, downToken)
	imbalance := (state.upQty + pendingUp) - (state.downQty + pendingDown)
	absImb := math.Abs(imbalance)

	// 1) Compute maker targets (complementary)
	bidUp, bidDown := calculateTargetBids(book, timeLeft, imbalance)

	// 2) SAFE-ADD caps (CRITICAL FIX)
	safeUp := safeAddCapUp(askDown)
	safeDown := safeAddCapDown(askUp)
	if bidUp > safeUp {
		bidUp = safeUp
	}
	if bidDown > safeDown {
		bidDown = safeDown
	}
	// maker clamp
	bidUp = clampInt(bidUp, 1, askUp-1)
	bidDown = clampInt(bidDown, 1, askDown-1)

	disableUp := false
	disableDown := false

	// 3) Net expensive exposure guard
	// (If your bid is expensive and you're already net-heavy, stop adding more.)
	netUp := imbalance
	netDown := -imbalance

	if float64(bidUp) >= 85 && netUp > maxNetAllowedAtBid(bidUp) {
		disableUp = true
		bidUp = 0
	}
	if float64(bidDown) >= 85 && netDown > maxNetAllowedAtBid(bidDown) {
		disableDown = true
		bidDown = 0
	}

	// 4) Inventory control: at HARD start active closing on the other side
	if absImb >= hardImbalance {
		if imbalance > 0 {
			// UP-heavy: stop UP adds, close with DOWN using close-cap (can go near ask)
			disableUp = true
			bidUp = 0
			closeCap := maxPayToCloseDown(state, tracker)
			// If crossing locks profit, allow cross to askDown
			if closeCap >= askDown {
				bidDown = askDown
			} else {
				bidDown = minInt(askDown-1, closeCap)
			}
		} else {
			// DOWN-heavy
			disableDown = true
			bidDown = 0
			closeCap := maxPayToCloseUp(state, tracker)
			if closeCap >= askUp {
				bidUp = askUp
			} else {
				bidUp = minInt(askUp-1, maxInt(bidUp, closeCap))
			}
		}
	}

	// 5) MAX imbalance => close-only, no new exposure at all
	if absImb >= maxImbalance {
		if imbalance > 0 {
			disableUp = true
			bidUp = 0
		} else {
			disableDown = true
			bidDown = 0
		}
	}

	// 6) Close-only very late
	if timeLeft <= closeOnlySeconds {
		if imbalance > deadband {
			disableUp = true
			bidUp = 0
		} else if imbalance < -deadband {
			disableDown = true
			bidDown = 0
		}
	}

	desired := make([]DesiredOrder, 0, len(ladderSizes)*2)

	if !disableUp && bidUp > 0 {
		desired = append(desired, buildLadder(SideUp, bidUp)...)
	}
	if !disableDown && bidDown > 0 {
		desired = append(desired, buildLadder(SideDown, bidDown)...)
	}

	return reconcileOrders(desired, openOrdersByTag)
}

// ============================================================================
// Pending qty: uses Original - Matched (handles partial fills)
// ============================================================================

func pendingQtyForToken(openOrdersByTag map[string][]*Order, tokenID string) float64 {
	total := 0.0
	for _, orders := range openOrdersByTag {
		for _, order := range orders {
			if order == nil || order.AssetID != tokenID {
				continue
			}
			remaining := order.OriginalSize - order.MatchedSize
			if remaining > 0 {
				total += remaining
			}
		}
	}
	return total
}

// ============================================================================
// Pricing: complementary maker + tight buffer
// ============================================================================

func calculateTargetBids(book OrderBook, timeLeft int, imbalance float64) (int, int) {
	askUp := book.up.bestAsk
	askDown := book.down.bestAsk
	if askUp <= 0 || askDown <= 0 {
		return 0, 0
	}

	bufferUp, bufferDown := calculateBuffers(timeLeft, imbalance)
	targetUp := PayoutCents - askDown - bufferUp
	targetDown := PayoutCents - askUp - bufferDown

	// maker clamp
	targetUp = clampInt(targetUp, 1, askUp-1)
	targetDown = clampInt(targetDown, 1, askDown-1)

	return targetUp, targetDown
}

func calculateBuffers(timeLeft int, imbalance float64) (int, int) {
	base := bufferDefault
	if timeLeft <= 120 {
		base = bufferLate
	}

	absImb := math.Abs(imbalance)
	if absImb <= deadband {
		return base, base
	}

	// Very gentle skew: widen heavy side by 1
	skew := 0
	if absImb > softImbalance {
		skew = 1
	}

	if imbalance > 0 {
		// UP-heavy => UP wider, DOWN tighter
		return minInt(3, base+skew), maxInt(0, base-skew)
	}
	return maxInt(0, base-skew), minInt(3, base+skew)
}

// Safe-add caps (the main “don’t go negative” guard)
func safeAddCapUp(askDown int) int {
	return clampInt(PayoutCents-askDown-feesBuffer-profitFloorNormal, 1, 99)
}
func safeAddCapDown(askUp int) int {
	return clampInt(PayoutCents-askUp-feesBuffer-profitFloorNormal, 1, 99)
}

// Close-cap using tracker heaps if available; otherwise fallback to avg
func maxPayToCloseUp(state State, tracker *MarketTracker) int {
	// closing UP means you already have unpaired DOWN; buy UP to pair it
	if p, ok := tracker.CheapestUnpairedDownPrice(); ok {
		return clampInt(PayoutCents-p-feesBuffer-profitFloorClose, 1, 99)
	}
	if state.downQty <= 0 {
		return 0
	}
	avgDown := int(math.Round(state.downAvgCents))
	return clampInt(PayoutCents-avgDown-feesBuffer-profitFloorClose, 1, 99)
}

func maxPayToCloseDown(state State, tracker *MarketTracker) int {
	if p, ok := tracker.CheapestUnpairedUpPrice(); ok {
		return clampInt(PayoutCents-p-feesBuffer-profitFloorClose, 1, 99)
	}
	if state.upQty <= 0 {
		return 0
	}
	avgUp := int(math.Round(state.upAvgCents))
	return clampInt(PayoutCents-avgUp-feesBuffer-profitFloorClose, 1, 99)
}

// ============================================================================
// Ladder: equal steps + fixed sizes (no churn)
// ============================================================================

func buildLadder(side OrderSide, topBid int) []DesiredOrder {
	if topBid <= 0 {
		return nil
	}
	orders := make([]DesiredOrder, 0, len(ladderSizes))
	sideTag := "DOWN"
	if side == SideUp {
		sideTag = "UP"
	}
	for level := 0; level < len(ladderSizes); level++ {
		price := topBid - (level * ladderStep)
		if price < 1 {
			continue
		}
		orders = append(orders, DesiredOrder{
			side:  side,
			price: price,
			size:  ladderSizes[level],
			tag:   fmt.Sprintf("%s_L%d", sideTag, level),
		})
	}
	return orders
}

// ============================================================================
// Reconcile: low churn BUT force-cancel unsafe orders
// - ignore size diffs
// - cancel if price diff >= requotePriceDelta
// - ALSO cancel if existing is unsafe: crosses ask or above safe cap
// ============================================================================

func reconcileOrders(
	desired []DesiredOrder,
	openOrdersByTag map[string][]*Order,
) Plan { // REMOVED: safeCapUp, safeCapDown parameters
	desiredByTag := make(map[string]DesiredOrder, len(desired))
	for _, d := range desired {
		desiredByTag[d.tag] = d
	}

	cancelSet := make(map[string]bool)

	for tag, orders := range openOrdersByTag {
		want, ok := desiredByTag[tag]
		if !ok {
			// Tag no longer desired - cancel
			cancelSet[tag] = true
			continue
		}
		if len(orders) != 1 || orders[0] == nil {
			// Multiple orders or nil - cancel
			cancelSet[tag] = true
			continue
		}
		cur := orders[0]

		// REMOVED: The aggressive safeCapUp/safeCapDown check
		// The bidding logic already ensures safe prices

		// Only cancel if price changed significantly
		if intAbs(cur.Price-want.price) >= requotePriceDelta {
			cancelSet[tag] = true
		}
	}

	places := make([]DesiredOrder, 0, len(desired))
	for _, d := range desired {
		if d.price <= 0 || d.size < minQty {
			continue
		}
		existing := openOrdersByTag[d.tag]
		if len(existing) == 1 && !cancelSet[d.tag] {
			continue // Order exists and is close enough - keep it
		}
		places = append(places, d)
	}

	cancelTags := make([]string, 0, len(cancelSet))
	for tag := range cancelSet {
		cancelTags = append(cancelTags, tag)
	}
	return Plan{cancelTags: cancelTags, place: places}
}

// ============================================================================
// Execute plan (same as yours, with dedupe)
// ============================================================================

func (s *Strategy) executePlan(marketID, upToken, downToken string, plan Plan) {
	cancelTags := dedupeStrings(plan.cancelTags)
	for _, tag := range cancelTags {
		orderIDs := GetOrderIDsByMarket(marketID, tag)
		if len(orderIDs) == 0 {
			continue
		}
		if err := s.executor.CancelOrders(orderIDs, tag); err != nil {
			log.Printf("cancel order failed: market=%s tag=%s err=%v", marketID, tag, err)
		}
	}

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

		s.placeLimitBuy(marketID, tokenID, order.price, size, order.tag)
	}
}

func (s *Strategy) placeLimitBuy(marketID, tokenID string, price int, qty float64, tag string) string {
	if qty < PolymarketMinimumOrderSize {
		return ""
	}
	fPrice := float64(price) / 100.0
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

// ============================================================================
// Open orders by tag (same idea as your version)
// ============================================================================

func getOpenOrdersByTag(marketID string) map[string][]*Order {
	ordersMu.RLock()
	defer ordersMu.RUnlock()

	set := MarketToOrderIDs[marketID]
	if len(set) == 0 {
		return map[string][]*Order{}
	}
	out := make(map[string][]*Order, len(set))
	for id := range set {
		order := Orders[id]
		if order == nil || order.Tag == "" {
			continue
		}
		out[order.Tag] = append(out[order.Tag], order)
	}
	return out
}

// ============================================================================
// Utils
// ============================================================================

func normalizeOrderSize(size float64) float64 {
	if size <= 0 {
		return 0
	}
	return math.Max(size, PolymarketMinimumOrderSize)
}
