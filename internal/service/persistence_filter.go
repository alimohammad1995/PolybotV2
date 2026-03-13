package service

import (
	"sync"
	"time"

	"Polybot/internal/domain"
)

// PersistenceFilter requires a trade signal to persist across N consecutive
// evaluations before it is allowed to execute. This kills false positives
// from transient edge.
type PersistenceFilter struct {
	requiredCount      int // for directional signals
	hedgeRequiredCount int // for hedge signals (can be more reactive)
	mu                 sync.Mutex
	streaks            map[domain.MarketID]*edgeStreak
}

type edgeStreak struct {
	Side      domain.TradeSignalSide
	Count     int
	FirstSeen time.Time
}

func NewPersistenceFilter(requiredCount, hedgeRequiredCount int) *PersistenceFilter {
	return &PersistenceFilter{
		requiredCount:      requiredCount,
		hedgeRequiredCount: hedgeRequiredCount,
		streaks:            make(map[domain.MarketID]*edgeStreak),
	}
}

// Check returns true if the signal has persisted long enough to trade.
// Signals with Side == SignalNone reset the streak.
func (f *PersistenceFilter) Check(signal domain.TradeSignal) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	if signal.Side == domain.SignalNone {
		delete(f.streaks, signal.MarketID)
		return false
	}

	streak, exists := f.streaks[signal.MarketID]
	if !exists || streak.Side != signal.Side {
		// New signal or side changed — start fresh
		f.streaks[signal.MarketID] = &edgeStreak{
			Side:      signal.Side,
			Count:     1,
			FirstSeen: signal.Timestamp,
		}
		streak = f.streaks[signal.MarketID]
	} else {
		streak.Count++
	}

	required := f.requiredCount
	if signal.Side == domain.SignalHedgeUp || signal.Side == domain.SignalHedgeDown {
		required = f.hedgeRequiredCount
	}

	return streak.Count >= required
}

// Reset clears the streak for a market (e.g. on market change).
func (f *PersistenceFilter) Reset(marketID domain.MarketID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.streaks, marketID)
}
