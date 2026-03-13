# Volatility Calibration Analysis Report

**Date**: 2026-03-13
**Data**: 305 pricing ticks across 2 BTC 5-minute markets
**Model**: DynamicGaussianModel (Gaussian CDF on log-moneyness / sigma_tau)

---

## 1. Executive Summary

The model's `p_raw` is systematically overconfident because `sigma_tau` (horizon-scaled volatility) is far too low. The root cause is twofold:

1. **66.6% of observations hit the vol floor** (0.00005), meaning measured vol is effectively zero most of the time.
2. **The vol floor itself is 2.1x too low** relative to what both BTC fundamentals and market-implied vol suggest.

The median model per-second vol is 0.00005 (the floor), while the market-implied per-second vol is 0.000119. The model runs at **0.42x** of what the market prices in.

---

## 2. Key Findings

### 2.1 sigma_tau is systematically too small

| Statistic | Model sigma_tau | Expected sigma_tau (60% ann) |
|-----------|----------------|------------------------------|
| At 240s remaining | 0.000618 (median) | 0.001655 |
| At 180s remaining | typical ~0.0005 | 0.001433 |
| Ratio | -- | Model is ~0.37x-0.47x of expected |

### 2.2 Vol floor dominates the output

- 203 of 305 observations (66.6%) have back-derived perSecVol = 0.00005 exactly (the floor)
- Median perSecVol = 0.00005, P75 = 0.00005843
- Only 25% of observations have measured vol above the floor

**Why**: Chainlink does not update every 500ms. The 500ms resampler repeats stale prices, producing `logReturn = 0` for most ticks. The rolling window of ~120 ticks therefore has mostly zero returns, driving variance toward zero.

### 2.3 z-scores are extreme

| Metric | Value |
|--------|-------|
| |z| > 2 | 45.2% of observations |
| |z| > 3 | 36.1% of observations |
| Max |z| | 41.9 |
| Median z | 1.77 |

For a well-calibrated Gaussian model, we would expect ~5% of |z| > 2. The model produces 9x that rate.

### 2.4 p_raw is overconfident

| Metric | p_raw | Market (up_ask) |
|--------|-------|-----------------|
| Extreme (>0.95 or <0.05) | 51.8% | 18.0% |
| Median | 0.962 | 0.82 |
| Mean bias (p_raw - up_ask) | +0.172 | -- |

The model is almost always more confident than the market, by a median of ~10 cents.

### 2.5 sigma_tau threshold for divergence

| sigma_tau bucket | N | Mean |error| | Mean p_raw | Mean market |
|-----------------|---|---------------|------------|-------------|
| 0.0000 - 0.0005 | 100 | **0.3062** | 0.9996 | 0.6934 |
| 0.0005 - 0.0010 | 148 | 0.1405 | 0.8747 | 0.7342 |
| 0.0010 - 0.0020 | 34 | **0.0798** | 0.8754 | 0.7956 |
| 0.0020 - 0.0050 | 21 | **0.0435** | 0.9250 | 0.9681 |

The model is well-calibrated when sigma_tau > 0.002 (mean error 4.4 cents). Below sigma_tau < 0.0005, mean error is 30.6 cents -- essentially useless. The critical threshold is sigma_tau ~ 0.001.

### 2.6 Market-implied volatility confirms the diagnosis

By inverting `up_ask = Phi(z)` to extract the vol the market prices in:

| Metric | Market-implied per-sec vol |
|--------|--------------------------|
| Median | 0.000119 |
| P25 | 0.000108 |
| P75 | 0.000143 |
| Mean | 0.000124 |

Compare:
- BTC theoretical (60% annual): 0.000107
- Current model median: 0.000050 (the floor)
- Market-implied median: 0.000119

**The market prices BTC 5-minute vol at ~1.1x the 60% annualized figure.** This is sensible -- short-term microstructure effects add a small premium above annualized vol.

---

## 3. Root Cause Deep Dive

### 3.1 The vol computation path

```
reference_analytics.go:
  perTickVol = sqrt(variance of log returns in window)
  perSecVol  = perTickVol / sqrt(tickIntervalSec)      // tickIntervalSec = 0.5

dynamic_gaussian.go:
  if perSecVol < 0.00005: perSecVol = 0.00005           // vol floor
  sigma_tau = perSecVol * sqrt(remaining_seconds)
  z = log(S/K) / sigma_tau
  p_raw = Phi(z)
```

### 3.2 The normalization formula is mathematically correct but empirically broken

The formula `perTickVol / sqrt(dt)` is the correct scaling for i.i.d. returns at fixed intervals. However, the **data violates the i.i.d. assumption**: most 500ms ticks have zero log-returns because Chainlink updates are sparse (typically every few seconds at best).

If 90% of ticks have `logReturn = 0`, the measured variance is ~10% of the true variance, and `sqrt(variance)` is ~32% of true vol. After dividing by `sqrt(0.5) = 0.707`, we get about 45% of true per-second vol. This matches our observation that the model runs at 0.42x of market-implied vol.

### 3.3 The vol floor is too low

