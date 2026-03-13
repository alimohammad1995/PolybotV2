package domain

import "time"

type FairValue struct {
	MarketID         MarketID
	ProbUp           float64
	ProbUpLower      float64
	ProbUpUpper      float64
	ProbRaw          float64 // uncalibrated probability from model
	ProbCalibrated   float64 // after isotonic calibration (== ProbRaw if no calibration)
	SigmaTau         float64 // horizon-scaled vol used in model
	ZScore           float64 // log(S/K) / sigma_tau
	ModelUncertainty float64
	RemainingSeconds float64
	RequiredLogMove  float64
	ModelRegime      string
	Timestamp        time.Time
}

// PricingInput contains everything needed to compute fair probability.
// CurrentPrice + vol come from Chainlink (truth process).
// PriceToBeat + RemainingSeconds come from market metadata.
// Polymarket prices are NEVER fed into the pricing model.
type PricingInput struct {
	CurrentPrice     float64
	PriceToBeat      float64
	RemainingSeconds float64
	RealizedVol1m    float64
	RealizedVol5m    float64
	JumpScore        float64
	Regime           string
}

type ReferenceSnapshot struct {
	Asset     string
	Price     float64
	Timestamp time.Time
}

// ChainlinkTick is a single price observation from Chainlink Data Streams.
type ChainlinkTick struct {
	Asset     string
	Price     float64
	Timestamp time.Time
}

// ReferenceState is the full analytics state built ONLY from Chainlink.
// This is the "truth process" — never contaminated with Polymarket data.
type ReferenceState struct {
	Asset             string
	CurrentPrice      float64
	RealizedVol1m     float64
	RealizedVol5m     float64
	VolStabilityScore float64
	JumpScore         float64
	Regime            string
	TickCount         int
	LastUpdate        time.Time
}

// MarketState is the tradable state built ONLY from Polymarket.
// This is the "execution venue" — what others are willing to pay.
type MarketState struct {
	MarketID    MarketID
	PriceToBeat float64
	UpBid       float64
	UpAsk       float64
	DownBid     float64
	DownAsk     float64
	UpDepth     float64
	DownDepth   float64
	Spread      float64
	Timestamp   time.Time
}

type RepriceEvent struct {
	MarketID MarketID
	Reason   string
}
