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

	Snapshot(fmt.Sprintf("%d", marketInfo.StartDateTS), upBestBidAsk, downBestBidAsk)
	return

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
	plan := TradingDecision(state, book, timeLeftSec, openOrdersByTag, upToken, downToken)
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
// Engine state (per market) + config
// ============================================================================

type Engine struct {
	Locked     bool
	LastAction time.Time
}

var enginesMu sync.Mutex
var engines = map[string]*Engine{}

func getEngine(marketID string) *Engine {
	enginesMu.Lock()
	defer enginesMu.Unlock()
	e := engines[marketID]
	if e == nil {
		e = &Engine{}
		engines[marketID] = e
	}
	return e
}

const (
	minQty = 5

	// Price band ONLY for UNPAIRED/speculative buys (hedging deficit ignores this)
	MAX_PRICE_CAP   = 95
	MIN_PRICE_FLOOR = 8

	// Cooldown to avoid spamming orders on every WS tick
	minDecisionInterval = 250 * time.Millisecond

	// Lock behavior
	lockSec         = 150    // last 2.5 min -> lock
	lockProfitCents = 1000.0 // once worst-case >= +$10 -> lock
	closeFirstSec   = 240    // last 4 min -> prefer hedging

	// Hard caps
	maxCloseSize = 200.0
	maxPairSize  = 50.0
)

// ============================================================================
// Core PnL math
// ============================================================================

func minPnLCents(st State) float64 {
	totalCost := st.upQty*st.upAvgCents + st.downQty*st.downAvgCents
	return math.Min(st.upQty, st.downQty)*PayoutCents - totalCost
}

func bestPnLCents(st State) float64 {
	totalCost := st.upQty*st.upAvgCents + st.downQty*st.downAvgCents
	return math.Max(st.upQty, st.downQty)*PayoutCents - totalCost
}

func applyBuy(st State, side OrderSide, price int, qty float64) State {
	if qty <= 0 {
		return st
	}
	p := float64(price)
	if side == SideUp {
		newQty := st.upQty + qty
		if newQty > 0 {
			st.upAvgCents = (st.upQty*st.upAvgCents + p*qty) / newQty
			st.upQty = newQty
		}
		return st
	}
	newQty := st.downQty + qty
	if newQty > 0 {
		st.downAvgCents = (st.downQty*st.downAvgCents + p*qty) / newQty
		st.downQty = newQty
	}
	return st
}

// Unpaired = does NOT increase min(upQty, downQty)
func isUnpairedBuy(st State, side OrderSide) bool {
	if side == SideUp {
		return st.upQty >= st.downQty
	}
	return st.downQty >= st.upQty
}

// Max size you can buy at (price) while keeping worst-case >= minWorst.
// Correctly handles the pair-increasing region first.
func maxSizeKeepingWorst(st State, side OrderSide, price int, minWorst float64) float64 {
	if price <= 0 {
		return 0
	}
	p := float64(price)
	curWorst := minPnLCents(st)

	u := st.upQty
	d := st.downQty

	if side == SideUp {
		if u < d {
			x1 := d - u // pair-increasing amount
			if p >= 100 {
				return 0
			}
			wAfter := curWorst + (100.0-p)*x1
			if wAfter <= minWorst {
				return math.Floor(x1)
			}
			extra := (wAfter - minWorst) / p
			return math.Floor(x1 + extra)
		}
		// unpaired
		if curWorst <= minWorst {
			return 0
		}
		return math.Floor((curWorst - minWorst) / p)
	}

	// SideDown
	if d < u {
		x1 := u - d
		if p >= 100 {
			return 0
		}
		wAfter := curWorst + (100.0-p)*x1
		if wAfter <= minWorst {
			return math.Floor(x1)
		}
		extra := (wAfter - minWorst) / p
		return math.Floor(x1 + extra)
	}
	if curWorst <= minWorst {
		return 0
	}
	return math.Floor((curWorst - minWorst) / p)
}

// ============================================================================
// Phase params (simple, deterministic)
// ============================================================================

