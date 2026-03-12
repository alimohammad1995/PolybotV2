package service

import (
	"sync"

	"Polybot/internal/domain"
)

type MarketRegistry struct {
	mu      sync.RWMutex
	markets map[domain.MarketID]domain.BinaryMarket
	quotes  map[domain.MarketID]domain.MarketQuote
}

func NewMarketRegistry() *MarketRegistry {
	return &MarketRegistry{
		markets: make(map[domain.MarketID]domain.BinaryMarket),
		quotes:  make(map[domain.MarketID]domain.MarketQuote),
	}
}

func (r *MarketRegistry) SetMarket(m domain.BinaryMarket) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.markets[m.ID] = m
}

func (r *MarketRegistry) GetMarket(id domain.MarketID) (domain.BinaryMarket, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.markets[id]
	return m, ok
}

func (r *MarketRegistry) SetQuote(q domain.MarketQuote) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quotes[q.MarketID] = q
}

func (r *MarketRegistry) GetQuote(id domain.MarketID) (domain.MarketQuote, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	q, ok := r.quotes[id]
	return q, ok
}

func (r *MarketRegistry) ListMarkets() []domain.BinaryMarket {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]domain.BinaryMarket, 0, len(r.markets))
	for _, m := range r.markets {
		result = append(result, m)
	}
	return result
}

func (r *MarketRegistry) ListMarketIDsForAsset(asset string) []domain.MarketID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]domain.MarketID, 0, len(r.markets))
	for _, m := range r.markets {
		if m.Asset == asset {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

func (r *MarketRegistry) RemoveMarket(id domain.MarketID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.markets, id)
	delete(r.quotes, id)
}
