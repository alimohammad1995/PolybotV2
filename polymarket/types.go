package polymarket

import "encoding/json"

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
