package backtest

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"Polybot/internal/domain"
	"Polybot/internal/strategy"
)

type MarketSnapshot struct {
	Timestamp time.Time
	Market    domain.BinaryMarket
	Quote     domain.MarketQuote
	RefPrice  float64
}

type SettlementRecord struct {
	MarketID domain.MarketID
	Outcome  string // "up" or "down"
	Time     time.Time
}

type BacktestConfig struct {
	BankrollUSD float64
	StartTime   time.Time
	EndTime     time.Time
}

type Engine struct {
	runner      *strategy.StrategyRunner
	config      BacktestConfig
	logger      *slog.Logger
	snapshots   []MarketSnapshot
	settlements []SettlementRecord
}

func NewEngine(
	runner *strategy.StrategyRunner,
	config BacktestConfig,
	logger *slog.Logger,
) *Engine {
	return &Engine{
		runner: runner,
		config: config,
		logger: logger,
	}
}

func (e *Engine) AddSnapshot(snap MarketSnapshot) {
	e.snapshots = append(e.snapshots, snap)
}

func (e *Engine) AddSettlement(s SettlementRecord) {
	e.settlements = append(e.settlements, s)
}

func (e *Engine) Run(ctx context.Context) (*BacktestResult, error) {
	// Sort snapshots by timestamp
	sort.Slice(e.snapshots, func(i, j int) bool {
		return e.snapshots[i].Timestamp.Before(e.snapshots[j].Timestamp)
	})

	result := &BacktestResult{}

	for i, snap := range e.snapshots {
		if snap.Timestamp.Before(e.config.StartTime) || snap.Timestamp.After(e.config.EndTime) {
			continue
		}

		err := e.runner.OnMarketUpdate(
			ctx,
			snap.Market,
			snap.Quote,
			snap.RefPrice,
			e.config.BankrollUSD,
		)
		if err != nil {
			e.logger.Warn("backtest evaluation error",
				"index", i,
				"market", snap.Market.ID,
				"error", err,
			)
			result.Errors++
			continue
		}
		result.Evaluations++
	}

	return result, nil
}

type BacktestResult struct {
	Evaluations int
	Trades      int
	Errors      int
	TotalPnL    float64
	WinRate     float64
	MaxDrawdown float64
}

func (r *BacktestResult) String() string {
	return fmt.Sprintf(
		"Evaluations=%d Trades=%d Errors=%d PnL=%.2f WinRate=%.2f%% MaxDD=%.2f%%",
		r.Evaluations, r.Trades, r.Errors, r.TotalPnL, r.WinRate*100, r.MaxDrawdown*100,
	)
}
