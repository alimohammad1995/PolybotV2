package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"Polybot/internal/domain"
	"Polybot/internal/service"
)

// FullTickSnapshot is the full state logged per tick for debugging and optimization.
type FullTickSnapshot struct {
	Ts               string  `json:"ts"`
	RefPrice         float64 `json:"ref_price"`
	PriceToBeat      float64 `json:"price_to_beat"`
	RemainingMs      float64 `json:"remaining_ms"`
	SigmaTau         float64 `json:"sigma_tau"`
	Z                float64 `json:"z"`
	PRaw             float64 `json:"p_raw"`
	PCal             float64 `json:"p_cal"`
	PLo              float64 `json:"p_lo"`
	PHi              float64 `json:"p_hi"`
	UpBid            float64 `json:"up_bid"`
	UpAsk            float64 `json:"up_ask"`
	DownBid          float64 `json:"down_bid"`
	DownAsk          float64 `json:"down_ask"`
	DirEdgeUp        float64 `json:"dir_edge_up"`
	DirEdgeDown      float64 `json:"dir_edge_down"`
	UpQty            float64 `json:"up_qty"`
	DownQty          float64 `json:"down_qty"`
	UpCost           float64 `json:"up_cost"`
	DownCost         float64 `json:"down_cost"`
	GuaranteedFloor  float64 `json:"guaranteed_floor"`
	HedgeEdgeBuyDown float64 `json:"hedge_edge_buy_down"`
	HedgeEdgeBuyUp   float64 `json:"hedge_edge_buy_up"`
	Action           string  `json:"action"`
}

type PriceTracker struct {
	registry     *service.MarketRegistry
	refAnalytics *service.ReferenceAnalyticsService
	pricingModel service.PricingModel
	positionSvc  *service.PositionService
	hedgeEngine  *service.HedgeEngine
	logDir       string
	logger       *slog.Logger
}

func NewPriceTracker(
	registry *service.MarketRegistry,
	refAnalytics *service.ReferenceAnalyticsService,
	pricingModel service.PricingModel,
	positionSvc *service.PositionService,
	hedgeEngine *service.HedgeEngine,
	logDir string,
	logger *slog.Logger,
) *PriceTracker {
	return &PriceTracker{
		registry:     registry,
		refAnalytics: refAnalytics,
		pricingModel: pricingModel,
		positionSvc:  positionSvc,
		hedgeEngine:  hedgeEngine,
		logDir:       logDir,
		logger:       logger,
	}
}

func (t *PriceTracker) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var (
		file        *os.File
		currentSlug string
	)

	for {
		select {
		case <-ctx.Done():
			if file != nil {
				file.Close()
			}
			return
		case <-ticker.C:
			markets := t.registry.ListMarkets()
			if len(markets) == 0 {
				continue
			}
			market := markets[0]

			// Rotate file on market change
			if market.Slug != currentSlug {
				if file != nil {
					file.Close()
				}
				currentSlug = market.Slug
				f, err := t.openLogFile(currentSlug)
				if err != nil {
					t.logger.Warn("price tracker: failed to open log file", "error", err)
					continue
				}
				file = f
			}

			if file == nil {
				continue
			}

			refState, ok := t.refAnalytics.GetState(market.Asset)
			if !ok || refState.CurrentPrice <= 0 || market.PriceToBeat <= 0 {
				continue
			}

			quote, ok := t.registry.GetQuote(market.ID)
			if !ok {
				continue
			}

			remaining := time.Until(market.EndTime).Seconds()
			if remaining <= 0 {
				continue
			}

			fv, err := t.pricingModel.FairProbUp(ctx, domain.PricingInput{
				CurrentPrice:     refState.CurrentPrice,
				PriceToBeat:      market.PriceToBeat,
				RemainingSeconds: remaining,
				RealizedVol1m:    refState.RealizedVol1m,
				RealizedVol5m:    refState.RealizedVol5m,
				JumpScore:        refState.JumpScore,
				Regime:           refState.Regime,
			})
			if err != nil {
				continue
			}

			// Compute directional edges (same formula as SignalService)
			cost := 0.005 // approximate
			dirEdgeUp := fv.ProbUpLower - quote.Up.Ask - cost
			dirEdgeDown := (1.0 - fv.ProbUpUpper) - quote.Down.Ask - cost

			// Compute inventory and hedge edges
			var upQty, downQty, upCost, downCost float64
			var hedgeEdgeBuyUp, hedgeEdgeBuyDown, floor float64
			if t.positionSvc != nil {
				upQty, downQty, upCost, downCost = t.positionSvc.GetInventory(market.ID)
			}
			if t.hedgeEngine != nil {
				hedgeEdgeBuyUp, hedgeEdgeBuyDown, floor = t.hedgeEngine.ComputeHedgeEdges(ctx, market.ID, &quote)
			}

			// Determine action label
			action := "no_trade"
			if hedgeEdgeBuyDown > 0.002 {
				action = "hedge_buy_down"
			} else if hedgeEdgeBuyUp > 0.002 {
				action = "hedge_buy_up"
			} else if dirEdgeUp > 0.03 {
				action = "dir_buy_up"
			} else if dirEdgeDown > 0.03 {
				action = "dir_buy_down"
			}

			snap := FullTickSnapshot{
				Ts:               time.Now().Format(time.RFC3339),
				RefPrice:         refState.CurrentPrice,
				PriceToBeat:      market.PriceToBeat,
				RemainingMs:      remaining * 1000,
				SigmaTau:         fv.SigmaTau,
				Z:                fv.ZScore,
				PRaw:             fv.ProbRaw,
				PCal:             fv.ProbCalibrated,
				PLo:              fv.ProbUpLower,
				PHi:              fv.ProbUpUpper,
				UpBid:            quote.Up.Bid,
				UpAsk:            quote.Up.Ask,
				DownBid:          quote.Down.Bid,
				DownAsk:          quote.Down.Ask,
				DirEdgeUp:        dirEdgeUp,
				DirEdgeDown:      dirEdgeDown,
				UpQty:            upQty,
				DownQty:          downQty,
				UpCost:           upCost,
				DownCost:         downCost,
				GuaranteedFloor:  floor,
				HedgeEdgeBuyDown: hedgeEdgeBuyDown,
				HedgeEdgeBuyUp:   hedgeEdgeBuyUp,
				Action:           action,
			}

			line, _ := json.Marshal(snap)
			file.Write(line)
			file.Write([]byte("\n"))
		}
	}
}

func (t *PriceTracker) openLogFile(slug string) (*os.File, error) {
	if err := os.MkdirAll(t.logDir, 0o755); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("prices_%s.json", slug)
	return os.OpenFile(filepath.Join(t.logDir, name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}
