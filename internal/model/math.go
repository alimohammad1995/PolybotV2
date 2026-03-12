package model

import "math"

func NormalCDF(x float64) float64 {
	return 0.5 * (1.0 + math.Erf(x/math.Sqrt2))
}

func Clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}
