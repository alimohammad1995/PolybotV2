---
name: Price tracker action label does not reflect actual trade
description: The action field in pricing logs is a heuristic from edge thresholds, not the actual executed trade type
type: project
---

The `action` field in price_tracker.go (lines 158-168) is computed from edge threshold checks (hedgeEdgeBuyDown > 0.002, dirEdgeUp > 0.03), NOT from the signal that triggered the actual fill. Fills appear in qty changes and may correspond to the previous tick's signal. Example: action=no_trade but quantities changed.

**Why:** This makes post-trade analysis unreliable -- cannot determine from logs alone whether a fill was directional or hedge.

**How to apply:** When analyzing pricing logs, track fills by qty/cost changes between ticks, not by the action field. The action field only indicates what the system WOULD signal at that tick.
