package service

import (
	"context"
	"testing"
	"time"

	"Polybot/internal/domain"
)

// mockCostModel is a test double for ports.CostModel.
type mockCostModel struct {
	cost float64
	err  error
}

func (m *mockCostModel) EstimateAllInCost(_ context.Context, _ domain.MarketID) (float64, error) {
	return m.cost, m.err
}

func TestSignalService_Generate(t *testing.T) {
	ctx := context.Background()

	t.Run("buy_up_signal_when_edge_exceeds_hurdle", func(t *testing.T) {
		svc := NewSignalService(
			&mockCostModel{cost: 0.01},
			SignalConfig{BaseHurdle: 0.02, MaxSizeUSD: 1000},
		)

		// FairValue with high ProbUpLower so edge on buy_up side is large
		fv := domain.FairValue{
			ProbUp:      0.80,
			ProbUpLower: 0.75,
			ProbUpUpper: 0.85,
		}
		quote := domain.MarketQuote{
			MarketID:  "market-1",
			Up:        domain.SideQuote{Ask: 0.60},
			Down:      domain.SideQuote{Ask: 0.60},
			Timestamp: time.Now(),
		}

		sig, err := svc.Generate(ctx, &fv, &quote, 0.0, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// edgeBuyUp = 0.75 - 0.60 - 0.01 = 0.14 (> hurdle 0.02)
		// edgeBuyDown = (1 - 0.85) - 0.60 - 0.01 = -0.46 (negative)
		if sig.Side != domain.SignalBuyUp {
			t.Errorf("expected side=buy_up, got %s", sig.Side)
		}
		if sig.EdgeBuyUp <= sig.EffectiveHurdle {
			t.Errorf("expected EdgeBuyUp(%f) > hurdle(%f)", sig.EdgeBuyUp, sig.EffectiveHurdle)
		}
		if sig.Reason != "buy_up_edge_exceeds_hurdle" {
			t.Errorf("expected reason buy_up_edge_exceeds_hurdle, got %s", sig.Reason)
		}
	})

	t.Run("buy_down_signal_when_edge_exceeds_hurdle", func(t *testing.T) {
		svc := NewSignalService(
			&mockCostModel{cost: 0.01},
			SignalConfig{BaseHurdle: 0.02, MaxSizeUSD: 1000},
		)

		// FairValue with low ProbUpUpper so (1-ProbUpUpper) is large
		fv := domain.FairValue{
			ProbUp:      0.20,
			ProbUpLower: 0.15,
			ProbUpUpper: 0.25,
		}
		quote := domain.MarketQuote{
			MarketID:  "market-2",
			Up:        domain.SideQuote{Ask: 0.60},
			Down:      domain.SideQuote{Ask: 0.60},
			Timestamp: time.Now(),
		}

		sig, err := svc.Generate(ctx, &fv, &quote, 0.0, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// edgeBuyUp = 0.15 - 0.60 - 0.01 = -0.46 (negative)
		// edgeBuyDown = (1-0.25) - 0.60 - 0.01 = 0.14 (> hurdle 0.02)
		if sig.Side != domain.SignalBuyDown {
			t.Errorf("expected side=buy_down, got %s", sig.Side)
		}
		if sig.EdgeBuyDown <= sig.EffectiveHurdle {
			t.Errorf("expected EdgeBuyDown(%f) > hurdle(%f)", sig.EdgeBuyDown, sig.EffectiveHurdle)
		}
		if sig.Reason != "buy_down_edge_exceeds_hurdle" {
			t.Errorf("expected reason buy_down_edge_exceeds_hurdle, got %s", sig.Reason)
		}
	})

	t.Run("no_signal_when_no_edge", func(t *testing.T) {
		svc := NewSignalService(
			&mockCostModel{cost: 0.01},
			SignalConfig{BaseHurdle: 0.02, MaxSizeUSD: 1000},
		)

		// Fair value near 0.5 and asks near 0.5 => no edge
		fv := domain.FairValue{
			ProbUp:      0.50,
			ProbUpLower: 0.48,
			ProbUpUpper: 0.52,
		}
		quote := domain.MarketQuote{
			MarketID:  "market-3",
			Up:        domain.SideQuote{Ask: 0.50},
			Down:      domain.SideQuote{Ask: 0.50},
			Timestamp: time.Now(),
		}

		sig, err := svc.Generate(ctx, &fv, &quote, 0.0, 0.0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// edgeBuyUp = 0.48 - 0.50 - 0.01 = -0.03
		// edgeBuyDown = (1-0.52) - 0.50 - 0.01 = -0.03
		if sig.Side != domain.SignalNone {
			t.Errorf("expected side=none, got %s", sig.Side)
		}
		if sig.Reason != "no_trade" {
			t.Errorf("expected reason no_trade, got %s", sig.Reason)
		}
	})

	t.Run("hurdle_plus_inventory_penalty_blocks_marginal_trades", func(t *testing.T) {
		svc := NewSignalService(
			&mockCostModel{cost: 0.01},
			SignalConfig{BaseHurdle: 0.02, MaxSizeUSD: 1000},
		)

		// Edge would exceed base hurdle but not hurdle + inventory penalty
		fv := domain.FairValue{
			ProbUp:      0.70,
			ProbUpLower: 0.66,
			ProbUpUpper: 0.74,
		}
		quote := domain.MarketQuote{
			MarketID:  "market-4",
			Up:        domain.SideQuote{Ask: 0.60},
			Down:      domain.SideQuote{Ask: 0.60},
			Timestamp: time.Now(),
		}
		// edgeBuyUp = 0.66 - 0.60 - 0.01 = 0.05
		// With inventoryPenalty = 0.10, effective hurdle = 0.02 + 0.10 = 0.12
		// 0.05 < 0.12, so blocked
		sig, err := svc.Generate(ctx, &fv, &quote, 0.10, 0.10)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if sig.Side != domain.SignalNone {
			t.Errorf("expected side=none with high inventory penalty, got %s", sig.Side)
		}
		expectedHurdle := 0.12
		if diff := sig.EffectiveHurdle - expectedHurdle; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("expected effective hurdle≈0.12, got %f", sig.EffectiveHurdle)
		}
	})

	t.Run("cost_reduces_effective_edge", func(t *testing.T) {
		lowCost := NewSignalService(
			&mockCostModel{cost: 0.00},
			SignalConfig{BaseHurdle: 0.05, MaxSizeUSD: 1000},
		)
		highCost := NewSignalService(
			&mockCostModel{cost: 0.10},
			SignalConfig{BaseHurdle: 0.05, MaxSizeUSD: 1000},
		)

		fv := domain.FairValue{
			ProbUp:      0.70,
			ProbUpLower: 0.68,
			ProbUpUpper: 0.72,
		}
		quote := domain.MarketQuote{
			MarketID:  "market-5",
			Up:        domain.SideQuote{Ask: 0.55},
			Down:      domain.SideQuote{Ask: 0.55},
			Timestamp: time.Now(),
		}

		sigLow, _ := lowCost.Generate(ctx, &fv, &quote, 0.0, 0.0)
		sigHigh, _ := highCost.Generate(ctx, &fv, &quote, 0.0, 0.0)

		// Low cost: edgeBuyUp = 0.68 - 0.55 - 0.00 = 0.13 (signal)
		// High cost: edgeBuyUp = 0.68 - 0.55 - 0.10 = 0.03 (< 0.05 hurdle, no signal)
		if sigLow.Side != domain.SignalBuyUp {
			t.Errorf("expected buy_up with low cost, got %s", sigLow.Side)
		}
		if sigHigh.Side != domain.SignalNone {
			t.Errorf("expected none with high cost, got %s", sigHigh.Side)
		}
		if sigLow.EdgeBuyUp <= sigHigh.EdgeBuyUp {
			t.Errorf("expected low cost edge > high cost edge: low=%f high=%f",
				sigLow.EdgeBuyUp, sigHigh.EdgeBuyUp)
		}
	})
}
