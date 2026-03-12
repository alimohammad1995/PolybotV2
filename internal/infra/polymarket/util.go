package polymarket

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

func int64ToString(value int64) string {
	return strconv.FormatInt(value, 10)
}

func stringifyBody(body any) string {
	if body == nil {
		return ""
	}
	data, err := json.Marshal(body)
	if err == nil {
		return string(data)
	}
	return stringFromAny(body)
}

func IsTickSizeSmaller(a, b string) bool {
	fa, _ := strconv.ParseFloat(a, 64)
	fb, _ := strconv.ParseFloat(b, 64)
	return fa < fb
}

func PriceValid(price float64, tickSize string) bool {
	ts, _ := strconv.ParseFloat(tickSize, 64)
	return price >= ts && price <= 1-ts
}

func RoundDown(x float64, sigDigits int) float64 {
	factor := math.Pow10(sigDigits)
	return math.Floor(x*factor) / factor
}

func RoundNormal(x float64, sigDigits int) float64 {
	factor := math.Pow10(sigDigits)
	return math.Round(x*factor) / factor
}

func RoundUp(x float64, sigDigits int) float64 {
	factor := math.Pow10(sigDigits)
	return math.Ceil(x*factor) / factor
}

func DecimalPlaces(x float64) int {
	str := strconv.FormatFloat(x, 'f', -1, 64)
	idx := strings.Index(str, ".")
	if idx == -1 {
		return 0
	}
	return len(str) - idx - 1
}

func ToTokenDecimals(x float64) int64 {
	f := 1_000_000 * x
	if DecimalPlaces(f) > 0 {
		f = RoundNormal(f, 0)
	}
	return int64(f)
}

func toBool(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	case float64:
		return v != 0
	case json.Number:
		parsed, err := v.Float64()
		return err == nil && parsed != 0
	case int:
		return v != 0
	default:
		return false
	}
}
