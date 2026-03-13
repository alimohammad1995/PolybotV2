---
name: Warm-up guard validated
description: MinTickCount=10 warm-up guard confirmed working as of 2026-03-13 session. Prevents first-tick sigma_tau spikes from triggering trades.
type: project
---

The warm-up guard (`MinTickCount=10` in config, enforced at `fair_value_strategy.go` line 124-131) successfully blocked trading during the first 8 ticks of Market 1 when sigma_tau spiked to 0.00847 (8.5x normal). No position was taken until tick 9 when sigma_tau had settled to ~0.001.

**Why:** First-tick sigma_tau spikes were identified in a prior session as causing bad initial trades.

**How to apply:** This feature is working. No changes needed. The log still shows "action: dir_buy_down" during warm-up (intent, not execution) which can be confusing for analysis -- a cosmetic improvement would be to log "warmup_blocked" instead.
