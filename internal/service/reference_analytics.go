package service

import (
	"math"
	"sync"
	"time"

	"Polybot/internal/domain"
)

type tickRecord struct {
	Price     float64
	Timestamp time.Time
	LogReturn float64
}

// ReferenceAnalyticsService processes Chainlink ticks into full reference state.
// This is the "truth process" — never contaminated with Polymarket data.
type ReferenceAnalyticsService struct {
	mu       sync.RWMutex
	ticks    map[string][]tickRecord
	states   map[string]domain.ReferenceState
	maxTicks int

	// Jump detection threshold: if |log return| > this, it's a jump
	jumpThresholdMultiple float64
}

func NewReferenceAnalyticsService(maxTicks int) *ReferenceAnalyticsService {
	return &ReferenceAnalyticsService{
		ticks:                 make(map[string][]tickRecord),
		states:                make(map[string]domain.ReferenceState),
		maxTicks:              maxTicks,
		jumpThresholdMultiple: 4.0, // 4 sigma
	}
}

// OnTick processes a new Chainlink price tick and updates all analytics.
func (s *ReferenceAnalyticsService) OnTick(tick domain.ChainlinkTick) {
	// Lock briefly to append and copy records
	s.mu.Lock()
	records := s.ticks[tick.Asset]

	var lr float64
	if len(records) > 0 {
		prev := records[len(records)-1]
		if prev.Price > 0 && tick.Price > 0 {
			lr = math.Log(tick.Price / prev.Price)
		}
	}

	records = append(records, tickRecord{
		Price:     tick.Price,
		Timestamp: tick.Timestamp,
		LogReturn: lr,
	})

	// Trim to maxTicks
	if len(records) > s.maxTicks {
		records = records[len(records)-s.maxTicks:]
	}
	s.ticks[tick.Asset] = records

	// Copy records for computation outside lock
	recordsCopy := make([]tickRecord, len(records))
	copy(recordsCopy, records)
	s.mu.Unlock()

	// Compute state without holding the lock (expensive: vol windows, jump score)
	state := s.computeState(tick.Asset, recordsCopy)

	s.mu.Lock()
	s.states[tick.Asset] = state
	s.mu.Unlock()
}

// GetState returns the current reference state for an asset.
func (s *ReferenceAnalyticsService) GetState(asset string) (domain.ReferenceState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.states[asset]
	return state, ok
}

func (s *ReferenceAnalyticsService) computeState(asset string, records []tickRecord) domain.ReferenceState {
	if len(records) == 0 {
		return domain.ReferenceState{Asset: asset, Regime: "unknown"}
	}

	latest := records[len(records)-1]
	state := domain.ReferenceState{
		Asset:        asset,
		CurrentPrice: latest.Price,
		TickCount:    len(records),
		LastUpdate:   latest.Timestamp,
	}

	if len(records) < 2 {
		state.Regime = "unknown"
		return state
	}

	// Compute vol over different windows
	now := latest.Timestamp
	state.RealizedVol1m = s.computeVolWindow(records, now, 60*time.Second)
	state.RealizedVol5m = s.computeVolWindow(records, now, 5*60*time.Second)

	// Vol stability: ratio of short-term to medium-term vol
	if state.RealizedVol5m > 0 {
		state.VolStabilityScore = state.RealizedVol1m / state.RealizedVol5m
	} else {
		state.VolStabilityScore = 1.0
	}

	// Jump detection: check if latest log return is extreme
	state.JumpScore = s.computeJumpScore(records)

	// Regime classification
	state.Regime = s.classifyRegime(state)

	return state
}

func (s *ReferenceAnalyticsService) computeVolWindow(records []tickRecord, now time.Time, window time.Duration) float64 {
	cutoff := now.Add(-window)

	// Single-pass: accumulate sum and sumSq, skipping stale (zero-return) ticks.
	// Chainlink feeds can repeat the same price across consecutive ticks.
	// Including zero-return ticks crushes measured vol. We track the actual mean
	// interval between real price changes for correct time-scaling.
	var sum, sumSq float64
	var n int
	var firstRealTS, lastRealTS time.Time
	for i := 1; i < len(records); i++ {
		if records[i].Timestamp.Before(cutoff) {
			continue
		}
		lr := records[i].LogReturn
		if lr == 0 {
			continue // stale tick — skip
		}
		sum += lr
		sumSq += lr * lr
		n++
		if firstRealTS.IsZero() {
			firstRealTS = records[i].Timestamp
		}
		lastRealTS = records[i].Timestamp
	}

	if n < 2 {
		return 0
	}

	fn := float64(n)
	mean := sum / fn
	variance := sumSq/fn - mean*mean
	if variance < 0 {
		variance = 0
	}

	// Normalize per-change vol to per-second vol using the actual mean interval
	// between real price changes, not the resample interval.
	perChangeVol := math.Sqrt(variance)
	spanSec := lastRealTS.Sub(firstRealTS).Seconds()
	if spanSec <= 0 {
		return perChangeVol / math.Sqrt(1.0)
	}
	meanIntervalSec := spanSec / float64(n-1)
	if meanIntervalSec <= 0 {
		meanIntervalSec = 1.0
	}
	return perChangeVol / math.Sqrt(meanIntervalSec)
}

func (s *ReferenceAnalyticsService) computeJumpScore(records []tickRecord) float64 {
	if len(records) < 10 {
		return 0
	}

	// Single-pass over recent records: accumulate sum and sumSq directly
	recent := records[len(records)-min(60, len(records)):]
	var sum, sumSq float64
	var n int
	for i := 1; i < len(recent); i++ {
		lr := recent[i].LogReturn
		sum += lr
		sumSq += lr * lr
		n++
	}

	if n < 5 {
		return 0
	}

	fn := float64(n)
	mean := sum / fn
	variance := sumSq/fn - mean*mean
	if variance <= 0 {
		return 0
	}
	std := math.Sqrt(variance)
	if std <= 0 {
		return 0
	}

	// Jump score = |latest return| / std
	latestReturn := records[len(records)-1].LogReturn
	return math.Abs(latestReturn) / std
}

func (s *ReferenceAnalyticsService) classifyRegime(state domain.ReferenceState) string {
	// Primary: use 1m vol level (per-second vol)
	// Secondary: check vol stability and jump score
	// BTC annual vol ~60% => per-second vol ~0.0001
	switch {
	case state.JumpScore > s.jumpThresholdMultiple:
		return "jump"
	case state.RealizedVol1m < 0.0007:
		return "calm"
	case state.RealizedVol1m < 0.0028:
		return "normal"
	default:
		return "volatile"
	}
}
