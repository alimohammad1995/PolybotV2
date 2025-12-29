package main

import (
	"fmt"
	"math"
)

const (
	minQty = 5

	profitFloorCents = 4

	deadImbalance = 5
	softImbalance = 10
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
	askUp := book.up.bestAsk
	askDown := book.down.bestAsk

	pendingUp := 0.0
	pendingDown := 0.0

	net := (state.upQty + pendingUp) - (state.downQty + pendingDown)
	absNet := math.Abs(net)

	desired := make([]*DesiredOrder, 0, 16)

	{
		var needSide OrderSide
		var ask int

		if net < 0 {
			needSide = SideUp
			ask = askUp
		} else if net > 0 {
			needSide = SideDown
			ask = askDown
		}

		if ask > 0 {
			size := math.Min(absNet, 50)
			if size >= minQty {
				before := state.pnl()
				after := state.simulate(needSide, ask, size)

				if after > before {
					tag := "CLOSE_UP"

					if needSide == SideDown {
						tag = "CLOSE_DOWN"
					}

					desired = append(desired, &DesiredOrder{
						side:  needSide,
						price: ask,
						size:  size,
						tag:   tag,
					})

					return reconcileOrdersSimple(desired, openOrdersByTag)
				}
			}
		}
	}

	bidUp, bidDown := calculateMakerBids(book, net)

	if bidUp > 0 {
		desired = append(desired, buildLadder(SideUp, bidUp)...)
	}
	if bidDown > 0 {
		desired = append(desired, buildLadder(SideDown, bidDown)...)
	}

	return reconcileOrdersSimple(desired, openOrdersByTag)
}

func calculateMakerBids(book *OrderBook, net float64) (int, int) {
	askUp := book.up.bestAsk
	askDown := book.down.bestAsk

	bidUp := minInt(book.up.bestBid+1, askUp-1)
	bidDown := minInt(book.down.bestBid+1, askDown-1)

	bidUp = clampInt(bidUp, 1, askUp-1)
	bidDown = clampInt(bidDown, 1, askDown-1)

	skew := clampInt(int(math.Floor(math.Abs(net)/softImbalance)), 0, 4)
	if net > 0 {
		bidUp -= skew
		bidDown += skew
	} else if net < 0 {
		bidUp += skew
		bidDown -= skew
	}

	limit := PayoutCents - profitFloorCents
	sum := bidUp + bidDown
	if sum > limit {
		over := sum - limit
		if net > 0 {
			bidUp = maxInt(1, bidUp-over)
		} else if net < 0 {
			bidDown = maxInt(1, bidDown-over)
		} else {
			sh := (over + 1) / 2
			bidUp = maxInt(1, bidUp-sh)
			bidDown = maxInt(1, bidDown-(over-sh))
		}
	}

	bidUp = clampInt(bidUp, 0, askUp-1)
	bidDown = clampInt(bidDown, 0, askDown-1)

	return bidUp, bidDown
}

func reconcileOrdersSimple(desired []*DesiredOrder, openOrdersByTag map[string][]*Order) *Plan {
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
			if o.Price != want.price {
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
