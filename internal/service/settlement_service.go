package service

import (
	"context"
	"log/slog"

	"Polybot/internal/domain"
	"Polybot/internal/ports"
)

type SettlementService struct {
	positionRepo ports.PositionRepository
	eventRepo    ports.EventRepository
	logger       *slog.Logger
}

func NewSettlementService(posRepo ports.PositionRepository, eventRepo ports.EventRepository, logger *slog.Logger) *SettlementService {
	return &SettlementService{
		positionRepo: posRepo,
		eventRepo:    eventRepo,
		logger:       logger,
	}
}

func (s *SettlementService) Settle(ctx context.Context, marketID domain.MarketID, outcome string) error {
	positions, err := s.positionRepo.ListOpenPositions(ctx)
	if err != nil {
		return err
	}

	for _, p := range positions {
		if p.MarketID != marketID {
			continue
		}

		var pnl float64
		won := (outcome == "up" && p.Side == domain.PositionUp) ||
			(outcome == "down" && p.Side == domain.PositionDown)

		if won {
			// Payout is $1 per contract, cost was avg entry price
			pnl = p.Quantity * (1.0 - p.AvgEntryPrice)
		} else {
			pnl = -p.Quantity * p.AvgEntryPrice
		}

		settlement := domain.Settlement{
			MarketID:    marketID,
			Outcome:     outcome,
			RealizedPnL: pnl,
		}

		if err := s.eventRepo.SaveSettlement(ctx, settlement); err != nil {
			s.logger.Error("failed to save settlement", "market", marketID, "error", err)
		}

		if err := s.positionRepo.DeletePosition(ctx, marketID, p.Side); err != nil {
			s.logger.Error("failed to delete position", "market", marketID, "error", err)
		}

		s.logger.Info("settled position",
			"market", marketID,
			"side", p.Side,
			"outcome", outcome,
			"pnl", pnl,
		)
	}

	return nil
}
