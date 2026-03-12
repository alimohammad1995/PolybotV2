package service

import (
	"sync"

	"Polybot/internal/domain"
)

type ReferencePriceCache struct {
	mu     sync.RWMutex
	prices map[string]domain.ReferenceSnapshot
}

func NewReferencePriceCache() *ReferencePriceCache {
	return &ReferencePriceCache{
		prices: make(map[string]domain.ReferenceSnapshot),
	}
}

func (c *ReferencePriceCache) Set(snap domain.ReferenceSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.prices[snap.Asset] = snap
}

func (c *ReferencePriceCache) Get(asset string) (domain.ReferenceSnapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.prices[asset]
	return s, ok
}
