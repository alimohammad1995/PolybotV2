package domain

import "time"

type SideQuote struct {
	Bid float64
	Ask float64
}

type MarketQuote struct {
	MarketID  MarketID
	Up        SideQuote
	Down      SideQuote
	Timestamp time.Time
}
