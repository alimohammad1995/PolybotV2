package backtest

import (
	"fmt"
	"math"
)

type TradeRecord struct {
	MarketID string
	Side     string
	Entry    float64
	SizeUSD  float64
	Won      bool
	PnL      float64
}

type Evaluator struct {
	trades []TradeRecord
}

func NewEvaluator() *Evaluator {
	return &Evaluator{}
}

func (e *Evaluator) AddTrade(t TradeRecord) {
	e.trades = append(e.trades, t)
}

func (e *Evaluator) Evaluate() EvaluationReport {
	if len(e.trades) == 0 {
		return EvaluationReport{}
	}

	report := EvaluationReport{
		TotalTrades: len(e.trades),
	}

	var wins int
	var cumPnL float64
	var maxPnL float64
	maxDrawdown := 0.0

	for _, t := range e.trades {
		report.TotalPnL += t.PnL
		report.TotalVolume += t.SizeUSD
		if t.Won {
			wins++
			report.GrossProfit += t.PnL
		} else {
			report.GrossLoss += t.PnL
		}

		cumPnL += t.PnL
		if cumPnL > maxPnL {
			maxPnL = cumPnL
		}
		dd := maxPnL - cumPnL
		if dd > maxDrawdown {
			maxDrawdown = dd
		}
	}

	report.WinRate = float64(wins) / float64(len(e.trades))
	report.MaxDrawdown = maxDrawdown
	if report.GrossLoss != 0 {
		report.ProfitFactor = math.Abs(report.GrossProfit / report.GrossLoss)
	}

	// Sharpe approximation
	if len(e.trades) > 1 {
		var pnls []float64
		for _, t := range e.trades {
			pnls = append(pnls, t.PnL)
		}
		mean, std := meanStd(pnls)
		if std > 0 {
			report.SharpeApprox = mean / std * math.Sqrt(float64(len(e.trades)))
		}
	}

	return report
}

type EvaluationReport struct {
	TotalTrades  int
	TotalPnL     float64
	TotalVolume  float64
	WinRate      float64
	MaxDrawdown  float64
	GrossProfit  float64
	GrossLoss    float64
	ProfitFactor float64
	SharpeApprox float64
}

func (r EvaluationReport) String() string {
	return fmt.Sprintf(
		"Trades=%d PnL=%.2f Vol=%.2f WR=%.1f%% MDD=%.2f PF=%.2f Sharpe~%.2f",
		r.TotalTrades, r.TotalPnL, r.TotalVolume,
		r.WinRate*100, r.MaxDrawdown, r.ProfitFactor, r.SharpeApprox,
	)
}

func meanStd(vals []float64) (float64, float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean := sum / float64(len(vals))

	var sumSq float64
	for _, v := range vals {
		d := v - mean
		sumSq += d * d
	}
	std := math.Sqrt(sumSq / float64(len(vals)))
	return mean, std
}
