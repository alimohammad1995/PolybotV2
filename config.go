package main

const (
	// Discounts are in price*100 units (e.g., 2.0 == 0.02).
	DBase  = 2.0
	DMid   = 1.5
	DLate  = 1.0
	DFinal = 0.0

	MaxUnmatched = 30.0

	LadderLevels = 4
	LadderStep   = 1
	MinPrice     = 1
	MaxPrice     = 100
	MaxBaseBid   = 99

	StopNewUnmatchedSec = 90
	PauseSec            = 60
)

var LevelSize = []float64{5, 5, 5, 5}
