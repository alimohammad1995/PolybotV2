package domain

import "time"

type TradeSignalSide string

const (
	SignalNone    TradeSignalSide = "none"
	SignalBuyUp   TradeSignalSide = "buy_up"
	SignalBuyDown TradeSignalSide = "buy_down"
)

type TradeSignal struct {
	MarketID        MarketID
	Side            TradeSignalSide
	EdgeBuyUp       float64
	EdgeBuyDown     float64
	EffectiveHurdle float64
	TargetSizeUSD   float64
	Reason          string
	Timestamp       time.Time
}
