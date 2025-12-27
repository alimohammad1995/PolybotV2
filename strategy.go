package main

import (
	"Polybot/polymarket"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"
)

const Workers = 5

const (
	PayoutCents = 100
)

// ============================================================================
// Strategy runner
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

// ============================================================================
// Types
// ============================================================================

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
	place       []DesiredOrder
	cancelByTag map[string][]string
}

// ============================================================================
// Strategy knobs (simple + effective)
// ============================================================================

const (
	minQty = 5

	// Require maker quotes to allow pair profit:
	// sum of the two maker bids must be <= 99 (profitFloor=1)
	profitFloorCents = 5
	pairSumLimit     = PayoutCents - profitFloorCents // 99

	// inventory control
	softImbalance = 20 // start leaning
	hardImbalance = 80 // consider closing at ask

	// cancel/replace only if price moved enough (avoid churn)
	requoteDelta = 1

	ladderStep = 1

	// closing behavior
	closeMaxQty       = 50
	closeNearEndSec   = 5 * 60
	closeForceMaxCost = 99 // keep within [1..99] anyway
)

var ladderSizesBalanced = []float64{8, 8, 8, 8}
var ladderSizesPassive = []float64{5, 5}

// ============================================================================
// Core PnL math
// ============================================================================

func minPnLCents(st State) float64 {
	pairs := math.Min(st.upQty, st.downQty)
	totalCost := st.upQty*st.upAvgCents + st.downQty*st.downAvgCents
	return pairs*PayoutCents - totalCost
}

func simulateMinPnLCents(st State, side OrderSide, price int, qty float64) float64 {
	if qty <= 0 {
		return minPnLCents(st)
	}
	if side == SideUp {
		newCost := st.upQty*st.upAvgCents + float64(price)*qty
		newQty := st.upQty + qty
		if newQty > 0 {
			st.upAvgCents = newCost / newQty
		}
		st.upQty = newQty
	} else {
		newCost := st.downQty*st.downAvgCents + float64(price)*qty
		newQty := st.downQty + qty
		if newQty > 0 {
			st.downAvgCents = newCost / newQty
		}
		st.downQty = newQty
	}
	return minPnLCents(st)
}

// ============================================================================
// Pricing: join bid + enforce pair-edge + inventory skew
// ============================================================================

func joinBid(bestBid, bestAsk int) int {
	// maker must be < ask
	if bestAsk <= 1 {
		return 1
	}
	if bestBid >= bestAsk {
		return bestAsk - 1
	}
	if bestBid <= 0 {
		return 1
	}
	return bestBid
}

func calculateMakerBidsV2(book OrderBook, net float64) (bidUp, bidDown int) {
	bidUp = joinBid(book.up.bestBid, book.up.bestAsk)
	bidDown = joinBid(book.down.bestBid, book.down.bestAsk)

	// Inventory skew: push quotes towards the side we need (short side)
	skew := clampInt(int(math.Abs(net)*2/softImbalance), 0, 4)
	if net > 0 {
		// long UP -> less aggressive UP, more aggressive DOWN
		bidUp -= skew
		bidDown += skew
	} else if net < 0 {
		bidUp += skew
		bidDown -= skew
	}

	// Clamp valid maker ranges
	bidUp = clampInt(bidUp, 1, maxInt(1, book.up.bestAsk-1))
	bidDown = clampInt(bidDown, 1, maxInt(1, book.down.bestAsk-1))

	// Profitability rule: don't quote both sides so high that pair becomes breakeven
	sum := bidUp + bidDown
	if sum > pairSumLimit {
		over := sum - pairSumLimit
		if net > 0 {
			bidUp = maxInt(1, bidUp-over)
		} else if net < 0 {
			bidDown = maxInt(1, bidDown-over)
		} else {
			half := (over + 1) / 2
			bidUp = maxInt(1, bidUp-half)
			bidDown = maxInt(1, bidDown-(over-half))
		}
	}

	bidUp = clampInt(bidUp, 1, maxInt(1, book.up.bestAsk-1))
	bidDown = clampInt(bidDown, 1, maxInt(1, book.down.bestAsk-1))
	return
}

