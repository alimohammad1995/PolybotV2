package polymarket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"Polybot/internal/domain"
)

// MarketProvider resolves rolling Polymarket binary options and streams order books via WebSocket.
type MarketProvider struct {
	gamma    *GammaMarket
	clob     *ClobClient
	asset    string // e.g. "btc"
	interval int    // 5 or 15 (minutes)
	logger   *slog.Logger

	// Live order book state, updated by WebSocket
	mu       sync.RWMutex
	upBook   OrderBookSummary
	downBook OrderBookSummary
}

func NewMarketProvider(clob *ClobClient, asset string, interval int, logger *slog.Logger) *MarketProvider {
	return &MarketProvider{
		gamma:    NewGammaMarket(),
		clob:     clob,
		asset:    strings.ToLower(asset),
		interval: interval,
		logger:   logger,
	}
}

// CurrentSlug returns the slug for the currently active market window.
func (p *MarketProvider) CurrentSlug() string {
	intervalSecs := int64(p.interval * 60)
	now := time.Now().Unix()
	ts := now - now%intervalSecs + intervalSecs
	return fmt.Sprintf("%s-updown-%dm-%d", p.asset, p.interval, ts)
}

// NextSlug returns the slug for the next market window after the current one.
func (p *MarketProvider) NextSlug() string {
	intervalSecs := int64(p.interval * 60)
	now := time.Now().Unix()
	ts := now - now%intervalSecs + 2*intervalSecs
	return fmt.Sprintf("%s-updown-%dm-%d", p.asset, p.interval, ts)
}

// ResolveCurrentMarket fetches the current market from the gamma API.
func (p *MarketProvider) ResolveCurrentMarket() (domain.BinaryMarket, error) {
	return p.resolveMarket(p.CurrentSlug())
}

func (p *MarketProvider) resolveMarket(slug string) (domain.BinaryMarket, error) {
	summary, err := p.gamma.GetMarketBySlug(slug)
	if err != nil {
		return domain.BinaryMarket{}, fmt.Errorf("resolve %s: %w", slug, err)
	}

	return domain.BinaryMarket{
		ID:          domain.MarketID(summary.MarketID),
		Slug:        slug,
		Asset:       strings.ToUpper(p.asset),
		StartTime:   time.Unix(summary.StartDateTS, 0),
		EndTime:     time.Unix(summary.EndDateTS, 0),
		PriceToBeat: 0,
		Status:      domain.MarketStatusActive,
		UpTokenID:   summary.ClobTokenIDs[0],
		DownTokenID: summary.ClobTokenIDs[1],
	}, nil
}

// GetActiveMarkets returns the single currently active market.
func (p *MarketProvider) GetActiveMarkets(_ context.Context) ([]domain.BinaryMarket, error) {
	market, err := p.ResolveCurrentMarket()
	if err != nil {
		return nil, err
	}
	return []domain.BinaryMarket{market}, nil
}

// GetQuote returns the latest order book state (from REST fallback or cached WS state).
func (p *MarketProvider) GetQuote(_ context.Context, marketID domain.MarketID) (domain.MarketQuote, error) {
	p.mu.RLock()
	up := p.upBook
	down := p.downBook
	p.mu.RUnlock()

	// If WS has populated the books, use them
	if len(up.Bids) > 0 || len(up.Asks) > 0 || len(down.Bids) > 0 || len(down.Asks) > 0 {
		return domain.MarketQuote{
			MarketID:  marketID,
			Up:        extractBestQuote(up),
			Down:      extractBestQuote(down),
			Timestamp: time.Now(),
		}, nil
	}

	// Fallback: REST fetch
	return p.fetchQuoteREST(marketID)
}

func (p *MarketProvider) fetchQuoteREST(marketID domain.MarketID) (domain.MarketQuote, error) {
	slug := p.CurrentSlug()
	summary, err := p.gamma.GetMarketBySlug(slug)
	if err != nil {
		return domain.MarketQuote{}, fmt.Errorf("get quote: resolve slug: %w", err)
	}

	books, err := p.clob.GetOrderBooks(summary.ClobTokenIDs)
	if err != nil {
		return domain.MarketQuote{}, fmt.Errorf("get order books: %w", err)
	}

	upBook := books[summary.ClobTokenIDs[0]]
	downBook := books[summary.ClobTokenIDs[1]]

	p.mu.Lock()
	p.upBook = upBook
	p.downBook = downBook
	p.mu.Unlock()

	return domain.MarketQuote{
		MarketID:  marketID,
		Up:        extractBestQuote(upBook),
		Down:      extractBestQuote(downBook),
		Timestamp: time.Now(),
	}, nil
}

