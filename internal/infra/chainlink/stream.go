package chainlink

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"sync"
	"time"

	"Polybot/internal/domain"

	streams "github.com/smartcontractkit/data-streams-sdk/go"
	"github.com/smartcontractkit/data-streams-sdk/go/feed"
	streamsReport "github.com/smartcontractkit/data-streams-sdk/go/report"
	v3 "github.com/smartcontractkit/data-streams-sdk/go/report/v3"
)

// FeedMapping maps asset names to Chainlink feed IDs.
type FeedMapping struct {
	Asset  string
	FeedID feed.ID
}

// StreamConfig holds Chainlink Data Streams connection settings.
type StreamConfig struct {
	ApiKey    string // CHAINLINK_USER_ID
	ApiSecret string // CHAINLINK_SECRET
	RestURL   string
	WsURL     string
}

// Stream connects to Chainlink Data Streams for reference prices.
type Stream struct {
	config StreamConfig
	client streams.Client
	logger *slog.Logger
	mu     sync.RWMutex
	latest map[string]domain.ReferenceSnapshot
	feeds  map[string]feed.ID // asset -> feed ID
}

func NewStream(cfg StreamConfig, logger *slog.Logger) (*Stream, error) {
	s := &Stream{
		config: cfg,
		logger: logger,
		latest: make(map[string]domain.ReferenceSnapshot),
		feeds:  make(map[string]feed.ID),
	}

	// Create the Chainlink Data Streams client
	streamsCfg := streams.Config{
		ApiKey:    cfg.ApiKey,
		ApiSecret: cfg.ApiSecret,
		RestURL:   cfg.RestURL,
		WsURL:     cfg.WsURL,
		Logger:    func(format string, a ...any) { logger.Debug(fmt.Sprintf(format, a...)) },
	}

	client, err := streams.New(streamsCfg)
	if err != nil {
		return nil, fmt.Errorf("create chainlink client: %w", err)
	}
	s.client = client

	return s, nil
}

// DiscoverFeeds fetches available feeds and logs them.
func (s *Stream) DiscoverFeeds(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	feedList, err := s.client.GetFeeds(ctx)
	if err != nil {
		return fmt.Errorf("get feeds: %w", err)
	}

	s.logger.Info("discovered chainlink feeds", "count", len(feedList))
	for _, f := range feedList {
		s.logger.Info("feed", "id", f.FeedID.String())
	}
	return nil
}

// RegisterFeed maps an asset name to a Chainlink feed ID.
func (s *Stream) RegisterFeed(asset string, feedID feed.ID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.feeds[asset] = feedID
	s.logger.Info("registered feed", "asset", asset, "feed_id", feedID.String())
}

// RegisteredAssets returns the list of asset names with registered feeds.
func (s *Stream) RegisteredAssets() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	assets := make([]string, 0, len(s.feeds))
	for a := range s.feeds {
		assets = append(assets, a)
	}
	return assets
}

// RegisterFeedFromString maps an asset to a feed ID parsed from hex string.
func (s *Stream) RegisterFeedFromString(asset string, feedIDHex string) error {
	var id feed.ID
	id.FromString(feedIDHex)
	s.RegisterFeed(asset, id)
	return nil
}

func (s *Stream) GetLatestPrice(_ context.Context, asset string) (domain.ReferenceSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap, ok := s.latest[asset]
	if !ok {
		return domain.ReferenceSnapshot{}, fmt.Errorf("no price for asset %s", asset)
	}
	return snap, nil
}

// GetPriceAtTime fetches the Chainlink price for an asset at a specific timestamp via REST.
func (s *Stream) GetPriceAtTime(ctx context.Context, asset string, ts time.Time) (domain.ReferenceSnapshot, error) {
	return s.FetchReportAtTimestamp(ctx, asset, ts)
}