// ============================================================================
// Decision + reconciliation
// ============================================================================

func TradingDecision(
	state State,
	book OrderBook,
	timeLeft int,
	openOrdersByTag map[string][]*Order,
	upToken, downToken string,
	_ *MarketTracker,
) Plan {
	if book.up.bestAsk <= 0 || book.down.bestAsk <= 0 {
		return Plan{cancelByTag: map[string][]string{}}
	}

	// Use ALL pending (not only L0), otherwise you oscillate & churn
	pendingUp := pendingQtyForToken(openOrdersByTag, upToken)
	pendingDown := pendingQtyForToken(openOrdersByTag, downToken)

	net := (state.upQty + pendingUp) - (state.downQty + pendingDown)
	absNet := math.Abs(net)

	bidUp, bidDown := calculateMakerBidsV2(book, net)

	desired := make([]DesiredOrder, 0, 16)

	// ---- CLOSE LOGIC (your snippet) ----
	// Use ask to rebalance only if it IMPROVES minPnL.
	// Trigger: hard imbalance OR near end (optional but helps stop tail losses).
	if absNet >= hardImbalance || timeLeft <= closeNearEndSec {
		needSide := SideUp
		ask := book.up.bestAsk
		if net > 0 {
			needSide = SideDown
			ask = book.down.bestAsk
		}

		ask = clampInt(ask, 1, closeForceMaxCost)
		size := math.Min(absNet, closeMaxQty)

		if size >= minQty && ask > 0 {
			before := minPnLCents(state)
			after := simulateMinPnLCents(state, needSide, ask, size)

			if after > before {
				tag := "CLOSE_UP"
				if needSide == SideDown {
					tag = "CLOSE_DOWN"
				}
				desired = append(desired, DesiredOrder{
					side:  needSide,
					price: ask,
					size:  size,
					tag:   tag,
				})
			}
		}
	}

	// ---- MAKER QUOTES ----
	// Balanced: quote both with full ladder.
	// Imbalanced: quote mainly the short side; keep long side passive (smaller + further back).
	if absNet <= softImbalance {
		desired = append(desired, buildLadderWithSizes(SideUp, bidUp, ladderSizesBalanced, 0)...)
		desired = append(desired, buildLadderWithSizes(SideDown, bidDown, ladderSizesBalanced, 0)...)
	} else {
		if net > 0 {
			// long UP -> focus on DOWN
			desired = append(desired, buildLadderWithSizes(SideDown, bidDown, ladderSizesBalanced, 0)...)
			desired = append(desired, buildLadderWithSizes(SideUp, bidUp-2, ladderSizesPassive, 2)...)
		} else {
			// long DOWN -> focus on UP
			desired = append(desired, buildLadderWithSizes(SideUp, bidUp, ladderSizesBalanced, 0)...)
			desired = append(desired, buildLadderWithSizes(SideDown, bidDown-2, ladderSizesPassive, 2)...)
		}
	}

	return reconcileOrdersSimpleV2(desired, openOrdersByTag, book)
}

func buildLadderWithSizes(side OrderSide, topBid int, sizes []float64, extraBackoff int) []DesiredOrder {
	if topBid <= 0 {
		return nil
	}
	sideTag := "DOWN"
	if side == SideUp {
		sideTag = "UP"
	}

	out := make([]DesiredOrder, 0, len(sizes))
	for level := 0; level < len(sizes); level++ {
		price := topBid - extraBackoff - level*ladderStep
		if price < 1 {
			continue
		}
		out = append(out, DesiredOrder{
			side:  side,
			price: price,
			size:  sizes[level],
			tag:   fmt.Sprintf("%s_L%d", sideTag, level),
		})
	}
	return out
}

