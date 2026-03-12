package strategy

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"Polybot/internal/domain"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestStrategyRunner_CheckFreshness(t *testing.T) {
	runner := &StrategyRunner{
		Freshness: FreshnessConfig{
			MaxReferenceAge:  10 * time.Second,
			MaxQuoteAge:      30 * time.Second,
			MaxAllowedSpread: 0.10,
		},
	}

	now := time.Now()

	t.Run("fresh_data_passes", func(t *testing.T) {
		reason := runner.checkFreshness(now,
			&domain.ReferenceState{LastUpdate: now.Add(-5 * time.Second)},
			&domain.MarketState{Timestamp: now.Add(-10 * time.Second)},
		)
		if reason != "" {
			t.Errorf("expected no rejection, got %s", reason)
		}
	})

	t.Run("stale_chainlink_rejected", func(t *testing.T) {
		reason := runner.checkFreshness(now,
			&domain.ReferenceState{LastUpdate: now.Add(-15 * time.Second)},
			&domain.MarketState{Timestamp: now.Add(-5 * time.Second)},
		)
		if reason != "stale_chainlink" {
			t.Errorf("expected stale_chainlink, got %q", reason)
		}
	})

	t.Run("stale_polymarket_rejected", func(t *testing.T) {
		reason := runner.checkFreshness(now,
			&domain.ReferenceState{LastUpdate: now.Add(-5 * time.Second)},
			&domain.MarketState{Timestamp: now.Add(-60 * time.Second)},
		)
		if reason != "stale_polymarket_quote" {
			t.Errorf("expected stale_polymarket_quote, got %q", reason)
		}
	})
}

func TestStrategyRunner_ValidateQuotes(t *testing.T) {
	runner := &StrategyRunner{}

	t.Run("valid_quotes_pass", func(t *testing.T) {
		err := runner.validateQuotes(&domain.MarketState{
			UpBid: 0.40, UpAsk: 0.55,
			DownBid: 0.35, DownAsk: 0.50,
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("zero_ask_rejected", func(t *testing.T) {
		err := runner.validateQuotes(&domain.MarketState{
			UpBid: 0.40, UpAsk: 0,
			DownBid: 0.35, DownAsk: 0.50,
		})
		if err == nil {
			t.Error("expected error for zero up ask")
		}
	})

	t.Run("ask_above_1_rejected", func(t *testing.T) {
		err := runner.validateQuotes(&domain.MarketState{
			UpBid: 0.40, UpAsk: 1.5,
			DownBid: 0.35, DownAsk: 0.50,
		})
		if err == nil {
			t.Error("expected error for ask > 1.0")
		}
	})

	t.Run("bid_above_ask_rejected", func(t *testing.T) {
		err := runner.validateQuotes(&domain.MarketState{
			UpBid: 0.60, UpAsk: 0.55,
			DownBid: 0.35, DownAsk: 0.50,
		})
		if err == nil {
			t.Error("expected error for bid >= ask")
		}
	})
}

func TestStrategyRunner_EvaluateMarket_SafetyGuards(t *testing.T) {
	ctx := context.Background()

	t.Run("spread_too_wide_skips", func(t *testing.T) {
		runner := &StrategyRunner{
			Freshness: FreshnessConfig{
				MaxAllowedSpread: 0.05,
			},
			Clock:  &mockClock{now: time.Now()},
			Logger: testLogger(),
		}

		err := runner.EvaluateMarket(ctx,
			&domain.BinaryMarket{ID: "m1", SettlementTime: time.Now().Add(5 * time.Minute)},
			&domain.ReferenceState{CurrentPrice: 100, LastUpdate: time.Now()},
			&domain.MarketState{
				UpBid: 0.30, UpAsk: 0.55, DownBid: 0.30, DownAsk: 0.55,
				Spread:    0.25,
				Timestamp: time.Now(),
			},
			1000,
		)
		// Should return nil (skipped, not error)
		if err != nil {
			t.Errorf("expected nil (skipped), got %v", err)
		}
	})

	t.Run("extreme_jump_rejects", func(t *testing.T) {
		runner := &StrategyRunner{
			Freshness: FreshnessConfig{
				MaxAllowedSpread: 0.50,
			},
			Clock:  &mockClock{now: time.Now()},
			Logger: testLogger(),
		}

		err := runner.EvaluateMarket(ctx,
			&domain.BinaryMarket{ID: "m1", SettlementTime: time.Now().Add(5 * time.Minute)},
			&domain.ReferenceState{
				CurrentPrice: 100,
				JumpScore:    8.0,
				LastUpdate:   time.Now(),
			},
			&domain.MarketState{
				UpBid: 0.40, UpAsk: 0.55, DownBid: 0.35, DownAsk: 0.50,
				Spread:    0.05,
				Timestamp: time.Now(),
			},
			1000,
		)
		if err != nil {
			t.Errorf("expected nil (skipped), got %v", err)
		}
	})
}

type mockClock struct {
	now time.Time
}

func (c *mockClock) Now() time.Time { return c.now }