The current floor of `0.00005` per-second corresponds to:
- Annualized: `0.00005 * sqrt(365.25*24*3600) = 28%` annual vol
- This is half of BTC's typical 60% annual vol
- It is 0.42x of the market-implied per-second vol

---

## 4. Recommended Fixes (Specific Parameters)

### Fix 1: Raise the vol floor to 0.00012

```go
// In dynamic_gaussian.go, line 72:
// OLD:
const volFloorPerSec = 0.00005
// NEW:
const volFloorPerSec = 0.00012
```

**Rationale**: Market-implied median per-second vol is 0.000119. Setting the floor at 0.00012 ensures the model never prices below what the market considers normal BTC vol. This reduces mean |error| from 0.182 to 0.108 and median |error| from 0.100 to 0.026.

| Metric | Current floor (0.00005) | Proposed floor (0.00012) |
|--------|------------------------|-------------------------|
| Mean |error| | 0.182 | ~0.108 |
| Median |error| | 0.100 | ~0.026 |
| Extreme p_raw (>0.95 or <0.05) | 51.8% | ~23% |

### Fix 2: Exclude zero-returns from vol computation

In `reference_analytics.go`, the vol computation counts all resampled ticks including those with `logReturn = 0` from stale Chainlink data. This deflates measured vol.

```go
// In computeVolWindow(), around line 143:
// OLD:
if records[i].LogReturn != 0 || (i > 0 && records[i-1].Price > 0) {
    lr := records[i].LogReturn
    sum += lr
    sumSq += lr * lr
    n++
}

// NEW: Only count ticks where price actually changed
if records[i].LogReturn != 0 {
    lr := records[i].LogReturn
    sum += lr
    sumSq += lr * lr
    n++
}
```

Then adjust the normalization to account for the actual time between price-change events:

```go
// After computing perTickVol, normalize by actual mean interval between
// non-zero returns, not the resample interval
actualIntervalSec := window.Seconds() / float64(n)  // average seconds between real price changes
return perTickVol / math.Sqrt(actualIntervalSec)
```

This ensures that if Chainlink updates every ~3 seconds on average, we divide by `sqrt(3)` instead of `sqrt(0.5)`, correctly attributing the observed moves to their actual time span.

### Fix 3: Apply Bayesian shrinkage toward BTC prior vol

Instead of using measured vol directly, blend it with a prior:

```go
// In dynamic_gaussian.go, after getting perSecVol:
const priorVolPerSec = 0.000107  // BTC ~60% annual
const priorWeight    = 0.3       // 30% prior, 70% measured

// Only shrink if measured vol is below prior (avoid dampening real vol spikes)
if perSecVol < priorVolPerSec {
    perSecVol = priorWeight*priorVolPerSec + (1-priorWeight)*perSecVol
}
```

Simulation results for blending weight:

| Weight on measured | Mean |error| | Median |error| |
|-------------------|--------------|----------------|
| 1.0 (current) | 0.182 | 0.100 |
| 0.7 | 0.153 | 0.077 |
| 0.5 | 0.135 | 0.063 |
| 0.3 | 0.121 | 0.044 |
| 0.0 (pure prior) | 0.107 | 0.027 |

The pure prior outperforms any blend on this dataset because measured vol is so unreliable. In practice, a 30/70 (prior/measured) blend gives resilience against both stale-data calm periods and genuine vol spikes.

### Fix 4: Add a sigma_tau floor (defense in depth)

Even after fixing per-second vol, add a minimum sigma_tau to prevent extreme z-scores:

```go
// In dynamic_gaussian.go, after computing horizonStd:
const minSigmaTau = 0.001  // ~prevents |z| > 5 for typical BTC moneyness
if horizonStd < minSigmaTau {
    horizonStd = minSigmaTau
}
```

### Fix 5: Clamp z-scores as final safety net

```go
// In dynamic_gaussian.go, after computing z:
const maxAbsZ = 3.0
if z > maxAbsZ {
    z = maxAbsZ
}
if z < -maxAbsZ {
    z = -maxAbsZ
}
```

This caps p_raw to the range [0.0013, 0.9987], preventing the model from ever being more than 99.87% confident. For a 5-minute binary option, this is appropriate -- there is always meaningful uncertainty.

---

## 5. Priority-Ordered Implementation Plan

| Priority | Fix | Impact | Effort |
|----------|-----|--------|--------|
| P0 | Raise vol floor to 0.00012 | Halves mean error | 1 line change |
| P0 | Clamp z to [-3, +3] | Eliminates p_raw = 0.999+ cases | 3 lines |
| P1 | Exclude zero-returns + fix interval normalization | Fixes root cause of low measured vol | ~15 lines |
| P2 | Bayesian shrinkage toward prior | Smooth transition between regimes | ~10 lines |
| P2 | sigma_tau floor at 0.001 | Defense in depth | 3 lines |

---

## 6. Validation Plan

After implementing fixes, verify:
1. Median |p_raw - up_ask| < 0.05 (currently 0.10)
2. Fraction of extreme p_raw (>0.95 or <0.05) < 25% (currently 51.8%)
3. Back-derived per-second vol median near 0.000107-0.00012 (currently 0.00005)
4. No |z| > 3 except during genuine jumps
5. Model-market correlation improves when measured on sufficiently varying market data
