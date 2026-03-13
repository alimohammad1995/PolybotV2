# Polybot

## Project

Polybot is a real-time quantitative trading bot for Polymarket binary options on BTC price movements (5-minute windows). It computes fair probability using a Gaussian model driven by Chainlink Data Streams volatility, compares against Polymarket order book prices, and executes directional + hedge trades when edge exceeds configurable hurdles.

### Architecture

- **Go backend** — single binary in `cmd/bot/`
- **Domain-driven** — `internal/domain/`, `internal/service/`, `internal/ports/`, `internal/infra/`
- **Two independent data streams** (never mixed in pricing):
  - **Chainlink Data Streams** (truth process): real-time BTC price + realized vol
  - **Polymarket WebSocket** (execution venue): order book quotes
- **Signal flow**: both streams feed `repriceCh` -> `repriceLoop` -> `EvaluateMarket` with zero artificial delay
- **Fill tracking**: live mode uses Polymarket user WebSocket for trade confirmations; paper mode records fills immediately

### Key constraints

- **Real-time**: no resampling, no batching, no throttling on signal paths. Every Chainlink tick and Polymarket quote update triggers immediate re-evaluation.
- **FOK orders**: Polymarket orders are Fill-or-Kill — they fill completely and instantly or are rejected. No resting orders, no cancellation needed.
- **Minimum 5 shares**: Polymarket requires at least 5 shares per order. Trade sizing snaps to whole shares.
- **Vol floor**: BTC per-second vol floored at 0.00012 (market-implied median). Chainlink feeds can have gaps between real price changes.
- **Z-score clamp**: z clamped to [-3, +3] for 5-minute binaries — always meaningful uncertainty.

## Rules

- Do NOT make changes beyond what was explicitly requested. If you notice additional improvements while working, mention them but do not implement without approval.
- No dead code. Do not leave unused functions, types, variables, imports, or config fields. If something is no longer referenced, delete it.
- Always run `go build ./...` after edits to verify compilation.
- Always run `go test ./...` after edits to verify tests pass.
- Prefer editing existing files over creating new ones.
- Keep solutions simple. Don't add error handling, fallbacks, or validation for scenarios that can't happen.
- Before implementing, present ONE concise approach and ask for confirmation. Do not start coding exploratory solutions.
