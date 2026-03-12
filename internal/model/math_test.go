package model

import (
	"math"
	"testing"
)

func TestNormalCDF(t *testing.T) {
	t.Run("zero_gives_half", func(t *testing.T) {
		result := NormalCDF(0)
		if math.Abs(result-0.5) > 1e-15 {
			t.Errorf("NormalCDF(0) = %f, want 0.5", result)
		}
	})

	t.Run("symmetry", func(t *testing.T) {
		values := []float64{0.5, 1.0, 1.5, 2.0, 3.0}
		for _, x := range values {
			pos := NormalCDF(x)
			neg := NormalCDF(-x)
			sum := pos + neg
			if math.Abs(sum-1.0) > 1e-12 {
				t.Errorf("NormalCDF(%f) + NormalCDF(%f) = %f, want 1.0", x, -x, sum)
			}
		}
	})

	t.Run("known_values", func(t *testing.T) {
		// Standard normal CDF known values
		cases := []struct {
			x    float64
			want float64
			tol  float64
		}{
			{1.0, 0.8413447, 1e-6},
			{-1.0, 0.1586553, 1e-6},
			{2.0, 0.9772499, 1e-6},
			{-2.0, 0.0227501, 1e-6},
		}
		for _, c := range cases {
			got := NormalCDF(c.x)
			if math.Abs(got-c.want) > c.tol {
				t.Errorf("NormalCDF(%f) = %f, want ~%f", c.x, got, c.want)
			}
		}
	})

	t.Run("monotonically_increasing", func(t *testing.T) {
		prev := NormalCDF(-5.0)
		for x := -4.5; x <= 5.0; x += 0.5 {
			curr := NormalCDF(x)
			if curr <= prev {
				t.Errorf("NormalCDF not monotonic: NormalCDF(%f)=%f <= NormalCDF(%f)=%f",
					x, curr, x-0.5, prev)
			}
			prev = curr
		}
	})

	t.Run("extreme_values", func(t *testing.T) {
		if NormalCDF(10) < 0.999999 {
			t.Errorf("NormalCDF(10) should be very close to 1, got %f", NormalCDF(10))
		}
		if NormalCDF(-10) > 0.000001 {
			t.Errorf("NormalCDF(-10) should be very close to 0, got %f", NormalCDF(-10))
		}
	})
}

func TestClamp01(t *testing.T) {
	t.Run("clamps_below_zero", func(t *testing.T) {
		if Clamp01(-0.5) != 0 {
			t.Errorf("Clamp01(-0.5) = %f, want 0", Clamp01(-0.5))
		}
		if Clamp01(-100) != 0 {
			t.Errorf("Clamp01(-100) = %f, want 0", Clamp01(-100))
		}
	})

	t.Run("clamps_above_one", func(t *testing.T) {
		if Clamp01(1.5) != 1 {
			t.Errorf("Clamp01(1.5) = %f, want 1", Clamp01(1.5))
		}
		if Clamp01(100) != 1 {
			t.Errorf("Clamp01(100) = %f, want 1", Clamp01(100))
		}
	})

	t.Run("passes_through_valid_values", func(t *testing.T) {
		values := []float64{0, 0.25, 0.5, 0.75, 1.0}
		for _, v := range values {
			if Clamp01(v) != v {
				t.Errorf("Clamp01(%f) = %f, want %f", v, Clamp01(v), v)
			}
		}
	})

	t.Run("boundary_values", func(t *testing.T) {
		if Clamp01(0) != 0 {
			t.Errorf("Clamp01(0) = %f, want 0", Clamp01(0))
		}
		if Clamp01(1) != 1 {
			t.Errorf("Clamp01(1) = %f, want 1", Clamp01(1))
		}
	})
}
