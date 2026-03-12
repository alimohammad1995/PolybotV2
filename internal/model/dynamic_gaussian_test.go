package model

import (
	"context"
	"math"
	"testing"

	"Polybot/internal/domain"
)

func TestDynamicGaussianModel_FairProbUp(t *testing.T) {
	ctx := context.Background()

	t.Run("uses_chainlink_1m_vol", func(t *testing.T) {
		m := NewDynamicGaussianModel(0.001, 0.02)
		fv, err := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     105.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 300,
			RealizedVol1m:    0.001,
			RealizedVol5m:    0.0008,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fv.ProbUp <= 0.5 {
			t.Errorf("expected ProbUp > 0.5 when price above beat, got %f", fv.ProbUp)
		}
	})

	t.Run("falls_back_to_5m_vol_when_1m_zero", func(t *testing.T) {
		m := NewDynamicGaussianModel(0.001, 0.02)
		fv, err := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     105.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 300,
			RealizedVol1m:    0,
			RealizedVol5m:    0.0008,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fv.ProbUp <= 0.5 {
			t.Errorf("expected ProbUp > 0.5, got %f", fv.ProbUp)
		}
	})

	t.Run("falls_back_to_default_vol_when_both_zero", func(t *testing.T) {
		m := NewDynamicGaussianModel(0.001, 0.02)
		fv, err := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     105.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 300,
			RealizedVol1m:    0,
			RealizedVol5m:    0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should still work with default vol
		if fv.ProbUp <= 0.5 {
			t.Errorf("expected ProbUp > 0.5, got %f", fv.ProbUp)
		}
		// Uncertainty should be widened due to missing vol data
		if fv.ModelUncertainty <= 0.02 {
			t.Errorf("expected wider uncertainty with no vol data, got %f", fv.ModelUncertainty)
		}
	})

	t.Run("remaining_zero_gives_step_function", func(t *testing.T) {
		m := NewDynamicGaussianModel(0.001, 0.02)

		fv, err := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     110.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fv.ProbUp != 1.0 {
			t.Errorf("expected 1.0 at expiry when above, got %f", fv.ProbUp)
		}

		fv, err = m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     90.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fv.ProbUp != 0.0 {
			t.Errorf("expected 0.0 at expiry when below, got %f", fv.ProbUp)
		}
	})

	t.Run("jump_widens_uncertainty", func(t *testing.T) {
		m := NewDynamicGaussianModel(0.001, 0.02)

		noJump, _ := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     100.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 300,
			RealizedVol1m:    0.001,
			JumpScore:        0.5,
		})
		withJump, _ := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     100.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 300,
			RealizedVol1m:    0.001,
			JumpScore:        5.0,
		})

		if withJump.ModelUncertainty <= noJump.ModelUncertainty {
			t.Errorf("expected jump to widen uncertainty: noJump=%f withJump=%f",
				noJump.ModelUncertainty, withJump.ModelUncertainty)
		}
	})

	t.Run("near_expiry_widens_uncertainty", func(t *testing.T) {
		m := NewDynamicGaussianModel(0.001, 0.02)

		far, _ := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     100.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 300,
			RealizedVol1m:    0.001,
		})
		near, _ := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     100.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 30,
			RealizedVol1m:    0.001,
		})

		if near.ModelUncertainty <= far.ModelUncertainty {
			t.Errorf("expected near-expiry to widen uncertainty: far=%f near=%f",
				far.ModelUncertainty, near.ModelUncertainty)
		}
	})

	t.Run("higher_vol_moves_prob_toward_half", func(t *testing.T) {
		m := NewDynamicGaussianModel(0.001, 0.0)

		lowVol, _ := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     102.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 300,
			RealizedVol1m:    0.0001,
		})
		highVol, _ := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     102.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 300,
			RealizedVol1m:    0.01,
		})

		distLow := math.Abs(lowVol.ProbUp - 0.5)
		distHigh := math.Abs(highVol.ProbUp - 0.5)
		if distHigh >= distLow {
			t.Errorf("higher vol should push prob closer to 0.5: lowVol=%f highVol=%f",
				lowVol.ProbUp, highVol.ProbUp)
		}
	})

	t.Run("regime_is_passed_through", func(t *testing.T) {
		m := NewDynamicGaussianModel(0.001, 0.02)
		fv, _ := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     100.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 300,
			RealizedVol1m:    0.001,
			Regime:           "volatile",
		})
		if fv.ModelRegime != "volatile" {
			t.Errorf("expected regime=volatile, got %s", fv.ModelRegime)
		}
	})
}
