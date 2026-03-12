package polymarket

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type ClobClient struct {
	clobHost  string
	dataHost  string
	chainID   int
	signer    *Signer
	creds     *ApiCreds
	mode      int
	builder   *OrderBuilder
	http      *HTTPClient
	tickSizes map[string]string
	negRisk   map[string]bool
	feeRates  map[string]int
}

type orderSummaryWire struct {
	Price stringOrNumber `json:"price"`
	Size  stringOrNumber `json:"size"`
}

type orderBookSummaryWire struct {
	Market       stringOrNumber     `json:"market"`
	AssetID      stringOrNumber     `json:"asset_id"`
	Timestamp    stringOrNumber     `json:"timestamp"`
	Bids         []orderSummaryWire `json:"bids"`
	Asks         []orderSummaryWire `json:"asks"`
	MinOrderSize stringOrNumber     `json:"min_order_size"`
	NegRisk      bool               `json:"neg_risk"`
	TickSize     stringOrNumber     `json:"tick_size"`
	Hash         stringOrNumber     `json:"hash"`
}

func (o orderBookSummaryWire) toSummary() OrderBookSummary {
	bids := make([]OrderSummary, len(o.Bids))
	for i, bid := range o.Bids {
		bids[i] = OrderSummary{Price: bid.Price.String(), Size: bid.Size.String()}
	}
	asks := make([]OrderSummary, len(o.Asks))
	for i, ask := range o.Asks {
		asks[i] = OrderSummary{Price: ask.Price.String(), Size: ask.Size.String()}
	}
	return OrderBookSummary{
		Market:       o.Market.String(),
		AssetID:      o.AssetID.String(),
		Timestamp:    o.Timestamp.String(),
		Bids:         bids,
		Asks:         asks,
		MinOrderSize: o.MinOrderSize.String(),
		NegRisk:      o.NegRisk,
		TickSize:     o.TickSize.String(),
		Hash:         o.Hash.String(),
	}
}

func NewClobClient(key string, signatureType uint8, funder string) (*ClobClient, error) {
	var signer *Signer
	var err error
	if key != "" {
		signer, err = NewSigner(key, POLYGON)
		if err != nil {
			return nil, err
		}
	}

	client := &ClobClient{
		clobHost:  strings.TrimRight(CLOBEndpoint, "/"),
		dataHost:  strings.TrimRight(DataAPIEndpoint, "/"),
		chainID:   POLYGON,
		signer:    signer,
		mode:      L0,
		http:      NewHTTPClient(20 * time.Second),
		tickSizes: map[string]string{},
		negRisk:   map[string]bool{},
		feeRates:  map[string]int{},
	}

	if signer != nil {
		client.builder = NewOrderBuilder(signer, signatureType, funder)
		client.mode = L1
	}

	return client, nil
}

func (c *ClobClient) SetAPICreds(creds *ApiCreds) {
	c.creds = creds
	if c.signer != nil {
		c.mode = L2
	}
}

func (c *ClobClient) Address() string {
	if c.signer == nil {
		return ""
	}
	return c.signer.Address()
}

func (c *ClobClient) GetServerTime() (any, error) {
	return c.http.Request("GET", c.clobHost+TimeEndpoint, nil, nil)
}

func (c *ClobClient) CreateAPIKey(nonce int64) (*ApiCreds, error) {
	if err := c.assertLevel1(); err != nil {
		return nil, err
	}
	headers, err := CreateLevel1Headers(c.signer, nonce)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Request("POST", c.clobHost+CreateAPIKeyEndpoint, headers, nil)
	if err != nil {
		return nil, err
	}
	return parseCreds(resp)
}

func (c *ClobClient) DeriveAPIKey(nonce int64) (*ApiCreds, error) {
	if err := c.assertLevel1(); err != nil {
		return nil, err
	}
	headers, err := CreateLevel1Headers(c.signer, nonce)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Request("GET", c.clobHost+DeriveAPIKeyEndpoint, headers, nil)
	if err != nil {
		return nil, err
	}
	return parseCreds(resp)
}

