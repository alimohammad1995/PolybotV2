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
	passSeconds := marketInfo.EndDateTS - now
	elapsedSeconds := now - marketInfo.StartDateTS

	if passSeconds <= 0 || elapsedSeconds <= MinimumStartWaitingSec {
		return
	}

	upToken := marketInfo.ClobTokenIDs[0]
	downToken := marketInfo.ClobTokenIDs[1]

	upBestBidAsk := GetBestBidAsk(upToken)
	downBestBidAsk := GetBestBidAsk(downToken)

	if upBestBidAsk[0] == nil || upBestBidAsk[1] == nil || downBestBidAsk[0] == nil || downBestBidAsk[1] == nil {
		return
	}

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
	plan := TradingDecision(state, book, int(elapsedSeconds), openOrdersByTag)
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

type Phase int

const (
	PhaseEarly Phase = iota
	PhaseMiddle
	PhaseLate
)

type Params struct {
	phase          Phase
	edgeTarget     int
	maxLossPerPair float64
	imbMax         float64
	baseSize       float64
}

func GetParams(elapsedSeconds int) Params {
	switch {
	case elapsedSeconds < 300:
		return Params{
			phase:          PhaseEarly,
			edgeTarget:     2,
			maxLossPerPair: 2.0,
			imbMax:         8,
			baseSize:       15,
		}
	case elapsedSeconds < 600:
		return Params{
			phase:          PhaseMiddle,
			edgeTarget:     1,
			maxLossPerPair: 3.0,
			imbMax:         5,
			baseSize:       10,
		}
	default:
		return Params{
			phase:          PhaseLate,
			edgeTarget:     0,
			maxLossPerPair: 5.0,
			imbMax:         3,
			baseSize:       5,
		}
	}
}

func Urgency(elapsedSeconds int) float64 {
	if elapsedSeconds <= 600 {
		return 0
	}
	return clampFloat((float64(elapsedSeconds)-600.0)/300.0, 0, 1)
}

func ComputePairedBids(book OrderBook, edgeTarget int, imbalance, imbMax float64) (int, int) {
	maxUp := maxInt(1, book.up.bestAsk-TickCents)
	maxDown := maxInt(1, book.down.bestAsk-TickCents)

	bU := minInt(book.up.bestBid+TickCents, maxUp)
	bD := minInt(book.down.bestBid+TickCents, maxDown)

	skew := imbalance / math.Max(imbMax, 1.0)
	skew = clampFloat(skew, -1.0, 1.0)

	skewTicks := int(math.Round(3.0 * skew))

	bU -= skewTicks
	bD += skewTicks

	bU = clampInt(bU, 1, maxUp)
	bD = clampInt(bD, 1, maxDown)

	sumCap := PayoutCents - edgeTarget
	sum := bU + bD

	if sum > sumCap {
		excess := sum - sumCap
		dropU := (excess + 1) / 2
		dropD := excess / 2

		bU -= dropU
		bD -= dropD
	}

	bU = clampInt(bU, 1, maxUp)
	bD = clampInt(bD, 1, maxDown)

	if bU+bD >= PayoutCents {
		bU = maxInt(1, bU-TickCents)
		bD = maxInt(1, bD-TickCents)
	}

	return bU, bD
}

func MaxHedgePrice(deficitSide OrderSide, book OrderBook, edgeTarget int) int {
	if deficitSide == SideDown {
		refUp := minInt(book.up.bestBid+TickCents, maxInt(1, book.up.bestAsk-TickCents))
		maxP := PayoutCents - edgeTarget - refUp
		return clampInt(maxP, 1, book.down.bestAsk)
	}
	refDown := minInt(book.down.bestBid+TickCents, maxInt(1, book.down.bestAsk-TickCents))
	maxP := PayoutCents - edgeTarget - refDown
	return clampInt(maxP, 1, book.up.bestAsk)
}

