package service

import (
	"sync"
	"time"

	"Polybot/internal/domain"
)

// Resampler converts irregular Chainlink ticks into fixed-interval resampled ticks.
// It forward-fills the last known price onto a regular grid (e.g. 500ms intervals).
type Resampler struct {
	interval    time.Duration
	mu          sync.Mutex
	lastRaw     map[string]domain.ChainlinkTick // asset → latest raw tick
	lastEmit    map[string]time.Time            // asset → last grid timestamp emitted
	subscribers []func(domain.ResampledTick)
}

func NewResampler(interval time.Duration) *Resampler {
	return &Resampler{
		interval: interval,
		lastRaw:  make(map[string]domain.ChainlinkTick),
		lastEmit: make(map[string]time.Time),
	}
}

// Subscribe registers a callback that fires on each resampled tick.
func (r *Resampler) Subscribe(fn func(domain.ResampledTick)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subscribers = append(r.subscribers, fn)
}

// OnRawTick processes an incoming irregular Chainlink tick.
// It stores the latest price and emits resampled ticks for any grid boundaries
// that have been crossed since the last emission.
func (r *Resampler) OnRawTick(tick domain.ChainlinkTick) {
	r.mu.Lock()
	r.lastRaw[tick.Asset] = tick

	now := tick.Timestamp
	gridNow := r.floorToGrid(now)

	lastEmit, hasLast := r.lastEmit[tick.Asset]
	if !hasLast {
		// First tick for this asset: emit one resampled tick at the current grid point
		r.lastEmit[tick.Asset] = gridNow
		r.mu.Unlock()

		r.emit(domain.ResampledTick{
			Asset:     tick.Asset,
			Price:     tick.Price,
			Timestamp: gridNow,
			Interval:  r.interval,
		})
		return
	}

	// Emit all missing grid points since last emission (forward-fill)
	var toEmit []domain.ResampledTick
	for ts := lastEmit.Add(r.interval); !ts.After(gridNow); ts = ts.Add(r.interval) {
		toEmit = append(toEmit, domain.ResampledTick{
			Asset:     tick.Asset,
			Price:     tick.Price,
			Timestamp: ts,
			Interval:  r.interval,
		})
	}

	if len(toEmit) > 0 {
		r.lastEmit[tick.Asset] = toEmit[len(toEmit)-1].Timestamp
	}
	r.mu.Unlock()

	for _, rt := range toEmit {
		r.emit(rt)
	}
}

func (r *Resampler) emit(tick domain.ResampledTick) {
	r.mu.Lock()
	subs := make([]func(domain.ResampledTick), len(r.subscribers))
	copy(subs, r.subscribers)
	r.mu.Unlock()

	for _, fn := range subs {
		fn(tick)
	}
}

func (r *Resampler) floorToGrid(t time.Time) time.Time {
	ns := t.UnixNano()
	intervalNs := r.interval.Nanoseconds()
	return time.Unix(0, ns-ns%intervalNs)
}
