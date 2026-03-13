package polymarket

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"Polybot/internal/domain"
	"Polybot/internal/service"
)

// FillListener subscribes to the Polymarket user WebSocket channel and
// forwards confirmed trade fills to the PositionService.
type FillListener struct {
	client      *ClobClient
	registry    *service.MarketRegistry
	positionSvc *service.PositionService
	logger      *slog.Logger
}

func NewFillListener(
	client *ClobClient,
	registry *service.MarketRegistry,
	positionSvc *service.PositionService,
	logger *slog.Logger,
) *FillListener {
	return &FillListener{
		client:      client,
		registry:    registry,
		positionSvc: positionSvc,
		logger:      logger,
	}
}

// Run connects to the Polymarket user WebSocket and listens for trade events.
// It reconnects automatically on disconnection. Blocks until ctx is cancelled.
func (f *FillListener) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		f.logger.Info("fill_listener: connecting to user channel")
		err := f.stream(ctx)
		if ctx.Err() != nil {
			return
		}
		f.logger.Warn("fill_listener: disconnected, reconnecting", "error", err)
		time.Sleep(2 * time.Second)
	}
}

func (f *FillListener) stream(ctx context.Context) error {
	msgCh := make(chan []byte, 256)

	ws := NewWebSocketOrderBook(UserChannel, func(message []byte) {
		select {
		case msgCh <- message:
		default:
		}
	})

	// The user channel requires L2 auth headers for the initial subscription.
	// We authenticate by sending the API key in the subscription payload.
	connectDone := make(chan error, 1)
	go func() {
		connectDone <- ws.Run(map[string]any{
			"auth":    f.buildAuthPayload(),
			"type":    UserChannel,
			"markets": []string{}, // empty = subscribe to all user events
		})
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-connectDone:
			return err
		case raw := <-msgCh:
			rawStr := string(raw)
			if rawStr == "PONG" || rawStr == "" {
				continue
			}
			f.handleMessage(ctx, raw)
		}
	}
}

func (f *FillListener) buildAuthPayload() map[string]any {
	if f.client.creds == nil {
		return nil
	}
	return map[string]any{
		"apiKey":     f.client.creds.APIKey,
		"secret":     f.client.creds.APISecret,
		"passphrase": f.client.creds.APIPassphrase,
	}
}

// userWSMessage represents a Polymarket user channel WebSocket message.
// The user channel sends trade/order events with this structure.
type userWSMessage struct {
	EventType string        `json:"event_type"`
	Trades    []userWSTrade `json:"trades"`
}

type userWSTrade struct {
	AssetID    string      `json:"asset_id"`
	Side       string      `json:"side"` // "BUY" or "SELL"
	Price      json.Number `json:"price"`
	Size       json.Number `json:"size"`
	Status     string      `json:"status"`      // "MATCHED", "MINED", etc.
	TraderSide string      `json:"trader_side"` // "MAKER" or "TAKER"
	Market     string      `json:"market"`
}

func (f *FillListener) handleMessage(ctx context.Context, raw []byte) {
	// Try single message
	var msg userWSMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		// Try array of messages
		var msgs []userWSMessage
		if err2 := json.Unmarshal(raw, &msgs); err2 != nil {
			return
		}
		for _, m := range msgs {
			f.processMessage(ctx, &m)
		}
		return
	}
	f.processMessage(ctx, &msg)
}

func (f *FillListener) processMessage(ctx context.Context, msg *userWSMessage) {
	for _, trade := range msg.Trades {
		// Only process confirmed fills
		if trade.Status != "MATCHED" && trade.Status != "MINED" {
			continue
		}
		// Only process buys (we don't sell mid-market)
		if trade.Side != "BUY" {
			continue
		}

		price, err := trade.Price.Float64()
		if err != nil || price <= 0 {
			continue
		}
		size, err := trade.Size.Float64()
		if err != nil || size <= 0 {
			continue
		}

		// Resolve which market and side this token belongs to
		marketID, signalSide, ok := f.resolveTokenSide(trade.AssetID)
		if !ok {
			f.logger.Debug("fill_listener: unknown token in trade", "asset_id", trade.AssetID)
			continue
		}

		sizeUSD := price * size

		fill := domain.Fill{
			MarketID:  marketID,
			Side:      signalSide,
			Price:     price,
			SizeUSD:   sizeUSD,
			Timestamp: time.Now(),
		}

		f.logger.Info("fill_listener: confirmed fill",
			"market", marketID,
			"side", signalSide,
			"price", price,
			"size", size,
			"size_usd", sizeUSD,
			"asset_id", trade.AssetID,
		)

		if err := f.positionSvc.AccumulateFill(ctx, fill); err != nil {
			f.logger.Error("fill_listener: failed to accumulate fill", "error", err)
		}
	}
}

