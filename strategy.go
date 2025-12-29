package main

import (
	"fmt"
	"math"
)

const (
	minQty = 5

	timeLeftForPositivePNLStop = 3 * 60
	profitFloorCents           = 4

	deadImbalance = 5
	softImbalance = 20
	hardImbalance = 80

	ladderStep = 1
)

var ladderSizes = []float64{5, 5, 5, 5}

func TradingDecision(
	state *State,
	book *OrderBook,
	timeLeft int,
	openOrdersByTag map[string][]*Order,
	upToken, downToken string,
) *Plan {
	pendingUp := 0.0
	pendingDown := 0.0

	net := (state.upQty + pendingUp) - (state.downQty + pendingDown)
	absNet := math.Abs(net)

	desired := make([]*DesiredOrder, 0, 16)

	closeOrder := tryProfitableClose(state, book, net, absNet)
	if closeOrder != nil {
		desired = append(desired, closeOrder)
		return reconcileOrders(desired, openOrdersByTag)
	}

	if timeLeft <= timeLeftForPositivePNLStop && state.pnl() > 0 {
		return reconcileOrders(nil, openOrdersByTag)
	}

	bidUp, bidDown := calculateMakerBids(book, state, timeLeft)

	if bidUp > 0 {
		desired = append(desired, buildLadder(SideUp, bidUp)...)
	}
	if bidDown > 0 {
		desired = append(desired, buildLadder(SideDown, bidDown)...)
	}

	return reconcileOrders(desired, openOrdersByTag)
}

func calculateMakerBids(book *OrderBook, state *State, timeLeft int) (int, int) {
	askUp := book.up.bestAsk
	askDown := book.down.bestAsk
	bidUp := book.up.bestBid
	bidDown := book.down.bestBid

	targetBidUp := minInt(bidUp+1, askUp-1)
	targetBidDown := minInt(bidDown+1, askDown-1)

	targetBidUp = clampInt(targetBidUp, 1, askUp-1)
	targetBidDown = clampInt(targetBidDown, 1, askDown-1)

	skew := clampInt(int(math.Floor(math.Abs(state.upQty-state.downQty)/softImbalance)), 0, 3)
	if state.upQty > state.downQty {
		targetBidUp -= skew
		targetBidDown += skew
	} else {
		targetBidUp += skew
		targetBidDown -= skew
	}

	limit := PayoutCents - profitFloorCents
	sum := targetBidUp + targetBidDown
	if sum > limit {
		over := sum - limit

		if state.net() > 50 {
			targetBidUp = maxInt(1, targetBidUp-over)
		} else if state.net() < -50 {
			targetBidDown = maxInt(1, targetBidDown-over)
		} else {
			sh := (over + 1) / 2
			targetBidUp = maxInt(1, targetBidUp-sh)
			targetBidDown = maxInt(1, targetBidDown-(over-sh))
		}
	}

	targetBidUp = clampInt(targetBidUp, 0, askUp-1)
	targetBidDown = clampInt(targetBidDown, 0, askDown-1)

	return targetBidUp, targetBidDown
}

func reconcileOrders(desired []*DesiredOrder, openOrdersByTag map[string][]*Order) *Plan {
	desiredByTag := make(map[string]*DesiredOrder, len(desired))
	for _, d := range desired {
		desiredByTag[d.tag] = d
	}

	cancelByTag := make(map[string][]string)

	for tag, orders := range openOrdersByTag {
		want, ok := desiredByTag[tag]

		if !ok {
			for _, o := range orders {
				cancelByTag[tag] = append(cancelByTag[tag], o.ID)
			}
			continue
		}

		for _, o := range orders {
			if desiredByTag[tag] == nil {
				cancelByTag[tag] = append(cancelByTag[tag], o.ID)
				continue
			}

			if o.Price != want.price || o.Remaining() < want.size {
				cancelByTag[tag] = append(cancelByTag[tag], o.ID)
			} else {
				desiredByTag[tag] = nil
			}
		}
	}

	newDesired := make([]*DesiredOrder, 0, len(desired))
	for _, d := range desiredByTag {
		if d != nil {
			newDesired = append(newDesired, d)
		}
	}

	for tag, ids := range cancelByTag {
		cancelByTag[tag] = dedupeStrings(ids)
	}

	return &Plan{cancelByTag: cancelByTag, place: newDesired}
}

func buildLadder(side OrderSide, topBid int) []*DesiredOrder {
	out := make([]*DesiredOrder, 0, len(ladderSizes))

	for level := 0; level < len(ladderSizes); level++ {
		price := topBid - level*ladderStep
		if price < 1 {
			continue
		}
		out = append(out, &DesiredOrder{
			side:  side,
			price: price,
			size:  ladderSizes[level],
			tag:   fmt.Sprintf("%s_L%d", side, level),
		})
	}
	return out
}

func tryProfitableClose(state *State, book *OrderBook, net, absNet float64) *DesiredOrder {
	if absNet < minQty {
		return nil
	}

	var needSide OrderSide
	var ask int
	var tag string

	if net < 0 {
		needSide = SideUp
		ask = book.up.bestAsk
		tag = "CLOSE_UP"
	} else if net > 0 {
		needSide = SideDown
		ask = book.down.bestAsk
		tag = "CLOSE_DOWN"
	}

	size := math.Min(absNet, 50)

	before := state.pnl()
	after := state.simulate(needSide, ask, size)

	if after > before {
		return &DesiredOrder{
			side:  needSide,
			price: ask,
			size:  size,
			tag:   tag,
		}
	}

	return nil
}
