---
name: Hedge engine sizing overshoot bug
description: Critical bug where hedge orders overshoot the balance point (Nu=Nd), converting hedged positions into massive directional bets. Identified 2026-03-13.
type: project
---

The hedge engine in `internal/service/hedge_engine.go` computes correct per-share marginal delta-G, but the signal is passed through `RiskService.ComputeTargetSizeUSD()` which applies Kelly sizing with `bankroll * fractionalKelly * edge * 10`. For hedge trades, the "edge" (delta-G per share) is only valid up to the balance point (Nu=Nd) -- beyond that, additional shares have NEGATIVE delta-G. The Kelly sizer doesn't know about the balance point and sizes orders 10-20x too large.

**Why:** In the 2026-03-13 paper trading session, this caused Market 3 to go from 18 up / 7 down (should have bought 11 down) to 18 up / 140 down (bought 133 down), resulting in G=-$31.99 and a realized loss of -$32.17.

**How to apply:** The fix is to cap hedge order size at `|Nu - Nd| * askPrice` in the strategy execution path, or better yet, have the hedge engine itself specify a max quantity. Do NOT pass hedge_edge through the standard Kelly sizer without a size cap.