func TradingDecision(state State, book OrderBook, elapsedSeconds int, openOrdersByTag map[string][]*Order) Plan {
	params := GetParams(elapsedSeconds)

	uQ := state.upQty
	dQ := state.downQty
	imbalance := uQ - dQ
	absImb := math.Abs(imbalance)

	totalCost := uQ*state.upAvgCents + dQ*state.downAvgCents

	matchedQty := math.Min(uQ, dQ)

	currentPnL := matchedQty*float64(PayoutCents) - totalCost

	projectPnL := func(side OrderSide, price int, qty float64) float64 {
		newUpQty, newDownQty := uQ, dQ
		newCost := totalCost + float64(price)*qty

		if side == SideUp {
			newUpQty += qty
		} else {
			newDownQty += qty
		}

		newMatched := math.Min(newUpQty, newDownQty)
		return newMatched*float64(PayoutCents) - newCost
	}

	hedgeNowPnL := func() float64 {
		if floatEq(uQ, dQ) {
			return currentPnL
		}

		if floatGt(uQ, dQ) {
			need := uQ - dQ
			return projectPnL(SideDown, book.down.bestAsk, need)
		}

		need := dQ - uQ
		return projectPnL(SideUp, book.up.bestAsk, need)
	}

	riskBudget := -10.0

	switch params.phase {
	case PhaseEarly:
		riskBudget = -20.0
	case PhaseMiddle:
		riskBudget = -10.0
	case PhaseLate:
		riskBudget = -5.0
	}

	plan := Plan{cancelTags: []string{}, place: []DesiredOrder{}}

	if params.phase == PhaseLate && currentPnL > 0 {
		plan.cancelTags = append(plan.cancelTags, TagEdgeUp, TagEdgeDown, TagHedgeUp, TagHedgeDown, TagArbUp, TagArbDown)
		return ReconcilePlanWithOpenOrders(plan, openOrdersByTag)
	}

	hedgePnL := hedgeNowPnL()
	needsEmergencyHedge := false

	if absImb > 20 || (currentPnL < riskBudget && hedgePnL > currentPnL) {
		needsEmergencyHedge = true
	}

	if params.phase == PhaseLate && currentPnL < 0 && absImb > 5 {
		needsEmergencyHedge = true
	}

	if needsEmergencyHedge {
		plan.cancelTags = append(plan.cancelTags, TagEdgeUp, TagEdgeDown, TagArbUp, TagArbDown)

		deficitSide := SideDown
		hedgeTag := TagHedgeDown
		bestAsk := book.down.bestAsk
		bestBid := book.down.bestBid

		if floatLt(imbalance, 0) {
			deficitSide = SideUp
			hedgeTag = TagHedgeUp
			bestAsk = book.up.bestAsk
			bestBid = book.up.bestBid
		}

		otherSideAvg := state.downAvgCents
		if deficitSide == SideDown {
			otherSideAvg = state.upAvgCents
		}

		maxPrice := float64(PayoutCents) - otherSideAvg

		targetPrice := bestBid + TickCents

		urgency := Urgency(elapsedSeconds)
		if params.phase == PhaseLate || currentPnL < riskBudget*1.5 {
			urgency = math.Max(urgency, 0.5)
		}

		targetPrice = int(math.Round((1.0-urgency)*float64(targetPrice) + urgency*float64(bestAsk)))

		targetPrice = clampInt(targetPrice, bestBid+TickCents, minInt(bestAsk, int(maxPrice)))

		need := absImb
		maxHedgeSize := 10.0
		if params.phase == PhaseLate {
			maxHedgeSize = 15.0
		}

		size := math.Min(need, maxHedgeSize)

		projectedAfterHedge := projectPnL(deficitSide, targetPrice, size)
		if projectedAfterHedge > currentPnL || params.phase == PhaseLate {
			plan.place = append(plan.place, DesiredOrder{
				side:  deficitSide,
				price: targetPrice,
				size:  size,
				tag:   hedgeTag,
			})
		}

		return ReconcilePlanWithOpenOrders(plan, openOrdersByTag)
	}

	if absImb > 3 && absImb <= params.imbMax {
		deficitSide := SideDown
		if floatLt(imbalance, 0) {
			deficitSide = SideUp
		}

		bestBid := book.down.bestBid
		if deficitSide == SideUp {
			bestBid = book.up.bestBid
		}

		balancePrice := bestBid + TickCents
		balanceSize := math.Min(absImb/2, 8.0)

		projectedBalance := projectPnL(deficitSide, balancePrice, balanceSize)

		if projectedBalance >= currentPnL*0.95 {
			plan.cancelTags = append(plan.cancelTags, TagEdgeUp, TagEdgeDown, TagArbUp, TagArbDown)

			hedgeTag := TagHedgeDown
			if deficitSide == SideUp {
				hedgeTag = TagHedgeUp
			}

			plan.place = append(plan.place, DesiredOrder{
				side:  deficitSide,
				price: balancePrice,
				size:  balanceSize,
				tag:   hedgeTag,
			})

			return ReconcilePlanWithOpenOrders(plan, openOrdersByTag)
		}
	}

	plan.cancelTags = append(plan.cancelTags, TagHedgeUp, TagHedgeDown, TagArbUp, TagArbDown)

	bU, bD := ComputePairedBids(book, params.edgeTarget, imbalance, params.imbMax)

	sizeU := params.baseSize
	sizeD := params.baseSize

	if floatGt(imbalance, 0) {
		sizeD += math.Min(2.0, absImb*0.5)
		sizeU = math.Max(1.0, sizeU-1.0)
	} else if floatLt(imbalance, 0) {
		sizeU += math.Min(2.0, absImb*0.5)
		sizeD = math.Max(1.0, sizeD-1.0)
	}

	if params.phase == PhaseLate {
		sizeU = math.Min(sizeU, 2.0)
		sizeD = math.Min(sizeD, 2.0)

		if currentPnL > -2.0 {
			sizeU = 1.0
			sizeD = 1.0
		}
	}

	projectedUp := projectPnL(SideUp, bU, sizeU)
	projectedDown := projectPnL(SideDown, bD, sizeD)

	if absImb > 2 {
		if floatGt(imbalance, 0) {
			plan.cancelTags = append(plan.cancelTags, TagEdgeUp)
			if projectedDown >= currentPnL*0.98 || params.phase == PhaseEarly {
				plan.place = append(plan.place, DesiredOrder{
					side:  SideDown,
					price: bD,
					size:  sizeD,
					tag:   TagEdgeDown,
				})
			}
		} else {
			plan.cancelTags = append(plan.cancelTags, TagEdgeDown)
			if projectedUp >= currentPnL*0.98 || params.phase == PhaseEarly {
				plan.place = append(plan.place, DesiredOrder{
					side:  SideUp,
					price: bU,
					size:  sizeU,
					tag:   TagEdgeUp,
				})
			}
		}
		return ReconcilePlanWithOpenOrders(plan, openOrdersByTag)
	}

	if projectedUp >= currentPnL*0.98 || params.phase == PhaseEarly {
		plan.place = append(plan.place, DesiredOrder{
			side:  SideUp,
			price: bU,
			size:  sizeU,
			tag:   TagEdgeUp,
		})
	}

	if projectedDown >= currentPnL*0.98 || params.phase == PhaseEarly {
		plan.place = append(plan.place, DesiredOrder{
			side:  SideDown,
			price: bD,
			size:  sizeD,
			tag:   TagEdgeDown,
		})
	}

	return ReconcilePlanWithOpenOrders(plan, openOrdersByTag)
}

