package service

import (
	"context"
	"sync"

	"Polybot/internal/domain"
	"Polybot/internal/ports"
)

type PositionService struct {
	repo      ports.PositionRepository
	mu        sync.RWMutex
	positions map[domain.MarketID]map[domain.PositionSide]domain.Position
}

func NewPositionService(repo ports.PositionRepository) *PositionService {
	return &PositionService{
		repo:      repo,
		positions: make(map[domain.MarketID]map[domain.PositionSide]domain.Position),
	}
}

func (s *PositionService) LoadFromRepo(ctx context.Context) error {
	positions, err := s.repo.ListOpenPositions(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range positions {
		if s.positions[p.MarketID] == nil {
			s.positions[p.MarketID] = make(map[domain.PositionSide]domain.Position)
		}
		s.positions[p.MarketID][p.Side] = p
	}
	return nil
}

func (s *PositionService) GetExposureForMarket(marketID domain.MarketID) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var total float64
	for _, p := range s.positions[marketID] {
		total += p.NotionalUSD
	}
	return total
}

func (s *PositionService) GetTotalExposure() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var total float64
	for _, sides := range s.positions {
		for _, p := range sides {
			total += p.NotionalUSD
		}
	}
	return total
}

func (s *PositionService) GetInventoryPenalty(marketID domain.MarketID) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sides := s.positions[marketID]
	if len(sides) == 0 {
		return 0
	}
	// Penalize adding to existing positions
	var total float64
	for _, p := range sides {
		total += p.NotionalUSD
	}
	return total * 0.001 // 0.1% penalty per dollar of existing exposure
}

func (s *PositionService) RecordPosition(ctx context.Context, p domain.Position) error {
	s.mu.Lock()
	if s.positions[p.MarketID] == nil {
		s.positions[p.MarketID] = make(map[domain.PositionSide]domain.Position)
	}
	s.positions[p.MarketID][p.Side] = p
	s.mu.Unlock()

	return s.repo.SavePosition(ctx, p)
}
