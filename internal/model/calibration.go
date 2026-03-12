package model

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"
)

// StaticGaussianParamSource returns fixed params scaled by remaining time.
type StaticGaussianParamSource struct {
	AnnualizedVol float64
	Drift         float64
	Uncertainty   float64
}

func (s *StaticGaussianParamSource) GetParams(_ context.Context, remainingSeconds float64) (GaussianParams, error) {
	tau := remainingSeconds / (365.25 * 24 * 3600)
	return GaussianParams{
		MeanLogReturn: (s.Drift - 0.5*s.AnnualizedVol*s.AnnualizedVol) * tau,
		StdLogReturn:  s.AnnualizedVol * math.Sqrt(tau),
		Uncertainty:   s.Uncertainty,
	}, nil
}

// SimpleMixtureParamSource uses current vol to assign regime weights.
type SimpleMixtureParamSource struct {
	VolatilityGetter VolatilityGetter
}

type VolatilityGetter interface {
	GetCurrentVol(ctx context.Context, asset string) (float64, error)
}

func (s *SimpleMixtureParamSource) GetMixtureParams(ctx context.Context, asset string, remainingSeconds float64) (MixtureParams, error) {
	currentVol, err := s.VolatilityGetter.GetCurrentVol(ctx, asset)
	if err != nil {
		return MixtureParams{}, fmt.Errorf("get current vol: %w", err)
	}

	calmStd := 0.0015 * math.Sqrt(remainingSeconds/60.0)
	volStd := 0.0035 * math.Sqrt(remainingSeconds/60.0)

	var wCalm, wVol float64
	switch {
	case currentVol < 0.001:
		wCalm, wVol = 0.8, 0.2
	case currentVol < 0.002:
		wCalm, wVol = 0.5, 0.5
	default:
		wCalm, wVol = 0.2, 0.8
	}

	return MixtureParams{
		Components: []MixtureComponent{
			{Weight: wCalm, MeanLogReturn: 0, StdLogReturn: calmStd},
			{Weight: wVol, MeanLogReturn: 0, StdLogReturn: volStd},
		},
		Uncertainty: 0.02,
	}, nil
}

// FileBasedMixtureParamSource loads calibrated params from a JSON file.
type FileBasedMixtureParamSource struct {
	mu     sync.RWMutex
	params map[string]map[string]MixtureParams // asset -> horizon_bucket -> params
}

type fileParamEntry struct {
	Components []struct {
		Weight        float64 `json:"weight"`
		MeanLogReturn float64 `json:"mean_log_return"`
		StdLogReturn  float64 `json:"std_log_return"`
	} `json:"components"`
	Uncertainty float64 `json:"uncertainty"`
}

func NewFileBasedMixtureParamSource(path string) (*FileBasedMixtureParamSource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read param file: %w", err)
	}

	var raw map[string]map[string]fileParamEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse param file: %w", err)
	}

	result := make(map[string]map[string]MixtureParams)
	for asset, horizons := range raw {
		result[asset] = make(map[string]MixtureParams)
		for horizon, entry := range horizons {
			mp := MixtureParams{Uncertainty: entry.Uncertainty}
			for _, c := range entry.Components {
				mp.Components = append(mp.Components, MixtureComponent{
					Weight:        c.Weight,
					MeanLogReturn: c.MeanLogReturn,
					StdLogReturn:  c.StdLogReturn,
				})
			}
			result[asset][horizon] = mp
		}
	}

	return &FileBasedMixtureParamSource{params: result}, nil
}

func (f *FileBasedMixtureParamSource) GetMixtureParams(_ context.Context, asset string, remainingSeconds float64) (MixtureParams, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	horizons, ok := f.params[asset]
	if !ok {
		return MixtureParams{}, fmt.Errorf("no params for asset %s", asset)
	}

	// Find closest horizon bucket
	bucket := closestBucket(remainingSeconds, horizons)
	params, ok := horizons[bucket]
	if !ok {
		return MixtureParams{}, fmt.Errorf("no params for asset %s bucket %s", asset, bucket)
	}
	return params, nil
}

func closestBucket(remainingSeconds float64, horizons map[string]MixtureParams) string {
	var best string
	bestDiff := math.MaxFloat64
	for k := range horizons {
		var bucket float64
		fmt.Sscanf(k, "%f", &bucket)
		diff := math.Abs(bucket - remainingSeconds)
		if diff < bestDiff {
			bestDiff = diff
			best = k
		}
	}
	return best
}
