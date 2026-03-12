package model

import (
	"context"
	"math"
	"testing"

	"Polybot/internal/domain"
)

// mockGaussianParamSource is a test double for GaussianParamSource.
type mockGaussianParamSource struct {
	params GaussianParams
	err    error
}

func (m *mockGaussianParamSource) GetParams(_ context.Context, _ float64) (GaussianParams, error) {
	return m.params, m.err
}

func TestGaussianModel_FairProbUp(t *testing.T) {
	ctx := context.Background()

	t.Run("current_price_above_beat_with_low_vol_gives_prob_above_half", func(t *testing.T) {
		source := &mockGaussianParamSource{
			params: GaussianParams{
				MeanLogReturn: 0.0,
				StdLogReturn:  0.01, // low vol
				Uncertainty:   0.02,
			},
		}
		m := NewGaussianModel(source)
		in := domain.PricingInput{
			CurrentPrice:     105.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 3600,
		}

		fv, err := m.FairProbUp(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fv.ProbUp <= 0.5 {
			t.Errorf("expected ProbUp > 0.5, got %f", fv.ProbUp)
		}
		if fv.ProbUpLower >= fv.ProbUp {
			t.Errorf("expected ProbUpLower < ProbUp, got lower=%f prob=%f", fv.ProbUpLower, fv.ProbUp)
		}
		if fv.ProbUpUpper <= fv.ProbUp {
			t.Errorf("expected ProbUpUpper > ProbUp, got upper=%f prob=%f", fv.ProbUpUpper, fv.ProbUp)
		}
	})

	t.Run("current_equals_beat_symmetric_params_gives_prob_near_half", func(t *testing.T) {
		source := &mockGaussianParamSource{
			params: GaussianParams{
				MeanLogReturn: 0.0, // symmetric: no drift
				StdLogReturn:  0.05,
				Uncertainty:   0.01,
			},
		}
		m := NewGaussianModel(source)
		in := domain.PricingInput{
			CurrentPrice:     100.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 3600,
		}

		fv, err := m.FairProbUp(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if math.Abs(fv.ProbUp-0.5) > 0.01 {
			t.Errorf("expected ProbUp near 0.5, got %f", fv.ProbUp)
		}
	})

	t.Run("remaining_zero_gives_step_function", func(t *testing.T) {
		source := &mockGaussianParamSource{} // params won't be called
		m := NewGaussianModel(source)

		// current > beat => prob = 1.0
		fv, err := m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     110.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fv.ProbUp != 1.0 {
			t.Errorf("expected ProbUp=1.0 when current > beat at expiry, got %f", fv.ProbUp)
		}

		// current < beat => prob = 0.0
		fv, err = m.FairProbUp(ctx, domain.PricingInput{
			CurrentPrice:     90.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 0,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fv.ProbUp != 0.0 {
			t.Errorf("expected ProbUp=0.0 when current < beat at expiry, got %f", fv.ProbUp)
		}
	})

	t.Run("higher_vol_moves_probability_toward_half", func(t *testing.T) {
		in := domain.PricingInput{
			CurrentPrice:     102.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 3600,
		}

		lowVolSource := &mockGaussianParamSource{
			params: GaussianParams{
				MeanLogReturn: 0.0,
				StdLogReturn:  0.01,
				Uncertainty:   0.0,
			},
		}
		highVolSource := &mockGaussianParamSource{
			params: GaussianParams{
				MeanLogReturn: 0.0,
				StdLogReturn:  1.0, // much higher vol
				Uncertainty:   0.0,
			},
		}

		fvLow, err := NewGaussianModel(lowVolSource).FairProbUp(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		fvHigh, err := NewGaussianModel(highVolSource).FairProbUp(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// With price above beat and zero mean, both should be > 0.5.
		// Higher vol should push the probability closer to 0.5.
		distLow := math.Abs(fvLow.ProbUp - 0.5)
		distHigh := math.Abs(fvHigh.ProbUp - 0.5)
		if distHigh >= distLow {
			t.Errorf("expected higher vol to move prob closer to 0.5: lowVol=%f highVol=%f",
				fvLow.ProbUp, fvHigh.ProbUp)
		}
	})

	t.Run("invalid_prices_return_error", func(t *testing.T) {
		source := &mockGaussianParamSource{}
		m := NewGaussianModel(source)

		cases := []domain.PricingInput{
			{CurrentPrice: 0, PriceToBeat: 100, RemainingSeconds: 3600},
			{CurrentPrice: -1, PriceToBeat: 100, RemainingSeconds: 3600},
			{CurrentPrice: 100, PriceToBeat: 0, RemainingSeconds: 3600},
			{CurrentPrice: 100, PriceToBeat: -5, RemainingSeconds: 3600},
		}

		for _, c := range cases {
			_, err := m.FairProbUp(ctx, c)
			if err == nil {
				t.Errorf("expected error for input %+v, got nil", c)
			}
		}
	})

	t.Run("zero_std_returns_error", func(t *testing.T) {
		source := &mockGaussianParamSource{
			params: GaussianParams{
				MeanLogReturn: 0.0,
				StdLogReturn:  0.0, // invalid
				Uncertainty:   0.01,
			},
		}
		m := NewGaussianModel(source)
		in := domain.PricingInput{
			CurrentPrice:     100.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 3600,
		}

		_, err := m.FairProbUp(ctx, in)
		if err == nil {
			t.Fatal("expected error for zero StdLogReturn, got nil")
		}
	})
}
