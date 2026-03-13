---
name: BTC vol calibration issue
description: DynamicGaussianModel p_raw overconfidence due to vol floor too low and stale Chainlink ticks deflating measured vol
type: project
---

The DynamicGaussianModel produces wildly overconfident p_raw values (median 0.962 vs market 0.82) because sigma_tau is systematically too low. 66.6% of observations hit the vol floor at 0.00005 per-second. Market-implied per-second vol is 0.000119 (2.4x the floor). Root cause: Chainlink data at 500ms resampling produces mostly zero log-returns, deflating variance. The vol normalization formula is mathematically correct but the input data violates i.i.d. assumptions.

**Why:** This causes the bot to see phantom edges (p_raw=0.999 vs market=0.64), leading to bad trades.

**How to apply:** When working on the model or vol pipeline, ensure vol floor is raised to ~0.00012, z-scores are clamped to [-3,3], and consider excluding zero-returns from vol computation with proper interval adjustment.
