package service

import (
	"testing"
)

func TestRiskService_ShouldAllowNewTrade(t *testing.T) {
	t.Run("returns_false_below_cutoff", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			NoNewTradeCutoffSecs: 300,
		})

		if svc.ShouldAllowNewTrade(100) {
			t.Error("expected ShouldAllowNewTrade=false when remaining < cutoff")
		}
		if svc.ShouldAllowNewTrade(300) {
			t.Error("expected ShouldAllowNewTrade=false when remaining == cutoff")
		}
	})

	t.Run("returns_true_above_cutoff", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			NoNewTradeCutoffSecs: 300,
		})

		if !svc.ShouldAllowNewTrade(301) {
			t.Error("expected ShouldAllowNewTrade=true when remaining > cutoff")
		}
		if !svc.ShouldAllowNewTrade(3600) {
			t.Error("expected ShouldAllowNewTrade=true when plenty of time remaining")
		}
	})
}

func TestRiskService_ComputeTargetSizeUSD(t *testing.T) {
	t.Run("returns_zero_for_negative_edge", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 1000,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         1.0,
		})

		size := svc.ComputeTargetSizeUSD(-0.05, 10000, 0)
		if size != 0 {
			t.Errorf("expected 0 for negative edge, got %f", size)
		}
	})

	t.Run("returns_zero_for_zero_edge", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 1000,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         1.0,
		})

		size := svc.ComputeTargetSizeUSD(0, 10000, 0)
		if size != 0 {
			t.Errorf("expected 0 for zero edge, got %f", size)
		}
	})

	t.Run("caps_at_max_position", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 100,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         1.0,
		})

		// Large bankroll and edge => raw size = 100000 * 0.25 * 0.10 * 10 = 25000
		// Max remaining = 100 - 0 = 100
		// Capped at 100
		size := svc.ComputeTargetSizeUSD(0.10, 100000, 0)
		if size != 100 {
			t.Errorf("expected size capped at 100, got %f", size)
		}
	})

	t.Run("caps_considering_current_exposure", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 100,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         1.0,
		})

		// Already have 80 USD exposure, max remaining = 100 - 80 = 20
		size := svc.ComputeTargetSizeUSD(0.10, 100000, 80)
		if size != 20 {
			t.Errorf("expected size capped at 20 (max - current exposure), got %f", size)
		}
	})

	t.Run("returns_zero_below_min_trade_size", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 1000,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         50.0,
		})

		// Small edge and small bankroll => raw size = 100 * 0.25 * 0.001 * 10 = 0.25
		// 0.25 < 50 min trade size => returns 0
		size := svc.ComputeTargetSizeUSD(0.001, 100, 0)
		if size != 0 {
			t.Errorf("expected 0 when below min trade size, got %f", size)
		}
	})

	t.Run("positive_edge_computes_kelly_size", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 10000,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         1.0,
		})

		// size = 5000 * 0.25 * 0.05 * 10 = 625
		size := svc.ComputeTargetSizeUSD(0.05, 5000, 0)
		expected := 5000.0 * 0.25 * 0.05 * 10.0
		if size != expected {
			t.Errorf("expected size=%f, got %f", expected, size)
		}
	})
}
