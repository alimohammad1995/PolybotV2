package service

type RiskConfig struct {
	MaxPositionUSDPerMarket float64
	MaxTotalExposureUSD     float64
	NoNewTradeCutoffSecs    float64
	FractionalKelly         float64
	MinTradeSizeUSD         float64
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

func (r *RiskService) ComputeTargetSizeUSD(
	edge float64,
	bankrollUSD float64,
	currentMarketExposureUSD float64,
) float64 {
	if edge <= 0 {
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
	return size
}