func (c *ClobClient) CreateOrDeriveAPICreds(nonce int64) (*ApiCreds, error) {
	creds, err := c.CreateAPIKey(nonce)
	if err == nil {
		return creds, nil
	}
	return c.DeriveAPIKey(nonce)
}

func (c *ClobClient) GetMarket(conditionID string) (any, error) {
	return c.http.Request("GET", c.clobHost+GetMarketEndpoint+conditionID, nil, nil)
}

func (c *ClobClient) GetOrder(orderID string) (any, error) {
	if err := c.assertLevel2(); err != nil {
		return nil, err
	}
	reqPath := GetOrderEndpoint + orderID
	reqArgs := RequestArgs{Method: "GET", RequestPath: reqPath, Body: nil}
	headers, err := CreateLevel2Headers(c.signer, *c.creds, reqArgs)
	if err != nil {
		return nil, err
	}
	return c.http.Request("GET", c.clobHost+reqPath, headers, nil)
}

func (c *ClobClient) GetTradesTyped(params map[string]string) (*TradesResponse, error) {
	if err := c.assertLevel2(); err != nil {
		return nil, err
	}
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	reqArgs := RequestArgs{Method: "GET", RequestPath: TradesEndpoint, Body: nil}
	reqPath := TradesEndpoint + "?" + values.Encode()
	headers, err := CreateLevel2Headers(c.signer, *c.creds, reqArgs)
	if err != nil {
		return nil, err
	}
	var resp TradesResponse
	if err := c.http.RequestInto("GET", c.clobHost+reqPath, headers, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *ClobClient) GetActiveOrdersTyped(params map[string]string) (*ActiveOrdersResponse, error) {
	if err := c.assertLevel2(); err != nil {
		return nil, err
	}
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	reqArgs := RequestArgs{Method: "GET", RequestPath: OrdersEndpoint, Body: nil}
	reqPath := OrdersEndpoint + "?" + values.Encode()
	headers, err := CreateLevel2Headers(c.signer, *c.creds, reqArgs)
	if err != nil {
		return nil, err
	}
	var resp ActiveOrdersResponse
	if err := c.http.RequestInto("GET", c.clobHost+reqPath, headers, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *ClobClient) GetPositions(user string, params map[string]string) (any, error) {
	values := url.Values{}
	values.Set("user", user)
	for key, value := range params {
		values.Set(key, value)
	}
	query := values.Encode()
	reqPath := GetPositionsEndpoint
	if query != "" {
		reqPath += "?" + query
	}
	return c.http.Request("GET", c.dataHost+reqPath, nil, nil)
}

func (c *ClobClient) GetMarkets(nextCursor string) (any, error) {
	if nextCursor == "" {
		nextCursor = "MA=="
	}
	return c.http.Request("GET", fmt.Sprintf("%s%s?next_cursor=%s", c.clobHost, GetMarketsEndpoint, nextCursor), nil, nil)
}

func (c *ClobClient) GetOrderBook(tokenID string) (OrderBookSummary, error) {
	var book orderBookSummaryWire
	if err := c.http.RequestInto("GET", fmt.Sprintf("%s%s?token_id=%s", c.clobHost, GetOrderBookEndpoint, tokenID), nil, nil, &book); err != nil {
		return OrderBookSummary{}, err
	}
	return book.toSummary(), nil
}

func (c *ClobClient) GetOrderBooks(tokenIDs []string) (map[string]OrderBookSummary, error) {
	body := make([]map[string]string, 0, len(tokenIDs))
	for _, tokenID := range tokenIDs {
		body = append(body, map[string]string{"token_id": tokenID})
	}
	var rawList []orderBookSummaryWire
	if err := c.http.RequestInto("POST", c.clobHost+GetOrderBooksEndpoint, nil, body, &rawList); err != nil {
		return nil, err
	}
	books := make(map[string]OrderBookSummary, len(rawList))
	for _, book := range rawList {
		summary := book.toSummary()
		books[summary.AssetID] = summary
	}
	return books, nil
}

func (c *ClobClient) GetTickSize(tokenID string) (string, error) {
	if val, ok := c.tickSizes[tokenID]; ok {
		return val, nil
	}
	resp, err := c.http.Request("GET", fmt.Sprintf("%s%s?token_id=%s", c.clobHost, GetTickSizeEndpoint, tokenID), nil, nil)
	if err != nil {
		return "", err
	}
	result, ok := resp.(map[string]any)
	if !ok {
		return "", errors.New("invalid tick size response")
	}
	value := stringFromAny(result["minimum_tick_size"])
	c.tickSizes[tokenID] = value
	return value, nil
}

func (c *ClobClient) GetNegRisk(tokenID string) (bool, error) {
	if val, ok := c.negRisk[tokenID]; ok {
		return val, nil
	}
	resp, err := c.http.Request("GET", fmt.Sprintf("%s%s?token_id=%s", c.clobHost, GetNegRiskEndpoint, tokenID), nil, nil)
	if err != nil {
		return false, err
	}
	result, ok := resp.(map[string]any)
	if !ok {
		return false, errors.New("invalid neg risk response")
	}
	value := toBool(result["neg_risk"])
	c.negRisk[tokenID] = value
	return value, nil
}

func (c *ClobClient) GetFeeRateBps(tokenID string) (int, error) {
	if val, ok := c.feeRates[tokenID]; ok {
		return val, nil
	}
	resp, err := c.http.Request("GET", fmt.Sprintf("%s%s?token_id=%s", c.clobHost, GetFeeRateEndpoint, tokenID), nil, nil)
	if err != nil {
		return 0, err
	}
	result, ok := resp.(map[string]any)
	if !ok {
		return 0, errors.New("invalid fee rate response")
	}
	baseFee := 0
	if val, ok := result["base_fee"]; ok {
		switch v := val.(type) {
		case float64:
			baseFee = int(v)
		case int:
			baseFee = v
		case json.Number:
			parsed, _ := v.Int64()
			baseFee = int(parsed)
		}
	}
	c.feeRates[tokenID] = baseFee
	return baseFee, nil
}

func (c *ClobClient) CreateOrder(orderArgs *OrderArgs, options *PartialCreateOrderOptions) (*SignedOrder, error) {
	if err := c.assertLevel1(); err != nil {
		return nil, err
	}
	if orderArgs.Taker == "" {
		orderArgs.Taker = ZeroAddress
	}

	tickSize, err := c.resolveTickSize(orderArgs.TokenID, options)
	if err != nil {
		return nil, err
	}
	if !PriceValid(orderArgs.Price, tickSize) {
		return nil, fmt.Errorf("price (%f), min: %s - max: %f", orderArgs.Price, tickSize, 1-parseFloatDefault(tickSize))
	}

	negRisk, err := c.resolveNegRisk(orderArgs.TokenID, options)
	if err != nil {
		return nil, err
	}

	feeRate, err := c.resolveFeeRate(orderArgs.TokenID, orderArgs.FeeRateBps)
	if err != nil {
		return nil, err
	}
	orderArgs.FeeRateBps = feeRate

	return c.builder.CreateOrder(orderArgs, CreateOrderOptions{TickSize: tickSize, NegRisk: negRisk})
}

func (c *ClobClient) CreateMarketOrder(orderArgs MarketOrderArgs, options *PartialCreateOrderOptions) (SignedOrder, error) {
	if err := c.assertLevel1(); err != nil {
		return SignedOrder{}, err
	}
	if orderArgs.Taker == "" {
		orderArgs.Taker = ZeroAddress
	}
	if orderArgs.OrderType == "" {
		orderArgs.OrderType = OrderTypeFOK
	}

	tickSize, err := c.resolveTickSize(orderArgs.TokenID, options)
	if err != nil {
		return SignedOrder{}, err
	}

	if orderArgs.Price <= 0 {
		orderArgs.Price, err = c.CalculateMarketPrice(orderArgs.TokenID, orderArgs.Side, orderArgs.Amount, orderArgs.OrderType)
		if err != nil {
			return SignedOrder{}, err
		}
	}
	if !PriceValid(orderArgs.Price, tickSize) {
		return SignedOrder{}, fmt.Errorf("price (%f), min: %s - max: %f", orderArgs.Price, tickSize, 1-parseFloatDefault(tickSize))
	}

	negRisk, err := c.resolveNegRisk(orderArgs.TokenID, options)
	if err != nil {
		return SignedOrder{}, err
	}

	feeRate, err := c.resolveFeeRate(orderArgs.TokenID, orderArgs.FeeRateBps)
	if err != nil {
		return SignedOrder{}, err
	}
	orderArgs.FeeRateBps = feeRate

	return c.builder.CreateMarketOrder(orderArgs, CreateOrderOptions{TickSize: tickSize, NegRisk: negRisk})
}

func (c *ClobClient) PostOrder(order *SignedOrder, orderType OrderType) (any, error) {
	if err := c.assertLevel2(); err != nil {
		return nil, err
	}
	body := map[string]any{
		"order":     order.ToJSONMap(),
		"owner":     c.creds.APIKey,
		"orderType": orderType,
	}
	serialized, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	reqArgs := RequestArgs{Method: "POST", RequestPath: PostOrderEndpoint, Body: body, SerializedBody: string(serialized)}
	headers, err := CreateLevel2Headers(c.signer, *c.creds, reqArgs)
	if err != nil {
		return nil, err
	}
	return c.http.Request("POST", c.clobHost+PostOrderEndpoint, headers, reqArgs.SerializedBody)
}

func (c *ClobClient) PostOrders(args []PostOrdersArgs) (any, error) {
	if err := c.assertLevel2(); err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, nil
	}
	body := make([]map[string]any, 0, len(args))
	for _, arg := range args {
		body = append(body, map[string]any{
			"order":     arg.Order.ToJSONMap(),
			"owner":     c.creds.APIKey,
			"orderType": arg.OrderType,
		})
	}
	serialized, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	reqArgs := RequestArgs{Method: "POST", RequestPath: PostOrdersEndpoint, Body: body, SerializedBody: string(serialized)}
	headers, err := CreateLevel2Headers(c.signer, *c.creds, reqArgs)
	if err != nil {
		return nil, err
	}
	return c.http.Request("POST", c.clobHost+PostOrdersEndpoint, headers, reqArgs.SerializedBody)
}

func (c *ClobClient) CancelAllOrders() (any, error) {
	if err := c.assertLevel2(); err != nil {
		return nil, err
	}
	reqArgs := RequestArgs{Method: "DELETE", RequestPath: CancelAllEndpoint, Body: nil}
	headers, err := CreateLevel2Headers(c.signer, *c.creds, reqArgs)
	if err != nil {
		return nil, err
	}
	return c.http.Request("DELETE", c.clobHost+CancelAllEndpoint, headers, nil)
}

func (c *ClobClient) CancelOrders(orderIDs []string) (any, error) {
	if err := c.assertLevel2(); err != nil {
		return nil, err
	}
	if len(orderIDs) == 0 {
		return nil, nil
	}
	body := orderIDs
	serialized, _ := json.Marshal(orderIDs)
	reqArgs := RequestArgs{Method: "DELETE", RequestPath: CancelOrdersEndpoint, Body: body, SerializedBody: string(serialized)}
	headers, err := CreateLevel2Headers(c.signer, *c.creds, reqArgs)
	if err != nil {
		return nil, err
	}
	return c.http.Request("DELETE", c.clobHost+CancelOrdersEndpoint, headers, reqArgs.SerializedBody)
}

func (c *ClobClient) CancelMarketOrders(tokenID string) (any, error) {
	if err := c.assertLevel2(); err != nil {
		return nil, err
	}
	if tokenID == "" {
		return nil, errors.New("token id is required")
	}
	body := map[string]any{"token_id": tokenID}
	serialized, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	reqArgs := RequestArgs{Method: "POST", RequestPath: CancelMarketOrdersEndpoint, Body: body, SerializedBody: string(serialized)}
	headers, err := CreateLevel2Headers(c.signer, *c.creds, reqArgs)
	if err != nil {
		return nil, err
	}
	return c.http.Request("POST", c.clobHost+CancelMarketOrdersEndpoint, headers, reqArgs.SerializedBody)
}

func (c *ClobClient) CalculateMarketPrice(tokenID string, side OrderSide, amount float64, orderType OrderType) (float64, error) {
	book, err := c.GetOrderBook(tokenID)
	if err != nil {
		return 0, err
	}
	if side == SideBuy {
		if len(book.Asks) == 0 {
			return 0, errors.New("no match")
		}
		return c.builder.CalculateBuyMarketPrice(book.Asks, amount, orderType)
	}
	if len(book.Bids) == 0 {
		return 0, errors.New("no match")
	}
	return c.builder.CalculateSellMarketPrice(book.Bids, amount, orderType)
}

func (c *ClobClient) assertLevel1() error {
	if c.mode < L1 || c.signer == nil {
		return ErrLevel1Auth
	}
	return nil
}

func (c *ClobClient) assertLevel2() error {
	if c.mode < L2 || c.signer == nil || c.creds == nil {
		return ErrLevel2Auth
	}
	return nil
}

func (c *ClobClient) resolveTickSize(tokenID string, options *PartialCreateOrderOptions) (string, error) {
	minTickSize, err := c.GetTickSize(tokenID)
	if err != nil {
		return "", err
	}
	if options != nil && options.TickSize != "" {
		if IsTickSizeSmaller(options.TickSize, minTickSize) {
			return "", fmt.Errorf("invalid tick size (%s), minimum for the market is %s", options.TickSize, minTickSize)
		}
		return options.TickSize, nil
	}
	return minTickSize, nil
}

func (c *ClobClient) resolveNegRisk(tokenID string, options *PartialCreateOrderOptions) (bool, error) {
	if options != nil && options.NegRisk != nil {
		return *options.NegRisk, nil
	}
	return c.GetNegRisk(tokenID)
}

func (c *ClobClient) resolveFeeRate(tokenID string, userFeeRate int) (int, error) {
	feeRate, err := c.GetFeeRateBps(tokenID)
	if err != nil {
		return 0, err
	}
	if feeRate > 0 && userFeeRate > 0 && userFeeRate != feeRate {
		return 0, fmt.Errorf("invalid user provided fee rate: (%d), fee rate for the market must be %d", userFeeRate, feeRate)
	}
	return feeRate, nil
}

func parseCreds(resp any) (*ApiCreds, error) {
	data, ok := resp.(map[string]any)
	if !ok {
		return nil, errors.New("couldn't parse created CLOB creds")
	}
	apiKey := stringFromAny(data["apiKey"])
	secret := stringFromAny(data["secret"])
	passphrase := stringFromAny(data["passphrase"])
	if apiKey == "" || secret == "" || passphrase == "" {
		return nil, errors.New("couldn't parse created CLOB creds")
	}
	return &ApiCreds{APIKey: apiKey, APISecret: secret, APIPassphrase: passphrase}, nil
}

func parseFloatDefault(value string) float64 {
	parsed, _ := strconvParseFloat(value)
	return parsed
}
