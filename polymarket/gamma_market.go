package polymarket

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type GammaMarketSummary struct {
	MarketID     string
	Slug         string
	Active       bool
	Closed       bool
	EndDateTS    int64
	StartDateTS  int64
	ClobTokenIDs []string
}

func (g *GammaMarketSummary) ToMap() map[string]any {
	return map[string]any{
		"conditionId":    g.MarketID,
		"slug":           g.Slug,
		"active":         g.Active,
		"closed":         g.Closed,
		"endDate":        g.EndDateTS,
		"eventStartTime": g.StartDateTS,
		"clobTokenIds":   g.ClobTokenIDs,
	}
}

func (g *GammaMarketSummary) ToEnd() int64 {
	return g.EndDateTS - time.Now().Unix()
}

func (g *GammaMarketSummary) ToStart() int64 {
	return g.StartDateTS - time.Now().Unix()
}

func GammaMarketSummaryFromDict(payload map[string]any) *GammaMarketSummary {
	return &GammaMarketSummary{
		MarketID:     fmt.Sprintf("%v", payload["conditionId"]),
		Slug:         fmt.Sprintf("%v", payload["slug"]),
		Active:       toBool(payload["active"]),
		Closed:       toBool(payload["closed"]),
		EndDateTS:    parseISOTimestamp(fmt.Sprintf("%v", payload["endDate"])),
		StartDateTS:  parseISOTimestamp(fmt.Sprintf("%v", payload["eventStartTime"])),
		ClobTokenIDs: parseClobTokenIDs(payload["clobTokenIds"]),
	}
}

type GammaMarket struct {
	cache      map[string]*GammaMarketSummary
	cacheOrder []string
	cacheSize  int
	httpClient *http.Client
}

func NewGammaMarket() *GammaMarket {
	return NewGammaMarketWithCacheSize(GammaMarketCacheSize)
}

func NewGammaMarketWithCacheSize(cacheSize int) *GammaMarket {
	if cacheSize <= 0 {
		cacheSize = GammaMarketCacheSize
	}
	return &GammaMarket{
		cache:      map[string]*GammaMarketSummary{},
		cacheOrder: []string{},
		cacheSize:  cacheSize,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (g *GammaMarket) GetMarketBySlug(slug string) (*GammaMarketSummary, error) {
	if cached, ok := g.cache[slug]; ok {
		return cached, nil
	}

	url := fmt.Sprintf("https://gamma-api.polymarket.com/markets/slug/%s", slug)
	resp, err := g.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gamma market http %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	market := GammaMarketSummaryFromDict(payload)

	if len(market.ClobTokenIDs) != 2 {
		return nil, fmt.Errorf("expected 2 tokens for %s; got %v", slug, market.ClobTokenIDs)
	}
	if market.EndDateTS <= 0 {
		return nil, fmt.Errorf("invalid end date for %s: %d", slug, market.EndDateTS)
	}
	if market.StartDateTS <= 0 {
		return nil, fmt.Errorf("invalid start date for %s: %d", slug, market.StartDateTS)
	}
	if !market.Active {
		return nil, fmt.Errorf("market %s is not active", slug)
	}
	if market.Slug != slug {
		return nil, fmt.Errorf("market slug mismatch: expected %s, got %s", slug, market.Slug)
	}
	if market.MarketID == "" || market.MarketID == "<nil>" {
		return nil, fmt.Errorf("market id is empty for %s", slug)
	}

	g.cache[slug] = market
	g.cacheOrder = append(g.cacheOrder, slug)
	g.trimCache()

	return market, nil
}

func (g *GammaMarket) trimCache() {
	for len(g.cacheOrder) > g.cacheSize {
		removing := g.cacheOrder[0]
		g.cacheOrder = g.cacheOrder[1:]
		delete(g.cache, removing)
	}
}

func parseISOTimestamp(value string) int64 {
	if value == "" || value == "<nil>" {
		return 0
	}
	normalized := strings.Replace(value, "Z", "+00:00", 1)
	parsed, err := time.Parse(time.RFC3339, normalized)
	if err != nil {
		return 0
	}
	return parsed.Unix()
}

func parseClobTokenIDs(raw any) []string {
	if raw == nil {
		return []string{}
	}
	switch v := raw.(type) {
	case []any:
		ids := make([]string, 0, len(v))
		for _, item := range v {
			ids = append(ids, fmt.Sprintf("%v", item))
		}
		return ids
	case []string:
		return append([]string{}, v...)
	case string:
		if v == "" {
			return []string{}
		}
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err != nil {
			return []string{}
		}
		if list, ok := parsed.([]any); ok {
			ids := make([]string, 0, len(list))
			for _, item := range list {
				ids = append(ids, fmt.Sprintf("%v", item))
			}
			return ids
		}
		return []string{}
	default:
		return []string{}
	}
}