// SubscribePrices connects to the Chainlink WebSocket stream for one asset.
func (s *Stream) SubscribePrices(ctx context.Context, asset string) (<-chan domain.ReferenceSnapshot, error) {
	s.mu.RLock()
	feedID, ok := s.feeds[asset]
	s.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no feed registered for asset %s", asset)
	}

	ch := make(chan domain.ReferenceSnapshot, 256)

	go func() {
		defer close(ch)
		s.logger.Info("connecting to chainlink stream", "asset", asset, "feed_id", feedID.String())

		stream, err := s.client.Stream(ctx, []feed.ID{feedID})
		if err != nil {
			s.logger.Error("failed to open chainlink stream", "asset", asset, "error", err)
			return
		}
		defer stream.Close()

		s.logger.Info("chainlink stream connected", "asset", asset)

		for {
			report, err := stream.Read(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return // context cancelled, clean shutdown
				}
				s.logger.Error("chainlink stream read error", "asset", asset, "error", err)
				return
			}

			snap, err := s.reportToSnapshot(asset, report)
			if err != nil {
				s.logger.Warn("failed to decode report", "asset", asset, "error", err)
				continue
			}

			s.updateLatest(snap)

			s.logger.Info("chainlink price",
				"asset", asset,
				"price", snap.Price,
				"timestamp", snap.Timestamp.Format(time.RFC3339Nano),
			)

			select {
			case ch <- snap:
			default:
				// Channel full, skip oldest
			}
		}
	}()

	return ch, nil
}

// FetchReportAtTimestamp fetches the price for an asset at a specific timestamp.
func (s *Stream) FetchReportAtTimestamp(ctx context.Context, asset string, ts time.Time) (domain.ReferenceSnapshot, error) {
	s.mu.RLock()
	feedID, ok := s.feeds[asset]
	s.mu.RUnlock()

	if !ok {
		return domain.ReferenceSnapshot{}, fmt.Errorf("no feed registered for asset %s", asset)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	reports, err := s.client.GetReports(ctx, []feed.ID{feedID}, uint64(ts.Unix()))
	if err != nil {
		return domain.ReferenceSnapshot{}, fmt.Errorf("get report at %v: %w", ts, err)
	}
	if len(reports) == 0 {
		return domain.ReferenceSnapshot{}, fmt.Errorf("no report at %v for %s", ts, asset)
	}

	return s.reportToSnapshot(asset, reports[0])
}

// FetchLatestReport does a one-shot REST fetch of the latest price for an asset.
func (s *Stream) FetchLatestReport(ctx context.Context, asset string) (domain.ReferenceSnapshot, error) {
	s.mu.RLock()
	feedID, ok := s.feeds[asset]
	s.mu.RUnlock()

	if !ok {
		return domain.ReferenceSnapshot{}, fmt.Errorf("no feed registered for asset %s", asset)
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	report, err := s.client.GetLatestReport(ctx, feedID)
	if err != nil {
		return domain.ReferenceSnapshot{}, fmt.Errorf("get latest report: %w", err)
	}

	return s.reportToSnapshot(asset, report)
}

func (s *Stream) reportToSnapshot(asset string, report *streams.ReportResponse) (domain.ReferenceSnapshot, error) {
	decoded, err := streamsReport.Decode[v3.Data](report.FullReport)
	if err != nil {
		return domain.ReferenceSnapshot{}, fmt.Errorf("decode v3 report: %w", err)
	}

	price := bigIntToFloat64(decoded.Data.BenchmarkPrice)
	ts := time.Unix(int64(decoded.Data.ObservationsTimestamp), 0)

	return domain.ReferenceSnapshot{
		Asset:     asset,
		Price:     price,
		Timestamp: ts,
	}, nil
}

func (s *Stream) updateLatest(snap domain.ReferenceSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latest[snap.Asset] = snap
}

// SetPrice allows manual price injection (useful for testing and paper trading).
func (s *Stream) SetPrice(asset string, price float64) {
	snap := domain.ReferenceSnapshot{
		Asset:     asset,
		Price:     price,
		Timestamp: time.Now(),
	}
	s.updateLatest(snap)
}

// bigIntToFloat64 converts a *big.Int price (typically 18 decimals) to float64.
func bigIntToFloat64(val *big.Int) float64 {
	if val == nil {
		return 0
	}
	// Chainlink prices are typically scaled by 1e18
	f := new(big.Float).SetInt(val)
	divisor := new(big.Float).SetFloat64(math.Pow10(18))
	result, _ := new(big.Float).Quo(f, divisor).Float64()
	return result
}
