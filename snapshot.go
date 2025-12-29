package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"sync"
	"time"
)

var snapshotManager = NewSnapshot()

type Snapshot struct {
	mu       sync.RWMutex
	marketID string
	snapshot map[string]any
}

func NewSnapshot() *Snapshot {
	return &Snapshot{
		snapshot: make(map[string]any),
	}
}

func (s *Snapshot) Run() {
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			s.Save()
		}
	}()
}

func (s *Snapshot) Tick(marketID string, upBestBidAsk, downBestBidAsk []*MarketOrder) {
	s.mu.Lock()
	defer s.mu.Unlock()

	snapshotManager.marketID = marketID

	now := time.Now().Unix()

	if _, ok := s.snapshot[marketID]; !ok {
		s.snapshot[marketID] = make(map[string]any)
	}

	marketSnapshot, ok := s.snapshot[marketID].(map[string]any)

	if !ok {
		marketSnapshot = make(map[string]any)
		s.snapshot[marketID] = marketSnapshot
	}

	marketSnapshot[strconv.FormatInt(now, 10)] = map[string]any{
		"up":   upBestBidAsk,
		"down": downBestBidAsk,
	}
}

func (s *Snapshot) Save() {
	s.mu.RLock()
	marketID := s.marketID
	if marketID == "" {
		s.mu.RUnlock()
		return
	}

	data, err := json.MarshalIndent(s.snapshot[marketID], "", "  ")
	s.mu.RUnlock()

	if err != nil {
		log.Printf("failed to marshal snapshot: %v", err)
		return
	}

	filename := fmt.Sprintf("/Users/alimohammad/PycharmProjects/Scripts/data/snapshot_%s.json", marketID)
	if err := ioutil.WriteFile(filename, data, 0644); err != nil {
		log.Printf("failed to write snapshot file: %v", err)
	}
}