func ReconcilePlanWithOpenOrders(plan Plan, openOrdersByTag map[string][]*Order) Plan {
	cancelSet := make(map[string]bool)
	for _, tag := range plan.cancelTags {
		if len(openOrdersByTag[tag]) > 0 {
			cancelSet[tag] = true
		}
	}

	finalPlaces := make([]DesiredOrder, 0, len(plan.place))
	for _, desired := range plan.place {
		existing := openOrdersByTag[desired.tag]
		if len(existing) == 0 {
			finalPlaces = append(finalPlaces, desired)
			continue
		}

		if len(existing) == 1 && isOrderClose(existing[0], desired) {
			continue
		}

		cancelSet[desired.tag] = true
		finalPlaces = append(finalPlaces, desired)
	}

	cancelTags := make([]string, 0, len(cancelSet))
	for tag := range cancelSet {
		cancelTags = append(cancelTags, tag)
	}

	return Plan{cancelTags: cancelTags, place: finalPlaces}
}

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
		if size <= 0 {
			continue
		}

		s.placeLimitBuy(marketID, tokenID, order.price, size, order.tag)
	}
}

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

func normalizeOrderSize(size float64) float64 {
	if size <= 0 {
		return 0
	}
	return math.Max(size, PolymarketMinimumOrderSize)
}

func isOrderClose(order *Order, desired DesiredOrder) bool {
	if order == nil {
		return false
	}
	if intAbs(order.Price-desired.price) >= TickCents {
		return false
	}
	return math.Abs(order.OriginalSize-desired.size) < 0.5
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
