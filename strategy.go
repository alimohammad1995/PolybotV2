package main

type Strategy struct {
	client   *PolymarketClient
	executor *OrderExecutor
}

func NewStrategy(client *PolymarketClient) *Strategy {
	return &Strategy{
		client:   client,
		executor: NewOrderExecutor(client),
	}
}

func (s *Strategy) OnOrderBookUpdate(assetID []string) {

}

func (s *Strategy) OnAssetUpdate(assetID []string) {

}
