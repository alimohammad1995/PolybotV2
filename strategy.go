package main

const Workers = 5

type Strategy struct {
	client   *PolymarketClient
	executor *OrderExecutor
	markets  chan string
}

func NewStrategy(client *PolymarketClient) *Strategy {
	strategy := &Strategy{
		client:   client,
		executor: NewOrderExecutor(client),
		markets:  make(chan string, Workers),
	}
	strategy.Run()
	return strategy
}

func (s *Strategy) OnOrderBookUpdate(assetID []string) {
	marketsToCheck := map[string]bool{}
	for _, id := range assetID {
		if marketID, ok := TokenToMarketID[id]; ok {
			marketsToCheck[marketID] = true
			s.markets <- marketID
		}
	}

}

func (s *Strategy) OnAssetUpdate(assetID []string) {
	marketsToCheck := map[string]bool{}
	for _, id := range assetID {
		if marketID, ok := TokenToMarketID[id]; ok {
			marketsToCheck[marketID] = true
			s.markets <- marketID
		}
	}
}

func (s *Strategy) Run() {
	for i := 0; i < Workers; i++ {
		go func() {
			for {
				market := <-s.markets
				s.handle(market)
			}
		}()
	}
}

func (s *Strategy) handle(marketID string) {
}
