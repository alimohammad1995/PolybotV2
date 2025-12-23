package polymarket

import (
	"encoding/json"
	"fmt"
)

type OrderType string

type OrderSide string

const (
	OrderTypeGTC OrderType = "GTC"
	OrderTypeFOK OrderType = "FOK"
	OrderTypeGTD OrderType = "GTD"
	OrderTypeFAK OrderType = "FAK"

	SideBuy  OrderSide = "BUY"
	SideSell OrderSide = "SELL"
)

type ApiCreds struct {
	APIKey        string
	APISecret     string
	APIPassphrase string
}

type RequestArgs struct {
	Method         string
	RequestPath    string
	Body           any
	SerializedBody string
}

type OrderArgs struct {
	TokenID    string
	Price      float64
	Size       float64
	Side       OrderSide
	FeeRateBps int
	Nonce      int64
	Expiration int64
	Taker      string
}

type MarketOrderArgs struct {
	TokenID    string
	Amount     float64
	Side       OrderSide
	Price      float64
	FeeRateBps int
	Nonce      int64
	Taker      string
	OrderType  OrderType
}

type BookParams struct {
	TokenID string
	Side    string
}

type OrderSummary struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

type OrderBookSummary struct {
	Market       string         `json:"market"`
	AssetID      string         `json:"asset_id"`
	Timestamp    string         `json:"timestamp"`
	Bids         []OrderSummary `json:"bids"`
	Asks         []OrderSummary `json:"asks"`
	MinOrderSize string         `json:"min_order_size"`
	NegRisk      bool           `json:"neg_risk"`
	TickSize     string         `json:"tick_size"`
	Hash         string         `json:"hash"`
}

func (o *OrderBookSummary) ToJSON() string {
	msg, _ := json.Marshal(o)
	return string(msg)
}

type CreateOrderOptions struct {
	TickSize string
	NegRisk  bool
}

type PartialCreateOrderOptions struct {
	TickSize string
	NegRisk  *bool
}

type RoundConfig struct {
	Price  int
	Size   int
	Amount int
}

type ContractConfig struct {
	Exchange          string
	Collateral        string
	ConditionalTokens string
}

type SignedOrder struct {
	Order     Order
	Signature string
}

type stringOrNumber string

func (s stringOrNumber) String() string {
	return string(s)
}

func (s *stringOrNumber) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*s = ""
		return nil
	}
	if data[0] == '"' {
		var str string
		if err := json.Unmarshal(data, &str); err != nil {
			return err
		}
		*s = stringOrNumber(str)
		return nil
	}
	var num json.Number
	if err := json.Unmarshal(data, &num); err != nil {
		return fmt.Errorf("stringOrNumber: %w", err)
	}
	*s = stringOrNumber(num.String())
	return nil
}

type Trade struct {
	AssetID      string       `json:"asset_id"`
	Market       string       `json:"market"`
	Side         string       `json:"side"`
	Price        json.Number  `json:"price"`
	Size         json.Number  `json:"size"`
	Status       string       `json:"status"`
	TraderSide   string       `json:"trader_side"`
	MakerAddress string       `json:"maker_address"`
	MakerOrders  []MakerOrder `json:"maker_orders"`
}

type MakerOrder struct {
	MakerAddress  string      `json:"maker_address"`
	MatchedAmount json.Number `json:"matched_amount"`
	Price         json.Number `json:"price"`
	AssetID       string      `json:"asset_id"`
	Side          string      `json:"side"`
}

type TradesResponse struct {
	Data []Trade `json:"data"`
}

type ActiveOrder struct {
	ID           string      `json:"id"`
	Market       string      `json:"market"`
	AssetID      string      `json:"asset_id"`
	Price        json.Number `json:"price"`
	OriginalSize json.Number `json:"original_size"`
	SizeMatched  json.Number `json:"size_matched"`
}

type ActiveOrdersResponse struct {
	Data []ActiveOrder `json:"data"`
}
