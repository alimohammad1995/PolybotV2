package main

import (
	"encoding/json"
	"strconv"
)

func parseFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
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
