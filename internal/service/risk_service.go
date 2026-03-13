package service

import "math"

type RiskConfig struct {
	MaxPositionUSDPerMarket float64
	MaxTotalExposureUSD     float64
	NoNewTradeCutoffSecs    float64
	FractionalKelly         float64
	MinTradeSizeUSD         float64
	MinTradeShares          float64 // Polymarket minimum order size in shares (default: 5)
}

type RiskService struct {
	Config RiskConfig
}

func NewRiskService(cfg RiskConfig) *RiskService {
	return &RiskService{Config: cfg}
}

func (r *RiskService) ShouldAllowNewTrade(remainingSeconds float64) bool {
	return remainingSeconds > r.Config.NoNewTradeCutoffSecs
}

// ComputeTargetSizeUSD computes the trade size in USD given edge, bankroll,
// current exposure, and the price per share. The result is snapped to whole
// shares and enforces both MinTradeSizeUSD and MinTradeShares constraints.
func (r *RiskService) ComputeTargetSizeUSD(
	edge float64,
	bankrollUSD float64,
	currentMarketExposureUSD float64,
	pricePerShare float64,
) float64 {
	if edge <= 0 || pricePerShare <= 0 {
		return 0
	}

	size := bankrollUSD * r.Config.FractionalKelly * edge * 10.0
	maxRemaining := r.Config.MaxPositionUSDPerMarket - currentMarketExposureUSD
	if size > maxRemaining {
		size = maxRemaining
	}
	if size < r.Config.MinTradeSizeUSD {
		return 0
	}
	if size < 0 {
		return 0
	}

	// Snap to whole shares: Polymarket requires integer share quantities
	shares := math.Floor(size / pricePerShare)

	// Enforce minimum shares (Polymarket requires at least 5 shares per order)
	minShares := r.Config.MinTradeShares
	if minShares <= 0 {
		minShares = 5
	}
	if shares < minShares {
		return 0
	}

	return shares * pricePerShare
}