func reconcileOrdersSimpleV2(
	desired []DesiredOrder,
	openOrdersByTag map[string][]*Order,
	book OrderBook,
) Plan {
	desiredByTag := make(map[string]DesiredOrder, len(desired))
	for _, d := range desired {
		desiredByTag[d.tag] = d
	}

	cancelByTag := make(map[string][]string)

	for tag, orders := range openOrdersByTag {
		want, ok := desiredByTag[tag]
		if !ok {
			for _, o := range orders {
				if o != nil && o.ID != "" {
					cancelByTag[tag] = append(cancelByTag[tag], o.ID)
				}
			}
			continue
		}

		isClose := strings.HasPrefix(tag, "CLOSE_")
		isUp := strings.HasPrefix(tag, "UP_") || tag == "CLOSE_UP"
		isDown := strings.HasPrefix(tag, "DOWN_") || tag == "CLOSE_DOWN"

		keepIdx := pickBestOrderIndex(orders, want.price)
		for i, o := range orders {
			if o == nil || o.ID == "" {
				continue
			}
			if i != keepIdx {
				cancelByTag[tag] = append(cancelByTag[tag], o.ID)
			}
		}

		if keepIdx < 0 || keepIdx >= len(orders) || orders[keepIdx] == nil {
			continue
		}
		cur := orders[keepIdx]

		// Maker validity: never sit at/above ask (unless close order).
		if !isClose && isUp && cur.Price >= book.up.bestAsk {
			cancelByTag[tag] = append(cancelByTag[tag], cur.ID)
			continue
		}
		if !isClose && isDown && cur.Price >= book.down.bestAsk {
			cancelByTag[tag] = append(cancelByTag[tag], cur.ID)
			continue
		}

		// Reduce churn: only requote if price moved meaningfully
		if intAbs(cur.Price-want.price) >= requoteDelta {
			cancelByTag[tag] = append(cancelByTag[tag], cur.ID)
			continue
		}
	}

	places := make([]DesiredOrder, 0, len(desired))
	for _, d := range desired {
		if d.price <= 0 || d.size < minQty {
			continue
		}
		existing := openOrdersByTag[d.tag]
		if hasActiveNonCanceled(existing, cancelByTag[d.tag]) {
			continue
		}
		places = append(places, d)
	}

	for tag, ids := range cancelByTag {
		cancelByTag[tag] = dedupeStrings(ids)
	}

	return Plan{cancelByTag: cancelByTag, place: places}
}

// ============================================================================
// Execution
// ============================================================================

func (s *Strategy) executePlan(marketID, upToken, downToken string, plan Plan) {
	for tag, ids := range plan.cancelByTag {
		if len(ids) == 0 {
			continue
		}
		if err := s.executor.CancelOrders(ids, tag); err != nil {
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
// Order helpers
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
		o := Orders[id]
		if o == nil || o.Tag == "" {
			continue
		}
		out[o.Tag] = append(out[o.Tag], o)
	}
	return out
}

func pendingQtyForToken(openOrdersByTag map[string][]*Order, tokenID string) float64 {
	total := 0.0
	for _, orders := range openOrdersByTag {
		for _, o := range orders {
			if o == nil || o.AssetID != tokenID {
				continue
			}
			rem := o.OriginalSize - o.MatchedSize
			if rem > 0 {
				total += rem
			}
		}
	}
	return total
}

func pickBestOrderIndex(orders []*Order, desiredPrice int) int {
	bestIdx := -1
	bestScore := int(^uint(0) >> 1)
	for i, o := range orders {
		if o == nil || o.ID == "" {
			continue
		}
		score := intAbs(o.Price - desiredPrice)
		if score < bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	return bestIdx
}

func hasActiveNonCanceled(orders []*Order, canceledIDs []string) bool {
	if len(orders) == 0 {
		return false
	}
	cset := make(map[string]bool, len(canceledIDs))
	for _, id := range canceledIDs {
		cset[id] = true
	}
	for _, o := range orders {
		if o == nil || o.ID == "" {
			continue
		}
		if !cset[o.ID] {
			return true
		}
	}
	return false
}

// ============================================================================
// Utils
// ============================================================================

func normalizeOrderSize(size float64) float64 {
	if size <= 0 {
		return 0
	}
	if size < PolymarketMinimumOrderSize {
		return PolymarketMinimumOrderSize
	}
	return size
}