// resolveTokenSide maps a token asset ID to a market ID and signal side
// by looking through the registry for markets that use this token.
func (f *FillListener) resolveTokenSide(assetID string) (domain.MarketID, domain.TradeSignalSide, bool) {
	for _, market := range f.registry.ListMarkets() {
		if market.UpTokenID == assetID {
			return market.ID, domain.SignalBuyUp, true
		}
		if market.DownTokenID == assetID {
			return market.ID, domain.SignalBuyDown, true
		}
	}
	return "", "", false
}

// LoadTradesFromAPI fetches recent trades from the Polymarket REST API
// and accumulates them into positions. Used for bootstrap on startup.
func (f *FillListener) LoadTradesFromAPI(ctx context.Context) error {
	address := f.client.Address()
	if address == "" {
		return nil
	}

	resp, err := f.client.GetTradesTyped(map[string]string{
		"maker_address": address,
	})
	if err != nil {
		return err
	}

	loaded := 0
	for _, trade := range resp.Data {
		if trade.Side != "BUY" {
			continue
		}

		price, err := trade.Price.Float64()
		if err != nil || price <= 0 {
			continue
		}
		size, err := trade.Size.Float64()
		if err != nil || size <= 0 {
			continue
		}

		marketID, signalSide, ok := f.resolveTokenSide(trade.AssetID)
		if !ok {
			// Token not in any active market — skip
			continue
		}

		sizeUSD := price * size
		fill := domain.Fill{
			MarketID:  marketID,
			Side:      signalSide,
			Price:     price,
			SizeUSD:   sizeUSD,
			Timestamp: time.Now(),
		}

		if err := f.positionSvc.AccumulateFill(ctx, fill); err != nil {
			f.logger.Error("bootstrap: failed to accumulate trade", "error", err)
			continue
		}
		loaded++
	}

	f.logger.Info("bootstrap: loaded trades from API", "count", loaded, "total_api_trades", len(resp.Data))
	return nil
}

// LoadPositionsFromAPI fetches current positions from the Polymarket data API
// and sets them directly. This is a more accurate bootstrap than loading trades
// because it reflects the current on-chain state.
func (f *FillListener) LoadPositionsFromAPI(ctx context.Context) error {
	address := f.client.Address()
	if address == "" {
		return nil
	}

	resp, err := f.client.GetPositions(address, nil)
	if err != nil {
		return err
	}

	positions, ok := resp.([]any)
	if !ok {
		// Try map response
		return f.LoadTradesFromAPI(ctx)
	}

	loaded := 0
	for _, pos := range positions {
		pm, ok := pos.(map[string]any)
		if !ok {
			continue
		}

		assetID, _ := pm["asset"].(string)
		if assetID == "" {
			assetID, _ = pm["asset_id"].(string)
		}
		sizeStr, _ := pm["size"].(string)
		if sizeStr == "" {
			if sizeNum, ok := pm["size"].(float64); ok {
				sizeStr = strconv.FormatFloat(sizeNum, 'f', -1, 64)
			}
		}

		size, err := strconv.ParseFloat(sizeStr, 64)
		if err != nil || size <= 0 {
			continue
		}

		avgPriceStr, _ := pm["avg_price"].(string)
		if avgPriceStr == "" {
			if priceNum, ok := pm["avg_price"].(float64); ok {
				avgPriceStr = strconv.FormatFloat(priceNum, 'f', -1, 64)
			}
		}
		avgPrice, _ := strconv.ParseFloat(avgPriceStr, 64)
		if avgPrice <= 0 {
			avgPrice = 0.5 // fallback
		}

		marketID, signalSide, ok := f.resolveTokenSide(assetID)
		if !ok {
			continue
		}

		var posSide domain.PositionSide
		if signalSide == domain.SignalBuyUp {
			posSide = domain.PositionUp
		} else {
			posSide = domain.PositionDown
		}

		p := domain.Position{
			MarketID:         marketID,
			Side:             posSide,
			AvgEntryPrice:    avgPrice,
			Quantity:         size,
			NotionalUSD:      avgPrice * size,
			OpenedAt:         time.Now(),
			HoldToSettlement: true,
		}

		if err := f.positionSvc.RecordPosition(ctx, p); err != nil {
			f.logger.Error("bootstrap: failed to record position", "error", err)
			continue
		}
		loaded++
	}

	f.logger.Info("bootstrap: loaded positions from API", "count", loaded)
	return nil
}
