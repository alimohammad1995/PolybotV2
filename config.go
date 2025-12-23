package main

const (
	DBase  = 2
	DLate  = 1
	DFinal = 0
	DLoss  = -5

	MaxUnmatched    = 10.0
	MaxSharePerSize = 40

	MinimumStartWaitingSec = 10
	MinPrice               = 5
	MaxPrice               = 100
	StopNewUnmatchedSec    = 3 * 60
)

type Side string

const (
	UP   Side = "UP"
	DOWN Side = "DOWN"
)
