package domain

import "time"

type TradeSignalSide string

const (
	SignalNone      TradeSignalSide = "none"
	SignalBuyUp     TradeSignalSide = "buy_up"
	SignalBuyDown   TradeSignalSide = "buy_down"
	SignalHedgeUp   TradeSignalSide = "hedge_up"
	SignalHedgeDown TradeSignalSide = "hedge_down"
)

type TradeSignal struct {
	MarketID        MarketID
	Side            TradeSignalSide
	SignalType      string // "directional" or "hedge"
	EdgeBuyUp       float64
	EdgeBuyDown     float64
	EffectiveHurdle float64
	TargetSizeUSD   float64
	GuaranteedFloor float64 // current floor value (for hedge signals)
	HedgeEdge       float64 // ΔG from this trade (for hedge signals)
	Reason          string
	Timestamp       time.Time
}
