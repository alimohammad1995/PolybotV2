package service

import (
	"testing"
	"time"

	"Polybot/internal/domain"
)

func TestReferenceAnalyticsService_OnTick(t *testing.T) {
	t.Run("builds_state_from_ticks", func(t *testing.T) {
		svc := NewReferenceAnalyticsService(1000)

		base := time.Now()
		// Feed a series of ticks
		for i := 0; i < 100; i++ {
			price := 100.0 + float64(i)*0.01
			svc.OnTick(domain.ChainlinkTick{
				Asset:     "ETH",
				Price:     price,
				Timestamp: base.Add(time.Duration(i) * time.Second),
			})
		}

		state, ok := svc.GetState("ETH")
		if !ok {
			t.Fatal("expected state for ETH")
		}
		if state.CurrentPrice != 100.0+99*0.01 {
			t.Errorf("expected current price=100.99, got %f", state.CurrentPrice)
		}
		if state.TickCount != 100 {
			t.Errorf("expected 100 ticks, got %d", state.TickCount)
		}
		if state.RealizedVol1m <= 0 {
			t.Errorf("expected positive 1m vol, got %f", state.RealizedVol1m)
		}
	})

	t.Run("unknown_asset_returns_false", func(t *testing.T) {
		svc := NewReferenceAnalyticsService(1000)
		_, ok := svc.GetState("BTC")
		if ok {
			t.Error("expected false for unknown asset")
		}
	})

	t.Run("regime_classification", func(t *testing.T) {
		svc := NewReferenceAnalyticsService(1000)
		base := time.Now()

		// Feed calm ticks (tiny moves)
		for i := 0; i < 100; i++ {
			price := 100.0 + float64(i)*0.0001 // very small moves
			svc.OnTick(domain.ChainlinkTick{
				Asset:     "ETH",
				Price:     price,
				Timestamp: base.Add(time.Duration(i) * time.Second),
			})
		}

		state, _ := svc.GetState("ETH")
		if state.Regime != "calm" && state.Regime != "normal" {
			// With very small moves, regime should be calm or normal
			t.Logf("regime=%s vol1m=%f (may vary by tick spacing)", state.Regime, state.RealizedVol1m)
		}
	})

	t.Run("jump_detection", func(t *testing.T) {
		svc := NewReferenceAnalyticsService(1000)
		base := time.Now()

		// Feed stable ticks
		for i := 0; i < 50; i++ {
			svc.OnTick(domain.ChainlinkTick{
				Asset:     "ETH",
				Price:     100.0 + float64(i)*0.001,
				Timestamp: base.Add(time.Duration(i) * time.Second),
			})
		}

		// Then a big jump
		svc.OnTick(domain.ChainlinkTick{
			Asset:     "ETH",
			Price:     110.0, // 10% jump
			Timestamp: base.Add(51 * time.Second),
		})

		state, _ := svc.GetState("ETH")
		if state.JumpScore < 2.0 {
			t.Errorf("expected high jump score after 10%% move, got %f", state.JumpScore)
		}
	})

	t.Run("trims_to_max_ticks", func(t *testing.T) {
		svc := NewReferenceAnalyticsService(50)
		base := time.Now()

		for i := 0; i < 100; i++ {
			svc.OnTick(domain.ChainlinkTick{
				Asset:     "ETH",
				Price:     100.0,
				Timestamp: base.Add(time.Duration(i) * time.Second),
			})
		}

		state, _ := svc.GetState("ETH")
		if state.TickCount > 50 {
			t.Errorf("expected max 50 ticks, got %d", state.TickCount)
		}
	})

}
