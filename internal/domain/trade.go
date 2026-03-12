package domain

import "time"

type ExecutionRequest struct {
	MarketID MarketID
	Side     TradeSignalSide
	MaxPrice float64
	SizeUSD  float64
	Reason   string
}

type Fill struct {
	MarketID  MarketID
	Side      TradeSignalSide
	Price     float64
	SizeUSD   float64
	Timestamp time.Time
}

type Settlement struct {
	MarketID    MarketID
	Outcome     string
	SettledAt   time.Time
	RealizedPnL float64
}
