package service

import (
	"context"
	"math"
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

// ImbalancePenaltyConfig holds the exponential penalty parameters.
type ImbalancePenaltyConfig struct {
	Alpha float64 // base penalty scale (default 0.005)
	Beta  float64 // exponential growth rate (default 0.15)
}

// GetInventoryPenalties returns per-side penalties for directional trading.
// Buying the heavy side gets an exponential penalty; buying the light side gets zero.
func (s *PositionService) GetInventoryPenalties(marketID domain.MarketID, cfg ImbalancePenaltyConfig) (penaltyBuyUp, penaltyBuyDown float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sides := s.positions[marketID]
	if len(sides) == 0 {
		return 0, 0
	}

	upQty := sides[domain.PositionUp].Quantity
	downQty := sides[domain.PositionDown].Quantity
	imbalance := math.Abs(upQty - downQty)

	penalty := cfg.Alpha * (math.Exp(cfg.Beta*imbalance) - 1)

	if upQty > downQty {
		// UP is heavy — penalize buying more UP, reward buying DOWN
		return penalty, 0
	} else if downQty > upQty {
		// DOWN is heavy — penalize buying more DOWN, reward buying UP
		return 0, penalty
	}
	return 0, 0
}

// GetInventory returns the quantity and total cost for UP and DOWN positions on a market.
// Used by the hedge engine to compute guaranteed floor.
func (s *PositionService) GetInventory(marketID domain.MarketID) (upQty, downQty, upCost, downCost float64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sides := s.positions[marketID]
	if up, ok := sides[domain.PositionUp]; ok {
		upQty = up.Quantity
		upCost = up.NotionalUSD
	}
	if down, ok := sides[domain.PositionDown]; ok {
		downQty = down.Quantity
		downCost = down.NotionalUSD
	}
	return
}

// AccumulateFill adds a fill to the existing position for the given market and side.
// If no position exists, it creates one. If a position exists, it updates the
// weighted average entry price, adds quantity, and adds notional cost.
func (s *PositionService) AccumulateFill(ctx context.Context, fill domain.Fill) error {
	side := fillSideToPositionSide(fill.Side)

	s.mu.Lock()
	if s.positions[fill.MarketID] == nil {
		s.positions[fill.MarketID] = make(map[domain.PositionSide]domain.Position)
	}

	existing, ok := s.positions[fill.MarketID][side]
	if !ok {
		existing = domain.Position{
			MarketID:         fill.MarketID,
			Side:             side,
			OpenedAt:         fill.Timestamp,
			HoldToSettlement: true,
		}
	}

	fillQty := fill.SizeUSD / fill.Price
	newQty := existing.Quantity + fillQty
	if newQty > 0 {
		existing.AvgEntryPrice = (existing.AvgEntryPrice*existing.Quantity + fill.Price*fillQty) / newQty
	}
	existing.Quantity = newQty
	existing.NotionalUSD += fill.SizeUSD

	s.positions[fill.MarketID][side] = existing
	s.mu.Unlock()

	return s.repo.SavePosition(ctx, existing)
}

// RecordPosition overwrites the position for the given market and side.
// Used for bootstrap loading and paper mode.
func (s *PositionService) RecordPosition(ctx context.Context, p domain.Position) error {
	s.mu.Lock()
	if s.positions[p.MarketID] == nil {
		s.positions[p.MarketID] = make(map[domain.PositionSide]domain.Position)
	}
	s.positions[p.MarketID][p.Side] = p
	s.mu.Unlock()

	return s.repo.SavePosition(ctx, p)
}

func fillSideToPositionSide(side domain.TradeSignalSide) domain.PositionSide {
	switch side {
	case domain.SignalBuyUp, domain.SignalHedgeUp:
		return domain.PositionUp
	default:
		return domain.PositionDown
	}
}
