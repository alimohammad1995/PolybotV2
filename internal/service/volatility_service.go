package service

import (
	"context"

	"Polybot/internal/domain"
)

// VolatilityService is a backward-compatible wrapper that delegates to ReferenceAnalyticsService.
// It preserves the GetCurrentVol interface used by existing callers.
type VolatilityService struct {
	analytics *ReferenceAnalyticsService
}

func NewVolatilityService(maxSamples int) *VolatilityService {
	return &VolatilityService{
		analytics: NewReferenceAnalyticsService(maxSamples),
	}
}

// AddSample converts a ReferenceSnapshot into a ChainlinkTick and feeds it to the analytics engine.
func (v *VolatilityService) AddSample(snap domain.ReferenceSnapshot) {
	v.analytics.OnTick(domain.ChainlinkTick{
		Asset:     snap.Asset,
		Price:     snap.Price,
		Timestamp: snap.Timestamp,
	})
}

// GetCurrentVol delegates to the underlying ReferenceAnalyticsService.
func (v *VolatilityService) GetCurrentVol(ctx context.Context, asset string) (float64, error) {
	return v.analytics.GetCurrentVol(ctx, asset)
}

// GetReferenceState exposes the full analytics state for callers that need it.
func (v *VolatilityService) GetReferenceState(asset string) (domain.ReferenceState, bool) {
	return v.analytics.GetState(asset)
}
