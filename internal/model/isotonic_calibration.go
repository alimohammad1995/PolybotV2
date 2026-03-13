package model

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
)

// CalibrationBucketKey identifies a calibration bucket by expiry and vol regime.
type CalibrationBucketKey struct {
	ExpiryBucket string // e.g. "60", "120", "300" seconds
	VolBucket    string // e.g. "low", "mid", "high"
}

// IsotonicFunction is a monotone non-decreasing piecewise-linear mapping from p_raw to p_cal.
type IsotonicFunction struct {
	XPoints []float64 // sorted p_raw breakpoints
	YPoints []float64 // corresponding p_cal values (monotone non-decreasing)
}

// Evaluate returns the calibrated probability for a given raw probability.
// Uses binary search + linear interpolation between breakpoints.
func (f *IsotonicFunction) Evaluate(pRaw float64) float64 {
	n := len(f.XPoints)
	if n == 0 {
		return pRaw // identity fallback
	}
	if pRaw <= f.XPoints[0] {
		return f.YPoints[0]
	}
	if pRaw >= f.XPoints[n-1] {
		return f.YPoints[n-1]
	}

	// Binary search for the interval containing pRaw
	i := sort.SearchFloat64s(f.XPoints, pRaw)
	if i == 0 {
		return f.YPoints[0]
	}
	if i >= n {
		return f.YPoints[n-1]
	}

	// Linear interpolation between breakpoints
	x0, x1 := f.XPoints[i-1], f.XPoints[i]
	y0, y1 := f.YPoints[i-1], f.YPoints[i]
	if x1 == x0 {
		return y0
	}
	t := (pRaw - x0) / (x1 - x0)
	return y0 + t*(y1-y0)
}

// CalibrationMap holds isotonic calibration functions bucketed by expiry and vol regime.
type CalibrationMap struct {
	Buckets      map[CalibrationBucketKey]*IsotonicFunction
	expiryBreaks []float64 // sorted expiry bucket boundaries
	volBreaks    []float64 // sorted vol bucket boundaries
}

// Calibrate returns the calibrated probability for a raw probability,
// given the remaining time and current volatility.
// Falls back to identity (returns pRaw) if no matching bucket exists.
func (c *CalibrationMap) Calibrate(pRaw, remainingSeconds, vol float64) float64 {
	key := c.findBucket(remainingSeconds, vol)
	fn, ok := c.Buckets[key]
	if !ok {
		return pRaw
	}
	return fn.Evaluate(pRaw)
}

func (c *CalibrationMap) findBucket(remainingSeconds, vol float64) CalibrationBucketKey {
	return CalibrationBucketKey{
		ExpiryBucket: findClosestBucketLabel(remainingSeconds, c.expiryBreaks),
		VolBucket:    classifyVolBucket(vol, c.volBreaks),
	}
}

func findClosestBucketLabel(value float64, breaks []float64) string {
	if len(breaks) == 0 {
		return "0"
	}
	best := breaks[0]
	bestDiff := math.Abs(value - best)
	for _, b := range breaks[1:] {
		d := math.Abs(value - b)
		if d < bestDiff {
			bestDiff = d
			best = b
		}
	}
	return fmt.Sprintf("%.0f", best)
}

func classifyVolBucket(vol float64, breaks []float64) string {
	if len(breaks) < 2 {
		return "mid"
	}
	if vol < breaks[0] {
		return "low"
	}
	if vol > breaks[len(breaks)-1] {
		return "high"
	}
	return "mid"
}

// JSON file format for loading calibration data
type calibrationFileEntry struct {
	ExpiryBucket string    `json:"expiry_bucket"`
	VolBucket    string    `json:"vol_bucket"`
	XPoints      []float64 `json:"x_points"`
	YPoints      []float64 `json:"y_points"`
}

type calibrationFile struct {
	ExpiryBreaks []float64              `json:"expiry_breaks"` // e.g. [60, 120, 300]
	VolBreaks    []float64              `json:"vol_breaks"`    // e.g. [0.0005, 0.002]
	Buckets      []calibrationFileEntry `json:"buckets"`
}

// NewCalibrationMapFromFile loads an isotonic calibration map from a JSON file.
func NewCalibrationMapFromFile(path string) (*CalibrationMap, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read calibration file: %w", err)
	}

	var cf calibrationFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parse calibration file: %w", err)
	}

	cm := &CalibrationMap{
		Buckets:      make(map[CalibrationBucketKey]*IsotonicFunction),
		expiryBreaks: cf.ExpiryBreaks,
		volBreaks:    cf.VolBreaks,
	}

	for _, entry := range cf.Buckets {
		if len(entry.XPoints) != len(entry.YPoints) {
			return nil, fmt.Errorf("bucket %s/%s: x_points and y_points length mismatch", entry.ExpiryBucket, entry.VolBucket)
		}
		key := CalibrationBucketKey{
			ExpiryBucket: entry.ExpiryBucket,
			VolBucket:    entry.VolBucket,
		}
		cm.Buckets[key] = &IsotonicFunction{
			XPoints: entry.XPoints,
			YPoints: entry.YPoints,
		}
	}

	return cm, nil
}
