package service

import "math"

type RiskConfig struct {
	MaxPositionUSDPerMarket float64
	MaxTotalExposureUSD     float64
	NoNewTradeCutoffSecs    float64
	FractionalKelly         float64
	MinTradeSizeUSD         float64
	MinTradeShares          float64 // Polymarket minimum order size in shares (default: 5)

	// Inventory risk gates
	MaxImbalanceShares float64 // hard block on abs(upQty - downQty) after trade
	MinGuaranteedFloor float64 // reject trades pushing floor below this
	MaxWorstCaseLoss   float64 // per-market worst-case loss cap
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

// PreTradeCheck validates inventory risk gates before a trade executes.
// Returns "" if allowed, or a rejection reason string.
func (r *RiskService) PreTradeCheck(
	buyingUp bool,
	shares float64,
	price float64,
	upQty, downQty, upCost, downCost float64,
) string {
	// Compute pro-forma state after trade
	newUpQty, newDownQty := upQty, downQty
	newUpCost, newDownCost := upCost, downCost
	tradeCost := shares * price
	if buyingUp {
		newUpQty += shares
		newUpCost += tradeCost
	} else {
		newDownQty += shares
		newDownCost += tradeCost
	}

	// Gate 1: max imbalance
	if r.Config.MaxImbalanceShares > 0 {
		imbalance := math.Abs(newUpQty - newDownQty)
		if imbalance > r.Config.MaxImbalanceShares {
			return "max_imbalance_exceeded"
		}
	}

	// Gate 2: guaranteed floor
	if r.Config.MinGuaranteedFloor < 0 {
		floor := math.Min(newUpQty, newDownQty) - newUpCost - newDownCost
		if floor < r.Config.MinGuaranteedFloor {
			return "floor_below_minimum"
		}
	}

	// Gate 3: worst-case loss
	if r.Config.MaxWorstCaseLoss > 0 {
		// If UP wins: gain upQty, lose downCost. If DOWN wins: gain downQty, lose upCost.
		lossIfUp := newDownCost - newDownQty // paid for DOWN tokens that pay $0, minus UP tokens that pay $1
		lossIfDown := newUpCost - newUpQty   // paid for UP tokens that pay $0, minus DOWN tokens that pay $1
		worstCase := math.Max(lossIfUp, lossIfDown)
		if worstCase > r.Config.MaxWorstCaseLoss {
			return "worst_case_loss_exceeded"
		}
	}

	return ""
}

// ComputeTargetSizeUSD computes the trade size in USD given edge, bankroll,
// current exposure, and the price per share. The result is snapped to whole
// shares and enforces both MinTradeSizeUSD and MinTradeShares constraints.
// imbalanceRatio is abs(upQty-downQty)/max(upQty+downQty,1) — used for Kelly decay on same-side trades.
// increasingImbalance is true if this trade adds to the heavy side.
func (r *RiskService) ComputeTargetSizeUSD(
	edge float64,
	bankrollUSD float64,
	currentMarketExposureUSD float64,
	pricePerShare float64,
	imbalanceRatio float64,
	increasingImbalance bool,
) float64 {
	if edge <= 0 || pricePerShare <= 0 {
		return 0
	}

	kelly := r.Config.FractionalKelly
	// Kelly imbalance decay: reduce sizing when adding to the heavy side
	if increasingImbalance && imbalanceRatio > 0 {
		kelly *= (1.0 - imbalanceRatio)
	}
	if kelly <= 0 {
		return 0
	}

	size := bankrollUSD * kelly * edge * 10.0
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