// SubscribeQuotes connects to the Polymarket WebSocket and streams live order book updates.
func (p *MarketProvider) SubscribeQuotes(ctx context.Context, _ []domain.MarketID) (<-chan domain.MarketQuote, error) {
	ch := make(chan domain.MarketQuote, 64)

	go func() {
		defer close(ch)
		p.runWSLoop(ctx, ch)
	}()

	return ch, nil
}

func (p *MarketProvider) runWSLoop(ctx context.Context, ch chan<- domain.MarketQuote) {
	for {
		if ctx.Err() != nil {
			return
		}

		slug := p.CurrentSlug()
		summary, err := p.gamma.GetMarketBySlug(slug)
		if err != nil {
			p.logger.Warn("ws: waiting for market", "slug", slug, "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
				continue
			}
		}

		upTokenID := summary.ClobTokenIDs[0]
		downTokenID := summary.ClobTokenIDs[1]
		marketID := domain.MarketID(summary.MarketID)

		// Seed the books with a REST fetch first
		p.seedBooks(upTokenID, downTokenID)

		p.logger.Info("ws: connecting to polymarket orderbook",
			"slug", slug,
			"up_token", upTokenID,
			"down_token", downTokenID,
		)

		// Connect WebSocket and stream until error or market expires
		err = p.streamWS(ctx, upTokenID, downTokenID, marketID, ch)
		if err != nil && ctx.Err() == nil {
			p.logger.Warn("ws: connection lost, reconnecting", "error", err)
			time.Sleep(1 * time.Second)
		}
	}
}

func (p *MarketProvider) seedBooks(upTokenID, downTokenID string) {
	books, err := p.clob.GetOrderBooks([]string{upTokenID, downTokenID})
	if err != nil {
		p.logger.Warn("ws: failed to seed books via REST", "error", err)
		return
	}

	p.mu.Lock()
	p.upBook = books[upTokenID]
	p.downBook = books[downTokenID]
	p.mu.Unlock()
}

// wsMessage represents a Polymarket WebSocket market channel message.
// Two formats:
// 1. Book snapshot (array): [{"asset_id":"...", "bids":[...], "asks":[...], ...}]
// 2. Price change: {"market":"...", "price_changes":[{"asset_id":"...", "price":"...", "size":"...", "side":"..."}]}
type wsMessage struct {
	// Book snapshot fields
	EventType string       `json:"event_type"`
	AssetID   string       `json:"asset_id"`
	Market    string       `json:"market"`
	Bids      []wsBookSide `json:"bids"`
	Asks      []wsBookSide `json:"asks"`
	Timestamp string       `json:"timestamp"`
	Hash      string       `json:"hash"`

	// Price change fields
	PriceChanges []wsPriceChange `json:"price_changes"`
}

type wsPriceChange struct {
	AssetID string `json:"asset_id"`
	Price   string `json:"price"`
	Size    string `json:"size"`
	Side    string `json:"side"`
}

type wsBookSide struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

func (p *MarketProvider) streamWS(ctx context.Context, upTokenID, downTokenID string, marketID domain.MarketID, ch chan<- domain.MarketQuote) error {
	msgCh := make(chan []byte, 256)

	ws := NewWebSocketOrderBook(MarketChannel, func(message []byte) {
		select {
		case msgCh <- message:
		default:
		}
	})

	// Connect WS with initial subscription payload
	connectDone := make(chan error, 1)
	go func() {
		connectDone <- ws.Run(map[string]any{
			"assets_ids": []string{upTokenID, downTokenID},
			"type":       MarketChannel,
		})
	}()

	// Also send explicit subscribe after connection establishes
	time.Sleep(1 * time.Second)
	if err := ws.SubscribeToTokenIDs([]string{upTokenID, downTokenID}); err != nil {
		p.logger.Warn("ws: explicit subscribe failed (initial may have worked)", "error", err)
	}
	p.logger.Info("ws: subscribed to token IDs", "up", upTokenID[:20]+"...", "down", downTokenID[:20]+"...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-connectDone:
			return fmt.Errorf("ws disconnected: %w", err)
		case raw := <-msgCh:
			rawStr := string(raw)
			if rawStr == "PONG" || rawStr == "" || rawStr == "NO NEW ASSETS" {
				continue
			}

			p.logger.Debug("ws: raw message", "len", len(raw), "preview", rawStr[:min(len(rawStr), 200)])

			var msgs []wsMessage
			// Try array first, then single message
			if err := json.Unmarshal(raw, &msgs); err != nil {
				var single wsMessage
				if err2 := json.Unmarshal(raw, &single); err2 != nil {
					continue
				}
				msgs = []wsMessage{single}
			}

			updated := false
			for _, msg := range msgs {
				if p.applyWSMessage(msg, upTokenID, downTokenID) {
					updated = true
				}
			}

			if updated {
				p.mu.RLock()
				quote := domain.MarketQuote{
					MarketID:  marketID,
					Up:        extractBestQuote(p.upBook),
					Down:      extractBestQuote(p.downBook),
					Timestamp: time.Now(),
				}
				p.mu.RUnlock()

				p.logger.Debug("ws: quote update",
					"up_bid", quote.Up.Bid,
					"up_ask", quote.Up.Ask,
					"down_bid", quote.Down.Bid,
					"down_ask", quote.Down.Ask,
				)

				select {
				case ch <- quote:
				default:
				}
			}
		}
	}
}

