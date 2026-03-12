package domain

import "time"

type PositionSide string

const (
	PositionUp   PositionSide = "up"
	PositionDown PositionSide = "down"
)

type Position struct {
	MarketID         MarketID
	Side             PositionSide
	AvgEntryPrice    float64
	Quantity         float64
	NotionalUSD      float64
	OpenedAt         time.Time
	HoldToSettlement bool
}
