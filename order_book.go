package main

type OrderBook struct {
	Asks map[float64]float64
	Bids map[float64]float64
}
