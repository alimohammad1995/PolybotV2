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
	// Calibration is an optional isotonic calibration map (p_raw -> p_cal)
	Calibration *CalibrationMap
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
			ProbRaw:          p,
			ProbCalibrated:   p,
			ModelUncertainty: 0,
			RemainingSeconds: 0,
			RequiredLogMove:  -logMoneyness,
			Timestamp:        time.Now(),
		}, nil
	}

	// Use Chainlink 1m realized vol (per-second), fall back to 5m, then default
	perSecVol := in.RealizedVol1m
	if perSecVol <= 0 {
		perSecVol = in.RealizedVol5m
	}
	if perSecVol <= 0 {
		perSecVol = m.DefaultVol
	}

	// Vol floor: BTC annual ~60% => per-second ~0.0001.
	// During calm periods measured vol can be unrealistically low; apply a floor
	// so z-scores stay reasonable.
	const volFloorPerSec = 0.00005
	if perSecVol < volFloorPerSec {
		perSecVol = volFloorPerSec
	}

	// Scale per-second vol to remaining horizon: σ_τ = σ_sec * √τ
	horizonStd := perSecVol * math.Sqrt(in.RemainingSeconds)

	if horizonStd <= 0 {
		return domain.FairValue{}, fmt.Errorf("computed horizon std is zero")
	}

	// p_up = Φ(log(S/K) / σ̂_τ)
	z := logMoneyness / horizonStd
	pRaw := NormalCDF(z)

	// Apply isotonic calibration if available
	pCal := pRaw
	if m.Calibration != nil {
		pCal = m.Calibration.Calibrate(pRaw, in.RemainingSeconds, perSecVol)
	}

	// Compute dynamic uncertainty from calibrated probability
	uncertainty := m.computeUncertainty(in)
	lower := Clamp01(pCal - uncertainty)
	upper := Clamp01(pCal + uncertainty)

	return domain.FairValue{
		ProbUp:           Clamp01(pCal),
		ProbUpLower:      lower,
		ProbUpUpper:      upper,
		ProbRaw:          Clamp01(pRaw),
		ProbCalibrated:   Clamp01(pCal),
		SigmaTau:         horizonStd,
		ZScore:           z,
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