func phaseParams(timeLeft int) (minWorst float64, maxGap float64, baseSize float64) {
	// You can tune these 4 numbers and the whole bot behavior changes cleanly.
	switch {
	case timeLeft > 600: // early (accumulate)
		return -600, 120, 10
	case timeLeft > 300: // mid (balance)
		return -300, 60, 8
	case timeLeft > lockSec: // late (tighten)
		return -100, 25, 5
	default: // very late
		return 0, 15, 5
	}
}

// ============================================================================
// Decision: taker-first, floor-first
// Cancels legacy ladder tags so you don't keep old orders alive.
// ============================================================================

type candidate struct {
	score float64
	order *DesiredOrder
}

func TradingDecision(
	state State,
	book OrderBook,
	timeLeft int,
	openOrdersByTag map[string][]*Order,
	upToken, downToken string,
) Plan {
	empty := Plan{cancelByTag: map[string][]string{}}

	if book.up.bestAsk <= 0 || book.down.bestAsk <= 0 {
		return empty
	}

	pendingUp := pendingQtyForToken(openOrdersByTag, upToken)
	pendingDown := pendingQtyForToken(openOrdersByTag, downToken)

	imbEff := (state.upQty + pendingUp) - (state.downQty + pendingDown)
	imbFilled := state.upQty - state.downQty

	curPNL := minPnLCents(state)
	isLocked := timeLeft <= lockSec || curPNL >= lockProfitCents

	worstPNL, maxGap, baseSize := phaseParams(timeLeft)

	desired := make([]DesiredOrder, 0, 2)

	if isLocked {
		if math.Abs(imbFilled) >= minQty {
			if imbFilled > 0 {
				size := minFloat3(math.Abs(imbFilled), book.down.bestAskSize, maxCloseSize)
				size = math.Floor(size)
				if size >= minQty && book.down.bestAsk > 0 {
					desired = append(desired, DesiredOrder{side: SideDown, price: book.down.bestAsk, size: size, tag: "HEDGE_DOWN"})
				}
			} else {
				size := minFloat3(math.Abs(imbFilled), book.up.bestAskSize, maxCloseSize)
				size = math.Floor(size)
				if size >= minQty && book.up.bestAsk > 0 {
					desired = append(desired, DesiredOrder{side: SideUp, price: book.up.bestAsk, size: size, tag: "HEDGE_UP"})
				}
			}
		}
		plan := reconcileTakerOrders(desired, openOrdersByTag)
		return plan
	}

	closeFirst := timeLeft <= closeFirstSec
	if math.Abs(imbEff) > maxGap || curPNL < worstPNL || closeFirst {
		if math.Abs(imbFilled) >= minQty {
			need := math.Abs(imbEff) - maxGap
			if closeFirst && need < minQty {
				need = math.Min(math.Abs(imbFilled), baseSize)
			}
			need = math.Max(need, minQty)

			if imbFilled > 0 {
				// buy DOWN, <= imbalance to remain pair-increasing
				size := minFloat3(need, math.Abs(imbFilled), maxCloseSize)
				size = math.Min(size, book.down.bestAskSize)
				size = math.Floor(size)
				if size >= minQty && book.down.bestAsk > 0 {
					desired = append(desired, DesiredOrder{side: SideDown, price: book.down.bestAsk, size: size, tag: "HEDGE_DOWN"})
				}
			} else {
				size := minFloat3(need, math.Abs(imbFilled), maxCloseSize)
				size = math.Min(size, book.up.bestAskSize)
				size = math.Floor(size)
				if size >= minQty && book.up.bestAsk > 0 {
					desired = append(desired, DesiredOrder{side: SideUp, price: book.up.bestAsk, size: size, tag: "HEDGE_UP"})
				}
			}

			plan := reconcileTakerOrders(desired, openOrdersByTag)
			if closeFirst || len(plan.place) > 0 {
				return plan
			}
		} else if closeFirst {
			return reconcileTakerOrders(nil, openOrdersByTag)
		}
	}

	best := candidate{score: curPNL, order: nil}

	if c := makeAccumulateCandidate(state, book, SideUp, worstPNL, baseSize); c.order != nil && c.score > best.score {
		best = c
	}
	if c := makeAccumulateCandidate(state, book, SideDown, worstPNL, baseSize); c.order != nil && c.score > best.score {
		best = c
	}

	if best.order != nil {
		desired = append(desired, *best.order)
	}

	return reconcileTakerOrders(desired, openOrdersByTag)
}

