package main

import (
	"fmt"
	"time"
)

var ActiveMarkets = make(map[string]bool)

func GetMarketName(market string, index int) (string, int64) {
	now := time.Now().Unix()
	ts := now - now%IntervalSeconds + int64(IntervalSeconds*index)
	return fmt.Sprintf("%s%d", market, ts), ts
}
