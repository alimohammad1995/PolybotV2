---
name: Market 2 balance cap bypass bug
description: Directional trades bypass the hedge balance cap, causing catastrophic late-expiry imbalances (2026-03-13 session)
type: project
---

Balance cap at fair_value_strategy.go:257 only applies to signal.SignalType == "hedge". Directional trades can create arbitrary imbalances from a balanced position. In the 2026-03-13 session, Market 2 L238 bought 45 UP shares at $0.06 from a balanced 18/18 position with 61 seconds remaining, causing a $2.73 loss.

**Why:** The selectSignal() function (line 189) evaluates hedge first, then directional. When hedge returns SignalNone (as it does when balanced), directional fires. The directional signal has no imbalance cap. Near expiry with extreme prices (up_ask=0.07), the directional edge threshold (3c) is easily exceeded, triggering large buys that cannot be hedged before market close.

**How to apply:** When reviewing trade sizing or balance cap changes, ensure caps apply to ALL signal types near expiry, not just hedge signals. The late-market guard (block trades in final 30s unless reducing imbalance) is the recommended fix.
