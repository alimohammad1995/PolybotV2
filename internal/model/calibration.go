package model

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"
)

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
