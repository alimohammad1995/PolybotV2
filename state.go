package main

type State struct {
	UpQuantity       float64
	UpAveragePrice   float64
	DownQuantity     float64
	DownAveragePrice float64
}

func NewState() *State {
	return &State{}
}

func (state *State) UpdateState(price float64, quantity float64, isUp bool) {
	if isUp {
		state.UpAveragePrice = (state.UpAveragePrice*state.UpQuantity + price*quantity) / (state.UpQuantity + quantity)
		state.UpQuantity += quantity
	} else {
		state.DownAveragePrice = (state.DownAveragePrice*state.DownQuantity + price*quantity) / (state.DownQuantity + quantity)
		state.DownQuantity += quantity
	}
}
