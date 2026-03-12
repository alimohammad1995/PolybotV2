package model

import (
	"context"
	"fmt"
	"math"
	"time"

	"Polybot/internal/domain"
)

type GaussianParams struct {
	MeanLogReturn float64
	StdLogReturn  float64
	Uncertainty   float64
}

type GaussianParamSource interface {
	GetParams(ctx context.Context, remainingSeconds float64) (GaussianParams, error)
}

type GaussianModel struct {
	ParamSource GaussianParamSource
}

func NewGaussianModel(source GaussianParamSource) *GaussianModel {
	return &GaussianModel{ParamSource: source}
}

func (m *GaussianModel) FairProbUp(ctx context.Context, in domain.PricingInput) (domain.FairValue, error) {
	if in.CurrentPrice <= 0 || in.PriceToBeat <= 0 {
		return domain.FairValue{}, fmt.Errorf("invalid prices: current=%f beat=%f", in.CurrentPrice, in.PriceToBeat)
	}
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
			RequiredLogMove:  math.Log(in.PriceToBeat / in.CurrentPrice),
			Timestamp:        time.Now(),
		}, nil
	}

	params, err := m.ParamSource.GetParams(ctx, in.RemainingSeconds)
	if err != nil {
		return domain.FairValue{}, fmt.Errorf("get gaussian params: %w", err)
	}
	if params.StdLogReturn <= 0 {
		return domain.FairValue{}, fmt.Errorf("invalid StdLogReturn: %f", params.StdLogReturn)
	}

	logMoneyness := math.Log(in.CurrentPrice / in.PriceToBeat)
	z := (logMoneyness + params.MeanLogReturn) / params.StdLogReturn
	p := NormalCDF(z)

	lower := Clamp01(p - params.Uncertainty)
	upper := Clamp01(p + params.Uncertainty)

	return domain.FairValue{
		ProbUp:           Clamp01(p),
		ProbUpLower:      lower,
		ProbUpUpper:      upper,
		ModelUncertainty: params.Uncertainty,
		RemainingSeconds: in.RemainingSeconds,
		RequiredLogMove:  math.Log(in.PriceToBeat / in.CurrentPrice),
		ModelRegime:      in.Regime,
		Timestamp:        time.Now(),
	}, nil
}
