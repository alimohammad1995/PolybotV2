package model

import (
	"context"
	"math"
	"testing"

	"Polybot/internal/domain"
)

// mockMixtureParamSource is a test double for MixtureParamSource.
type mockMixtureParamSource struct {
	params MixtureParams
	err    error
}

func (m *mockMixtureParamSource) GetMixtureParams(_ context.Context, _ string, _ float64) (MixtureParams, error) {
	return m.params, m.err
}

func TestMixtureModel_FairProbUp(t *testing.T) {
	ctx := context.Background()

	t.Run("two_regime_mixture_with_known_params", func(t *testing.T) {
		// Two components with equal weight:
		// Component 1: bullish drift, low vol
		// Component 2: bearish drift, low vol
		source := &mockMixtureParamSource{
			params: MixtureParams{
				Components: []MixtureComponent{
					{Weight: 0.5, MeanLogReturn: 0.05, StdLogReturn: 0.01},
					{Weight: 0.5, MeanLogReturn: -0.05, StdLogReturn: 0.01},
				},
				Uncertainty: 0.01,
			},
		}
		m := NewMixtureModel("BTC", source)
		in := domain.PricingInput{
			CurrentPrice:     100.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 3600,
		}

		fv, err := m.FairProbUp(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// With symmetric drifts and equal weights at the money, prob should be near 0.5
		if math.Abs(fv.ProbUp-0.5) > 0.05 {
			t.Errorf("expected ProbUp near 0.5 for symmetric mixture, got %f", fv.ProbUp)
		}
		if fv.ModelUncertainty != 0.01 {
			t.Errorf("expected ModelUncertainty=0.01, got %f", fv.ModelUncertainty)
		}
	})

	t.Run("single_component_degenerates_to_gaussian", func(t *testing.T) {
		mu := 0.0
		sigma := 0.05
		unc := 0.02

		gaussianSource := &mockGaussianParamSource{
			params: GaussianParams{
				MeanLogReturn: mu,
				StdLogReturn:  sigma,
				Uncertainty:   unc,
			},
		}
		mixtureSource := &mockMixtureParamSource{
			params: MixtureParams{
				Components: []MixtureComponent{
					{Weight: 1.0, MeanLogReturn: mu, StdLogReturn: sigma},
				},
				Uncertainty: unc,
			},
		}

		in := domain.PricingInput{
			CurrentPrice:     105.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 7200,
		}

		gm := NewGaussianModel(gaussianSource)
		mm := NewMixtureModel("ETH", mixtureSource)

		fvGauss, err := gm.FairProbUp(ctx, in)
		if err != nil {
			t.Fatalf("gaussian error: %v", err)
		}
		fvMix, err := mm.FairProbUp(ctx, in)
		if err != nil {
			t.Fatalf("mixture error: %v", err)
		}

		if math.Abs(fvGauss.ProbUp-fvMix.ProbUp) > 1e-9 {
			t.Errorf("single-component mixture should match gaussian: gauss=%f mix=%f",
				fvGauss.ProbUp, fvMix.ProbUp)
		}
		if math.Abs(fvGauss.ProbUpLower-fvMix.ProbUpLower) > 1e-9 {
			t.Errorf("lower bounds should match: gauss=%f mix=%f",
				fvGauss.ProbUpLower, fvMix.ProbUpLower)
		}
		if math.Abs(fvGauss.ProbUpUpper-fvMix.ProbUpUpper) > 1e-9 {
			t.Errorf("upper bounds should match: gauss=%f mix=%f",
				fvGauss.ProbUpUpper, fvMix.ProbUpUpper)
		}
	})

	t.Run("weights_sum_behavior", func(t *testing.T) {
		// Weights that sum to 1.0 should produce probabilities in [0,1]
		source := &mockMixtureParamSource{
			params: MixtureParams{
				Components: []MixtureComponent{
					{Weight: 0.3, MeanLogReturn: 0.01, StdLogReturn: 0.05},
					{Weight: 0.3, MeanLogReturn: -0.01, StdLogReturn: 0.05},
					{Weight: 0.4, MeanLogReturn: 0.0, StdLogReturn: 0.10},
				},
				Uncertainty: 0.0,
			},
		}
		m := NewMixtureModel("BTC", source)
		in := domain.PricingInput{
			CurrentPrice:     100.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 3600,
		}

		fv, err := m.FairProbUp(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fv.ProbUp < 0 || fv.ProbUp > 1 {
			t.Errorf("ProbUp out of [0,1]: %f", fv.ProbUp)
		}
	})

	t.Run("zero_weight_component_has_no_effect", func(t *testing.T) {
		base := &mockMixtureParamSource{
			params: MixtureParams{
				Components: []MixtureComponent{
					{Weight: 1.0, MeanLogReturn: 0.0, StdLogReturn: 0.05},
				},
				Uncertainty: 0.0,
			},
		}
		withZero := &mockMixtureParamSource{
			params: MixtureParams{
				Components: []MixtureComponent{
					{Weight: 1.0, MeanLogReturn: 0.0, StdLogReturn: 0.05},
					{Weight: 0.0, MeanLogReturn: 0.5, StdLogReturn: 0.01}, // zero weight, extreme drift
				},
				Uncertainty: 0.0,
			},
		}

		in := domain.PricingInput{
			CurrentPrice:     100.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 3600,
		}

		fvBase, _ := NewMixtureModel("BTC", base).FairProbUp(ctx, in)
		fvWithZero, _ := NewMixtureModel("BTC", withZero).FairProbUp(ctx, in)

		// Zero-weight component contributes nothing, but the implementation
		// skips negative weights only (c.Weight < 0). A zero weight contributes
		// 0 * NormalCDF(z) = 0, so the result should still match.
		if math.Abs(fvBase.ProbUp-fvWithZero.ProbUp) > 1e-9 {
			t.Errorf("zero-weight component should not affect result: base=%f withZero=%f",
				fvBase.ProbUp, fvWithZero.ProbUp)
		}
	})

	t.Run("zero_std_component_is_skipped", func(t *testing.T) {
		source := &mockMixtureParamSource{
			params: MixtureParams{
				Components: []MixtureComponent{
					{Weight: 0.5, MeanLogReturn: 0.0, StdLogReturn: 0.0},  // invalid std, skipped
					{Weight: 0.5, MeanLogReturn: 0.0, StdLogReturn: 0.05}, // valid
				},
				Uncertainty: 0.0,
			},
		}
		m := NewMixtureModel("BTC", source)
		in := domain.PricingInput{
			CurrentPrice:     100.0,
			PriceToBeat:      100.0,
			RemainingSeconds: 3600,
		}

		fv, err := m.FairProbUp(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Only the valid component (weight=0.5) contributes:
		// 0.5 * NormalCDF(0) = 0.5 * 0.5 = 0.25
		expected := 0.25
		if math.Abs(fv.ProbUp-expected) > 1e-9 {
			t.Errorf("expected ProbUp=%f with one skipped component, got %f", expected, fv.ProbUp)
		}
	})
}
