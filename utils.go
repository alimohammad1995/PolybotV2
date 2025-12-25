package main

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

func parseFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	case float64:
		return v, true
	default:
		return 0, false
	}
}

func priceIndexFromString(value string) (int, bool) {
	if value == "" || value == "<nil>" {
		return 0, false
	}
	intPart := 0
	frac := 0
	fracDigits := 0
	i := 0
	n := len(value)
	for i < n && value[i] != '.' {
		ch := value[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		intPart = intPart*10 + int(ch-'0')
		i++
	}
	if i == n {
		return intPart * 100, true
	}
	if value[i] != '.' {
		return 0, false
	}
	i++
	for i < n && fracDigits < 2 {
		ch := value[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		frac = frac*10 + int(ch-'0')
		fracDigits++
		i++
	}
	if fracDigits == 1 {
		frac *= 10
	}
	return intPart*100 + frac, true
}

func intAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func printJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func clampFloat(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func floatEq(a, b float64) bool { return math.Abs(a-b) < eps }
func floatGt(a, b float64) bool { return a-b > eps }
func floatLt(a, b float64) bool { return b-a > eps }
