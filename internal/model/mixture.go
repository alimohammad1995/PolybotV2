package model

import (
	"context"
	"fmt"
	"math"
	"time"

	"Polybot/internal/domain"
)

type MixtureComponent struct {
	Weight        float64
	MeanLogReturn float64
	StdLogReturn  float64
}

type MixtureParams struct {
	Components  []MixtureComponent
	Uncertainty float64
}

type MixtureParamSource interface {
	GetMixtureParams(ctx context.Context, asset string, remainingSeconds float64) (MixtureParams, error)
}

type MixtureModel struct {
	Asset       string
	ParamSource MixtureParamSource
}

func NewMixtureModel(asset string, source MixtureParamSource) *MixtureModel {
	return &MixtureModel{Asset: asset, ParamSource: source}
}

func (m *MixtureModel) FairProbUp(ctx context.Context, in domain.PricingInput) (domain.FairValue, error) {
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

	params, err := m.ParamSource.GetMixtureParams(ctx, m.Asset, in.RemainingSeconds)
	if err != nil {
		return domain.FairValue{}, fmt.Errorf("get mixture params: %w", err)
	}

	logMoneyness := math.Log(in.CurrentPrice / in.PriceToBeat)

	var p float64
	for _, c := range params.Components {
		if c.StdLogReturn <= 0 || c.Weight < 0 {
			continue
		}
		z := (logMoneyness + c.MeanLogReturn) / c.StdLogReturn
		p += c.Weight * NormalCDF(z)
	}

	p = Clamp01(p)
	lower := Clamp01(p - params.Uncertainty)
	upper := Clamp01(p + params.Uncertainty)

	return domain.FairValue{
		ProbUp:           p,
		ProbUpLower:      lower,
		ProbUpUpper:      upper,
		ModelUncertainty: params.Uncertainty,
		RemainingSeconds: in.RemainingSeconds,
		RequiredLogMove:  math.Log(in.PriceToBeat / in.CurrentPrice),
		ModelRegime:      in.Regime,
		Timestamp:        time.Now(),
	}, nil
}
