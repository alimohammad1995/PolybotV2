package main

const (
	// Discounts are in price*100 units (e.g., 2.0 == 0.02).
	DBase  = 2
	DLate  = 1
	DFinal = 0

	MaxUnmatched = 30.0

	LadderLevels = 4
	LadderStep   = 1
	MinPrice     = 1
	MaxPrice     = 100
	MaxBaseBid   = 99

	StopNewUnmatchedSec = 3 * 60
	PauseSec            = 60
)

var LevelSize = []float64{5, 5, 5, 5}

type Side string

const (
	UP   Side = "UP"
	DOWN Side = "DOWN"
)
