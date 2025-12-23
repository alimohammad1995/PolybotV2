package main

const (
	DBase  = 2
	DLate  = 1
	DFinal = 0

	MaxUnmatched    = 10.0
	MaxSharePerSize = 40

	MinimumStartWaitingSec = 10
	MinPrice               = 5
	MaxPrice               = 100
	StopNewUnmatchedSec    = 3 * 60
)

var LevelSize = []float64{5, 5, 5, 5}

type Side string

const (
	UP   Side = "UP"
	DOWN Side = "DOWN"
)
