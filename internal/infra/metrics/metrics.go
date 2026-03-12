package metrics

import (
	"log/slog"
	"sync"
	"sync/atomic"
)

type Metrics struct {
	evaluations atomic.Int64
	trades      atomic.Int64
	errors      atomic.Int64
	mu          sync.Mutex
	pnlRealized float64
	logger      *slog.Logger
}

func New(logger *slog.Logger) *Metrics {
	return &Metrics{logger: logger}
}

func (m *Metrics) RecordEvaluation() {
	m.evaluations.Add(1)
}

func (m *Metrics) RecordTrade() {
	m.trades.Add(1)
}

func (m *Metrics) RecordError() {
	m.errors.Add(1)
}

func (m *Metrics) AddPnL(amount float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pnlRealized += amount
}

func (m *Metrics) Snapshot() map[string]any {
	m.mu.Lock()
	pnl := m.pnlRealized
	m.mu.Unlock()

	return map[string]any{
		"evaluations":  m.evaluations.Load(),
		"trades":       m.trades.Load(),
		"errors":       m.errors.Load(),
		"pnl_realized": pnl,
	}
}
