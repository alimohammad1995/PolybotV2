package main

import (
	"math"
	"sync"
)

var trackersMu = &sync.RWMutex{}
var trackers = map[string]*MarketTracker{}

type MarketTracker struct {
	mu sync.Mutex

	upLots   LotMinHeap
	downLots LotMinHeap

	pairedQty         float64
	pairedProfitCents float64
}

func NewMarketTracker() *MarketTracker { return &MarketTracker{} }

func GetTracker(marketID string) *MarketTracker {
	trackersMu.RLock()
	tr := trackers[marketID]
	trackersMu.RUnlock()
	if tr != nil {
		return tr
	}
	trackersMu.Lock()
	defer trackersMu.Unlock()

	if trackers[marketID] == nil {
		trackers[marketID] = NewMarketTracker()
	}

	return trackers[marketID]
}

func (t *MarketTracker) OnFillUp(price int, qty float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.upLots.Push(price, qty)
	t.pairLots()
}

func (t *MarketTracker) OnFillDown(price int, qty float64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.downLots.Push(price, qty)
	t.pairLots()
}

func (t *MarketTracker) pairLots() {
	for {
		upTop, okU := t.upLots.Peek()
		dnTop, okD := t.downLots.Peek()
		if !okU || !okD {
			return
		}
		q := math.Min(upTop.qty, dnTop.qty)
		if q <= 1e-12 {
			return
		}
		// Pop from both at their cheapest prices
		uPrice, uGot := t.upLots.PopQty(q)
		dPrice, dGot := t.downLots.PopQty(q)
		q2 := math.Min(uGot, dGot)
		if q2 <= 1e-12 {
			return
		}
		// profit per paired share = 100 - (u+d) - FEES
		profit := float64(PayoutCents-(uPrice+dPrice)) * q2
		t.pairedQty += q2
		t.pairedProfitCents += profit
	}
}

func (t *MarketTracker) CheapestUnpairedDownPrice() (int, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	lot, ok := t.downLots.Peek()
	if !ok {
		return 0, false
	}
	return lot.price, true
}

func (t *MarketTracker) CheapestUnpairedUpPrice() (int, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	lot, ok := t.upLots.Peek()
	if !ok {
		return 0, false
	}
	return lot.price, true
}

type UnpairedStats struct {
	Qty       float64
	CostCents float64 // sum(price*qty)
	MinPrice  int
	MaxPrice  int
}

func (t *MarketTracker) UnpairedUpStats() (UnpairedStats, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.upLots.items) == 0 {
		return UnpairedStats{}, false
	}
	st := UnpairedStats{MinPrice: int(^uint(0) >> 1), MaxPrice: 0}
	for _, it := range t.upLots.items {
		if it.qty <= 1e-12 {
			continue
		}
		st.Qty += it.qty
		st.CostCents += float64(it.price) * it.qty
		if it.price < st.MinPrice {
			st.MinPrice = it.price
		}
		if it.price > st.MaxPrice {
			st.MaxPrice = it.price
		}
	}
	if st.Qty <= 1e-12 {
		return UnpairedStats{}, false
	}
	if st.MinPrice == int(^uint(0)>>1) {
		st.MinPrice = st.MaxPrice
	}
	return st, true
}

func (t *MarketTracker) UnpairedDownStats() (UnpairedStats, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.downLots.items) == 0 {
		return UnpairedStats{}, false
	}
	st := UnpairedStats{MinPrice: int(^uint(0) >> 1), MaxPrice: 0}
	for _, it := range t.downLots.items {
		if it.qty <= 1e-12 {
			continue
		}
		st.Qty += it.qty
		st.CostCents += float64(it.price) * it.qty
		if it.price < st.MinPrice {
			st.MinPrice = it.price
		}
		if it.price > st.MaxPrice {
			st.MaxPrice = it.price
		}
	}
	if st.Qty <= 1e-12 {
		return UnpairedStats{}, false
	}
	if st.MinPrice == int(^uint(0)>>1) {
		st.MinPrice = st.MaxPrice
	}
	return st, true
}

// ============================================================================
// Pair tracking (cheapest-unpaired lots) for correct close-cap
// ============================================================================

type Lot struct {
	price int
	qty   float64
}

type LotMinHeap struct {
	items []Lot
}

func (h *LotMinHeap) Len() int { return len(h.items) }
func (h *LotMinHeap) Peek() (Lot, bool) {
	if len(h.items) == 0 {
		return Lot{}, false
	}
	return h.items[0], true
}
func (h *LotMinHeap) Push(price int, qty float64) {
	if qty <= 0 || price <= 0 {
		return
	}
	h.items = append(h.items, Lot{price: price, qty: qty})
	h.siftUp(len(h.items) - 1)
}
func (h *LotMinHeap) PopQty(want float64) (price int, got float64) {
	if want <= 0 {
		return 0, 0
	}
	if len(h.items) == 0 {
		return 0, 0
	}
	top := h.items[0]
	use := math.Min(want, top.qty)
	top.qty -= use
	price = top.price
	got = use
	if top.qty <= 1e-12 {
		h.swap(0, len(h.items)-1)
		h.items = h.items[:len(h.items)-1]
		h.siftDown(0)
	} else {
		h.items[0] = top
	}
	return price, got
}

func (h *LotMinHeap) siftUp(i int) {
	for i > 0 {
		p := (i - 1) / 2
		if h.items[p].price <= h.items[i].price {
			break
		}
		h.swap(p, i)
		i = p
	}
}
func (h *LotMinHeap) siftDown(i int) {
	n := len(h.items)
	for {
		l := 2*i + 1
		r := 2*i + 2
		small := i
		if l < n && h.items[l].price < h.items[small].price {
			small = l
		}
		if r < n && h.items[r].price < h.items[small].price {
			small = r
		}
		if small == i {
			break
		}
		h.swap(i, small)
		i = small
	}
}

func (h *LotMinHeap) swap(i, j int) { h.items[i], h.items[j] = h.items[j], h.items[i] }
