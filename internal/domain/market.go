package domain

import "time"

type MarketID string

type MarketStatus string

const (
	MarketStatusActive    MarketStatus = "active"
	MarketStatusSettled   MarketStatus = "settled"
	MarketStatusCancelled MarketStatus = "cancelled"
)

type BinaryMarket struct {
	ID             MarketID
	Slug           string
	Asset          string
	StartTime      time.Time
	EndTime        time.Time
	SettlementTime time.Time
	PriceToBeat    float64
	Status         MarketStatus
	UpTokenID      string
	DownTokenID    string
}
