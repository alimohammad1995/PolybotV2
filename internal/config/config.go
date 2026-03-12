package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	// Trading parameters
	BaseHurdle              float64       `yaml:"base_hurdle"`
	DefaultModelUncertainty float64       `yaml:"default_model_uncertainty"`
	MaxPositionUSDPerMarket float64       `yaml:"max_position_usd_per_market"`
	MaxTotalExposureUSD     float64       `yaml:"max_total_exposure_usd"`
	NoNewTradeCutoffSecs    float64       `yaml:"no_new_trade_cutoff_secs"`
	MinTradeSizeUSD         float64       `yaml:"min_trade_size_usd"`
	FractionalKelly         float64       `yaml:"fractional_kelly"`
	MaxAllowedSpread        float64       `yaml:"max_allowed_spread"`
	MaxQuoteAge             time.Duration `yaml:"max_quote_age"`
	MaxReferenceAge         time.Duration `yaml:"max_reference_age"`
	BankrollUSD             float64       `yaml:"bankroll_usd"`
	WorkerCount             int           `yaml:"worker_count"`

	// Polymarket
	PrivateKey    string
	FunderAddress string

	// Chainlink Data Streams
	ChainlinkWSURL   string
	ChainlinkRestURL string
	ChainlinkUserID  string
	ChainlinkSecret  string

	// Market: single asset this instance trades (btc, eth, sol, xrp)
	Market string
	// Interval: market duration in minutes (5 or 15)
	Interval int

	// Mode: "live", "paper", or "debug"
	Mode string

	// Model params file (optional)
	ModelParamsFile string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		// Conservative defaults
		BaseHurdle:              0.03,
		DefaultModelUncertainty: 0.02,
		MaxPositionUSDPerMarket: 50.0,
		MaxTotalExposureUSD:     200.0,
		NoNewTradeCutoffSecs:    60.0,
		MinTradeSizeUSD:         1.0,
		FractionalKelly:         0.05,
		MaxAllowedSpread:        0.10,
		MaxQuoteAge:             30 * time.Second,
		MaxReferenceAge:         10 * time.Second,
		BankrollUSD:             100.0,
		WorkerCount:             8,
		Market:                  "btc",
		Interval:                5,
		Mode:                    "paper",
	}

	cfg.PrivateKey = os.Getenv("MAIN_ACCOUNT_PRIVATE_KEY")
	cfg.FunderAddress = os.Getenv("MAIN_ACCOUNT_FUNDER_ADDRESS")
	cfg.ChainlinkWSURL = os.Getenv("CHAINLINK_WS_URL")
	cfg.ChainlinkRestURL = os.Getenv("CHAINLINK_REST_URL")
	cfg.ChainlinkUserID = os.Getenv("CHAINLINK_USER_ID")
	cfg.ChainlinkSecret = os.Getenv("CHAINLINK_SECRET")
	cfg.ModelParamsFile = os.Getenv("MODEL_PARAMS_FILE")

	if mode := os.Getenv("MODE"); mode != "" {
		cfg.Mode = mode
	}
	if market := os.Getenv("MARKET"); market != "" {
		cfg.Market = strings.ToLower(market)
	}
	if interval := os.Getenv("INTERVAL"); interval != "" {
		if v, err := strconv.Atoi(interval); err == nil {
			cfg.Interval = v
		}
	}

	return cfg, nil
}
