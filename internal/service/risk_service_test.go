package service

import (
	"math"
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
	// Price used in all tests: $0.50 per share
	price := 0.50

	t.Run("returns_zero_for_negative_edge", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 1000,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         1.0,
			MinTradeShares:          5,
		})

		size := svc.ComputeTargetSizeUSD(-0.05, 10000, 0, price, 0, false)
		if size != 0 {
			t.Errorf("expected 0 for negative edge, got %f", size)
		}
	})

	t.Run("returns_zero_for_zero_edge", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 1000,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         1.0,
			MinTradeShares:          5,
		})

		size := svc.ComputeTargetSizeUSD(0, 10000, 0, price, 0, false)
		if size != 0 {
			t.Errorf("expected 0 for zero edge, got %f", size)
		}
	})

	t.Run("caps_at_max_position_snapped_to_shares", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 100,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         1.0,
			MinTradeShares:          5,
		})

		// Large bankroll and edge => raw size = 100000 * 0.25 * 0.10 * 10 = 25000
		// Capped at maxRemaining = 100
		// shares = floor(100 / 0.50) = 200
		// result = 200 * 0.50 = 100
		size := svc.ComputeTargetSizeUSD(0.10, 100000, 0, price, 0, false)
		if size != 100 {
			t.Errorf("expected size capped at 100, got %f", size)
		}
	})

	t.Run("caps_considering_current_exposure", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 100,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         1.0,
			MinTradeShares:          5,
		})

		// maxRemaining = 100 - 80 = 20
		// shares = floor(20 / 0.50) = 40
		// result = 40 * 0.50 = 20
		size := svc.ComputeTargetSizeUSD(0.10, 100000, 80, price, 0, false)
		if size != 20 {
			t.Errorf("expected size capped at 20, got %f", size)
		}
	})

	t.Run("returns_zero_below_min_trade_size", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 1000,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         50.0,
			MinTradeShares:          5,
		})

		// raw size = 100 * 0.25 * 0.001 * 10 = 0.25 < 50 min => 0
		size := svc.ComputeTargetSizeUSD(0.001, 100, 0, price, 0, false)
		if size != 0 {
			t.Errorf("expected 0 when below min trade size, got %f", size)
		}
	})

	t.Run("returns_zero_below_min_shares", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 1000,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         0.01,
			MinTradeShares:          5,
		})

		// raw size = 100 * 0.25 * 0.001 * 10 = 0.25 USD
		// shares = floor(0.25 / 0.50) = 0 < 5 min shares => 0
		size := svc.ComputeTargetSizeUSD(0.001, 100, 0, price, 0, false)
		if size != 0 {
			t.Errorf("expected 0 when below min shares, got %f", size)
		}
	})

	t.Run("snaps_to_whole_shares", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 10000,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         1.0,
			MinTradeShares:          5,
		})

		// raw size = 5000 * 0.25 * 0.05 * 10 = 625 USD
		// shares = floor(625 / 0.50) = 1250
		// result = 1250 * 0.50 = 625 (already whole shares at this price)
		size := svc.ComputeTargetSizeUSD(0.05, 5000, 0, price, 0, false)
		expected := 625.0
		if size != expected {
			t.Errorf("expected size=%f, got %f", expected, size)
		}

		// Test with a price that doesn't divide evenly
		// raw size = 625 USD, price = 0.30
		// shares = floor(625 / 0.30) = floor(2083.33) = 2083
		// result = 2083 * 0.30 = 624.9
		size2 := svc.ComputeTargetSizeUSD(0.05, 5000, 0, 0.30, 0, false)
		expectedShares := math.Floor(625.0 / 0.30)
		expected2 := expectedShares * 0.30
		if math.Abs(size2-expected2) > 0.001 {
			t.Errorf("expected size=%f, got %f", expected2, size2)
		}
	})

	t.Run("returns_zero_for_zero_price", func(t *testing.T) {
		svc := NewRiskService(RiskConfig{
			MaxPositionUSDPerMarket: 1000,
			FractionalKelly:         0.25,
			MinTradeSizeUSD:         1.0,
			MinTradeShares:          5,
		})

		size := svc.ComputeTargetSizeUSD(0.05, 10000, 0, 0, 0, false)
		if size != 0 {
			t.Errorf("expected 0 for zero price, got %f", size)
		}
	})
}
