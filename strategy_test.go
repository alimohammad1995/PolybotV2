package main

import (
	"fmt"
	"testing"
)

func TestMakeAccumulateCandidate(t *testing.T) {
	t.Run("paired up buy returns candidate", func(t *testing.T) {
		st := State{
			upQty:        5,
			upAvgCents:   60,
			downQty:      0,
			downAvgCents: 0,
		}
		book := OrderBook{
			up: OrderBookSide{
				bestAsk:     60,
				bestAskSize: 20,
			},
			down: OrderBookSide{
				bestAsk:     41,
				bestAskSize: 20,
			},
		}

		curWorst := minPnLCents(st)
		c1 := makeAccumulateCandidate(st, book, SideUp, -800, 10)
		c2 := makeAccumulateCandidate(st, book, SideDown, -800, 10)
		fmt.Println(curWorst, c1.score)
		fmt.Println(curWorst, c2.score)
		//if c.order == nil {
		//	t.Fatalf("expected candidate order")
		//}
		//if c.order.price != 60 || c.order.size != 10 || c.order.tag != "TAKE_UP" {
		//	t.Fatalf("unexpected order: %+v", *c.order)
		//}
	})
}
