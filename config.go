package main

const (
	DBase  = 2
	DLate  = 1
	DFinal = 0
	DLoss  = -5

	MaxUnmatched           = 1.0
	MaxHoldingSharePerSize = 50

	MinimumStartWaitingSec = 10
	MinPrice               = 5
	MaxPrice               = 100
	StopNewUnmatchedSec    = 3 * 60
)

const (
	TotalSeconds = 900
	PayoutCents  = 100
	TickCents    = 1

	TagEdgeUp    = "EDGE_UP"
	TagEdgeDown  = "EDGE_DOWN"
	TagHedgeUp   = "HEDGE_UP"
	TagHedgeDown = "HEDGE_DOWN"
	TagArbUp     = "ARB_UP"
	TagArbDown   = "ARB_DOWN"
)