// parsedChange holds pre-parsed price change data computed outside the lock.
type parsedChange struct {
	isUp   bool
	isBid  bool
	price  string
	size   string // kept as string to avoid FormatFloat allocation
	isZero bool
}

func (p *MarketProvider) applyWSMessage(msg wsMessage, upTokenID, downTokenID string) bool {
	// Book snapshot: top-level asset_id with bids/asks
	if len(msg.Bids) > 0 || len(msg.Asks) > 0 {
		// Pre-compute conversion outside lock
		bids := wsBookSidesToOrderSummary(msg.Bids)
		asks := wsBookSidesToOrderSummary(msg.Asks)

		p.mu.Lock()
		book := &p.downBook
		if msg.AssetID == upTokenID {
			book = &p.upBook
		} else if msg.AssetID != downTokenID {
			p.mu.Unlock()
			return false
		}
		book.Bids = bids
		book.Asks = asks
		p.mu.Unlock()
		return true
	}

	// Price changes: parse all changes outside the lock
	if len(msg.PriceChanges) > 0 {
		changes := make([]parsedChange, 0, len(msg.PriceChanges))
		for _, change := range msg.PriceChanges {
			isUp := change.AssetID == upTokenID
			if !isUp && change.AssetID != downTokenID {
				continue
			}
			side := change.Side
			isBid := side == "buy" || side == "BUY" || side == "bid" || side == "BID"
			changes = append(changes, parsedChange{
				isUp:   isUp,
				isBid:  isBid,
				price:  change.Price,
				size:   change.Size,
				isZero: change.Size == "0" || change.Size == "0.0" || change.Size == "0.00",
			})
		}

		if len(changes) == 0 {
			return false
		}

		p.mu.Lock()
		for _, c := range changes {
			targetBook := &p.downBook
			if c.isUp {
				targetBook = &p.upBook
			}
			if c.isBid {
				targetBook.Bids = applyChange(targetBook.Bids, c.price, c.size, c.isZero)
			} else {
				targetBook.Asks = applyChange(targetBook.Asks, c.price, c.size, c.isZero)
			}
		}
		p.mu.Unlock()
		return true
	}

	return false
}

// applyChange updates a price level. If isZero, removes it. Otherwise upserts.
// Uses the raw size string from WS to avoid FormatFloat allocation.
func applyChange(levels []OrderSummary, price, size string, isZero bool) []OrderSummary {
	if isZero {
		for i, l := range levels {
			if l.Price == price {
				return append(levels[:i], levels[i+1:]...)
			}
		}
		return levels
	}

	for i, l := range levels {
		if l.Price == price {
			levels[i].Size = size
			return levels
		}
	}
	return append(levels, OrderSummary{Price: price, Size: size})
}

func wsBookSidesToOrderSummary(sides []wsBookSide) []OrderSummary {
	result := make([]OrderSummary, len(sides))
	for i, s := range sides {
		result[i] = OrderSummary{Price: s.Price, Size: s.Size}
	}
	return result
}

// SecondsUntilExpiry returns how many seconds remain in the current market window.
func (p *MarketProvider) SecondsUntilExpiry() float64 {
	intervalSecs := int64(p.interval * 60)
	now := time.Now().Unix()
	endTS := now - now%intervalSecs + intervalSecs
	return float64(endTS - now)
}

func extractBestQuote(book OrderBookSummary) domain.SideQuote {
	var bestBid, bestAsk float64

	if len(book.Bids) > 0 {
		bestBid, _ = strconv.ParseFloat(book.Bids[0].Price, 64)
	}
	if len(book.Asks) > 0 {
		bestAsk, _ = strconv.ParseFloat(book.Asks[0].Price, 64)
	}

	return domain.SideQuote{Bid: bestBid, Ask: bestAsk}
}
