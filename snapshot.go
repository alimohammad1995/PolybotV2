package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"strconv"
	"time"
)

var snapshot = make(map[string]any)

func Snapshot(marketID string, upBestBidAsk, downBestBidAsk []*MarketOrder) {
	SnapShotSave(marketID, upBestBidAsk, downBestBidAsk)
}

func SnapShotSave(marketID string, upBestBidAsk, downBestBidAsk []*MarketOrder) {
	now := time.Now().Unix()

	if _, ok := snapshot[marketID]; !ok {
		snapshot[marketID] = make(map[string]any)
	}

	marketSnapshot, ok := snapshot[marketID].(map[string]any)

	if !ok {
		marketSnapshot = make(map[string]any)
		snapshot[marketID] = marketSnapshot
	}

	marketSnapshot[strconv.FormatInt(now, 10)] = map[string]any{
		"up":   upBestBidAsk,
		"down": downBestBidAsk,
	}

	data, err := json.MarshalIndent(snapshot[marketID], "", "  ")
	if err != nil {
		log.Printf("failed to marshal snapshot: %v", err)
		return
	}

	filename := fmt.Sprintf("/Users/alimohammad/PycharmProjects/Scripts/data/snapshot_%s.json", marketID)
	if err := ioutil.WriteFile(filename, data, 0644); err != nil {
		log.Printf("failed to write snapshot file: %v", err)
	}
}