func makeAccumulateCandidate(st State, book OrderBook, side OrderSide, minWorst float64, baseSize float64) candidate {
	var ask int
	var depth float64
	var tag string

	if side == SideUp {
		ask = book.up.bestAsk
		depth = book.up.bestAskSize
		tag = "TAKE_UP"
	} else {
		ask = book.down.bestAsk
		depth = book.down.bestAskSize
		tag = "TAKE_DOWN"
	}

	if ask <= 0 || depth < minQty {
		return candidate{score: -1e18, order: nil}
	}

	if isUnpairedBuy(st, side) {
		if ask > MAX_PRICE_CAP || ask < MIN_PRICE_FLOOR {
			return candidate{score: -1e18, order: nil}
		}
	}

	maxSafe := maxSizeKeepingWorst(st, side, ask, minWorst)
	size := minFloat3(baseSize, depth, maxSafe)
	size = math.Floor(size)
	if size < minQty {
		return candidate{score: -1e18, order: nil}
	}

	next := applyBuy(st, side, ask, size)
	w := minPnLCents(next)
	gap := math.Abs(next.upQty - next.downQty)

	score := (1.0 * w) - (0.15 * gap) + math.Abs(minWorst)
	fmt.Println(score)

	return candidate{
		score: score,
		order: &DesiredOrder{side: side, price: ask, size: size, tag: tag},
	}
}

func isManagedTag(tag string) bool {
	switch tag {
	case "PAIR_UP", "PAIR_DOWN", "HEDGE_UP", "HEDGE_DOWN", "TAKE_UP", "TAKE_DOWN":
		return true
	default:
		return false
	}
}

func reconcileTakerOrders(desired []DesiredOrder, openOrdersByTag map[string][]*Order) Plan {
	desiredByTag := make(map[string]DesiredOrder, len(desired))
	for _, d := range desired {
		desiredByTag[d.tag] = d
	}

	cancelByTag := make(map[string][]string)

	// Handle managed tags: keep if matches, else cancel & replace
	for tag, orders := range openOrdersByTag {
		if !isManagedTag(tag) {
			continue
		}
		want, ok := desiredByTag[tag]
		if !ok {
			// not desired anymore
			for _, o := range orders {
				if o != nil && o.ID != "" {
					cancelByTag[tag] = append(cancelByTag[tag], o.ID)
				}
			}
			continue
		}

		kept := false
		for _, o := range orders {
			if o == nil || o.ID == "" {
				continue
			}
			rem := o.OriginalSize - o.MatchedSize
			// keep if same price and still meaningful remaining
			if o.Price == want.price && rem >= want.size*0.5 {
				kept = true
				continue
			}
			cancelByTag[tag] = append(cancelByTag[tag], o.ID)
		}
		if kept {
			// don't place a new one for this tag
			delete(desiredByTag, tag)
		}
	}

	// Create orders in the same order as desired slice (deterministic)
	places := make([]DesiredOrder, 0, len(desiredByTag))
	for _, d := range desired {
		if _, ok := desiredByTag[d.tag]; !ok {
			continue
		}
		if d.price <= 0 || d.size < minQty {
			continue
		}
		places = append(places, d)
	}

	for tag, ids := range cancelByTag {
		cancelByTag[tag] = dedupeStrings(ids)
	}

	if cancelByTag == nil {
		cancelByTag = map[string][]string{}
	}

	return Plan{cancelByTag: cancelByTag, place: places}
}

func minFloat3(a, b, c float64) float64 {
	return math.Min(a, math.Min(b, c))
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
