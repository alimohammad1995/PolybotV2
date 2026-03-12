package storage

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"Polybot/internal/domain"
)

// InMemoryEventRepo stores events in memory. Replace with Postgres for production.
type InMemoryEventRepo struct {
	mu          sync.Mutex
	fairValues  []domain.FairValue
	signals     []domain.TradeSignal
	quotes      []domain.MarketQuote
	snapshots   []domain.ReferenceSnapshot
	fills       []domain.Fill
	settlements []domain.Settlement
	logger      *slog.Logger
}

func NewInMemoryEventRepo(logger *slog.Logger) *InMemoryEventRepo {
	return &InMemoryEventRepo{logger: logger}
}

func (r *InMemoryEventRepo) SaveFairValue(_ context.Context, fv domain.FairValue) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fairValues = append(r.fairValues, fv)
	return nil
}

func (r *InMemoryEventRepo) SaveSignal(_ context.Context, signal domain.TradeSignal) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signals = append(r.signals, signal)
	return nil
}

func (r *InMemoryEventRepo) SaveQuote(_ context.Context, quote domain.MarketQuote) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quotes = append(r.quotes, quote)
	return nil
}

func (r *InMemoryEventRepo) SaveReferenceSnapshot(_ context.Context, snap domain.ReferenceSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.snapshots = append(r.snapshots, snap)
	return nil
}

func (r *InMemoryEventRepo) SaveFill(_ context.Context, fill domain.Fill) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fills = append(r.fills, fill)
	return nil
}

func (r *InMemoryEventRepo) SaveSettlement(_ context.Context, s domain.Settlement) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settlements = append(r.settlements, s)
	return nil
}

func (r *InMemoryEventRepo) GetFairValues() []domain.FairValue {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.FairValue{}, r.fairValues...)
}

func (r *InMemoryEventRepo) GetSignals() []domain.TradeSignal {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]domain.TradeSignal{}, r.signals...)
}

// InMemoryPositionRepo stores positions in memory. Replace with Postgres for production.
type InMemoryPositionRepo struct {
	mu        sync.Mutex
	positions map[string]domain.Position // key: marketID:side
}

func NewInMemoryPositionRepo() *InMemoryPositionRepo {
	return &InMemoryPositionRepo{
		positions: make(map[string]domain.Position),
	}
}

func posKey(marketID domain.MarketID, side domain.PositionSide) string {
	return fmt.Sprintf("%s:%s", marketID, side)
}

func (r *InMemoryPositionRepo) SavePosition(_ context.Context, p domain.Position) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.positions[posKey(p.MarketID, p.Side)] = p
	return nil
}

func (r *InMemoryPositionRepo) GetPosition(_ context.Context, marketID domain.MarketID, side domain.PositionSide) (domain.Position, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.positions[posKey(marketID, side)]
	if !ok {
		return domain.Position{}, fmt.Errorf("position not found")
	}
	return p, nil
}

func (r *InMemoryPositionRepo) ListOpenPositions(_ context.Context) ([]domain.Position, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]domain.Position, 0, len(r.positions))
	for _, p := range r.positions {
		result = append(result, p)
	}
	return result, nil
}

func (r *InMemoryPositionRepo) DeletePosition(_ context.Context, marketID domain.MarketID, side domain.PositionSide) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.positions, posKey(marketID, side))
	return nil
}
