package backtest

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"Polybot/internal/domain"
)

type ReplayDataPoint struct {
	Timestamp    time.Time `json:"timestamp"`
	MarketID     string    `json:"market_id"`
	Asset        string    `json:"asset"`
	PriceToBeat  float64   `json:"price_to_beat"`
	RefPrice     float64   `json:"ref_price"`
	UpBid        float64   `json:"up_bid"`
	UpAsk        float64   `json:"up_ask"`
	DownBid      float64   `json:"down_bid"`
	DownAsk      float64   `json:"down_ask"`
	SettlementAt time.Time `json:"settlement_at"`
}

type ReplaySettlement struct {
	MarketID string    `json:"market_id"`
	Outcome  string    `json:"outcome"`
	Time     time.Time `json:"time"`
}

type ReplayData struct {
	Points      []ReplayDataPoint  `json:"points"`
	Settlements []ReplaySettlement `json:"settlements"`
}

func LoadReplayData(path string) (*ReplayData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read replay file: %w", err)
	}
	var rd ReplayData
	if err := json.Unmarshal(data, &rd); err != nil {
		return nil, fmt.Errorf("parse replay file: %w", err)
	}
	return &rd, nil
}

func (rd *ReplayData) ToSnapshots() ([]MarketSnapshot, []SettlementRecord) {
	snaps := make([]MarketSnapshot, 0, len(rd.Points))
	for _, p := range rd.Points {
		snaps = append(snaps, MarketSnapshot{
			Timestamp: p.Timestamp,
			Market: domain.BinaryMarket{
				ID:          domain.MarketID(p.MarketID),
				Asset:       p.Asset,
				PriceToBeat: p.PriceToBeat,
				EndTime:     p.SettlementAt,
				Status:      domain.MarketStatusActive,
			},
			Quote: domain.MarketQuote{
				MarketID:  domain.MarketID(p.MarketID),
				Up:        domain.SideQuote{Bid: p.UpBid, Ask: p.UpAsk},
				Down:      domain.SideQuote{Bid: p.DownBid, Ask: p.DownAsk},
				Timestamp: p.Timestamp,
			},
			RefPrice: p.RefPrice,
		})
	}

	settlements := make([]SettlementRecord, 0, len(rd.Settlements))
	for _, s := range rd.Settlements {
		settlements = append(settlements, SettlementRecord{
			MarketID: domain.MarketID(s.MarketID),
			Outcome:  s.Outcome,
			Time:     s.Time,
		})
	}

	return snaps, settlements
}
