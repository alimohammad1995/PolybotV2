package model

import (
	"context"
	"fmt"
	"math"
	"time"

	"Polybot/internal/domain"
)

// DynamicGaussianModel computes fair probability using Chainlink-derived
// realized volatility scaled to the remaining horizon.
//
// p_up = Φ(log(S/K) / σ̂_τ)
//
// where σ̂_τ is estimated from recent Chainlink vol and scaled to remaining time.
// This is the recommended V1 model — simpler and more grounded than a static mixture.
type DynamicGaussianModel struct {
	// DefaultVol is used when Chainlink has insufficient data
	DefaultVol float64
	// BaseUncertainty is the minimum model uncertainty
	BaseUncertainty float64
}

func NewDynamicGaussianModel(defaultVol, baseUncertainty float64) *DynamicGaussianModel {
	return &DynamicGaussianModel{
		DefaultVol:      defaultVol,
		BaseUncertainty: baseUncertainty,
	}
}

func (m *DynamicGaussianModel) FairProbUp(_ context.Context, in domain.PricingInput) (domain.FairValue, error) {
	if in.CurrentPrice <= 0 || in.PriceToBeat <= 0 {
		return domain.FairValue{}, fmt.Errorf("invalid prices: current=%f beat=%f", in.CurrentPrice, in.PriceToBeat)
	}

	logMoneyness := math.Log(in.CurrentPrice / in.PriceToBeat)

	if in.RemainingSeconds <= 0 {
		p := 0.0
		if in.CurrentPrice > in.PriceToBeat {
			p = 1.0
		}
		return domain.FairValue{
			ProbUp:           p,
			ProbUpLower:      p,
			ProbUpUpper:      p,
			ModelUncertainty: 0,
			RemainingSeconds: 0,
			RequiredLogMove:  -logMoneyness,
			Timestamp:        time.Now(),
		}, nil
	}

	// Use Chainlink 1m realized vol, fall back to 5m, then default
	tickVol := in.RealizedVol1m
	if tickVol <= 0 {
		tickVol = in.RealizedVol5m
	}
	if tickVol <= 0 {
		tickVol = m.DefaultVol
	}

	// Scale tick-level vol to remaining horizon
	// Assume tick vol is per-second std. Scale by sqrt(remaining seconds).
	// If ticks are ~1 second apart, vol is already per-tick ≈ per-second.
	horizonStd := tickVol * math.Sqrt(in.RemainingSeconds)

	if horizonStd <= 0 {
		return domain.FairValue{}, fmt.Errorf("computed horizon std is zero")
	}

	// p_up = Φ(log(S/K) / σ̂_τ)
	// No drift term — conservative assumption for short horizons
	z := logMoneyness / horizonStd
	p := NormalCDF(z)

	// Compute dynamic uncertainty
	uncertainty := m.computeUncertainty(in)

	lower := Clamp01(p - uncertainty)
	upper := Clamp01(p + uncertainty)

	return domain.FairValue{
		ProbUp:           Clamp01(p),
		ProbUpLower:      lower,
		ProbUpUpper:      upper,
		ModelUncertainty: uncertainty,
		RemainingSeconds: in.RemainingSeconds,
		RequiredLogMove:  -logMoneyness,
		ModelRegime:      in.Regime,
		Timestamp:        time.Now(),
	}, nil
}

func (m *DynamicGaussianModel) computeUncertainty(in domain.PricingInput) float64 {
	unc := m.BaseUncertainty

	// Widen uncertainty if vol estimate is unstable (1m vs 5m divergence)
	if in.RealizedVol5m > 0 && in.RealizedVol1m > 0 {
		ratio := in.RealizedVol1m / in.RealizedVol5m
		if ratio > 2.0 || ratio < 0.5 {
			unc += 0.01 // vol regime is shifting
		}
	}

	// Widen if jump detected
	if in.JumpScore > 3.0 {
		unc += 0.02
	}

	// Widen if too few ticks (vol estimate unreliable)
	if in.RealizedVol1m <= 0 && in.RealizedVol5m <= 0 {
		unc += 0.03
	}

	// Widen near expiry (model less reliable)
	if in.RemainingSeconds < 60 {
		unc += 0.02
	}

	return unc
}
