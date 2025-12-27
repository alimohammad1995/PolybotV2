package main

import "testing"

func TestCalculateMakerBids(t *testing.T) {
	book := OrderBook{
		up: OrderBookSide{
			bestBid: 70,
			bestAsk: 71,
		},
		down: OrderBookSide{
			bestBid: 29,
			bestAsk: 30,
		},
	}

	tests := []struct {
		name        string
		net         float64
		wantBidUp   int
		wantBidDown int
		wantCapUp   int
		wantCapDown int
	}{
		{
			name:        "neutral net",
			net:         0,
			wantBidUp:   69,
			wantBidDown: 27,
			wantCapUp:   41,
			wantCapDown: 69,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotBidUp, gotBidDown, gotCapUp, gotCapDown := calculateMakerBids(book, tt.net)
			if gotBidUp != tt.wantBidUp || gotBidDown != tt.wantBidDown {
				t.Fatalf("bids mismatch: got (%d,%d), want (%d,%d)", gotBidUp, gotBidDown, tt.wantBidUp, tt.wantBidDown)
			}
			if gotCapUp != tt.wantCapUp || gotCapDown != tt.wantCapDown {
				t.Fatalf("caps mismatch: got (%d,%d), want (%d,%d)", gotCapUp, gotCapDown, tt.wantCapUp, tt.wantCapDown)
			}
		})
	}
}

func TestSimulateMinPnLCents(t *testing.T) {
	base := State{
		upQty:        10,
		downQty:      10,
		upAvgCents:   40,
		downAvgCents: 50,
	}

	t.Run("zero qty returns current min pnl", func(t *testing.T) {
		got := simulateMinPnLCents(base, SideUp, 60, 0)
		want := minPnLCents(base)
		if !floatEq(got, want) {
			t.Fatalf("min pnl mismatch: got %v, want %v", got, want)
		}
	})

	t.Run("side up increases inventory cost", func(t *testing.T) {
		got := simulateMinPnLCents(base, SideUp, 60, 5)
		want := -200.0
		if !floatEq(got, want) {
			t.Fatalf("min pnl mismatch: got %v, want %v", got, want)
		}
	})
}
