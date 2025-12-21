package polymarket

const (
	L0 = 0
	L1 = 1
	L2 = 2

	ZeroAddress = "0x0000000000000000000000000000000000000000"

	AMOY    = 80002
	POLYGON = 137
)

const (
	CLOBEndpoint    = "https://clob.polymarket.com"
	PolyWSEndpoint  = "wss://ws-subscriptions-clob.polymarket.com/ws/"
	DataAPIEndpoint = "https://data-api.polymarket.com"
)

const (
	TimeEndpoint                   = "/time"
	CreateAPIKeyEndpoint           = "/auth/api-key"
	GetAPIKeysEndpoint             = "/auth/api-keys"
	DeleteAPIKeyEndpoint           = "/auth/api-key"
	DeriveAPIKeyEndpoint           = "/auth/derive-api-key"
	ClosedOnlyEndpoint             = "/auth/ban-status/closed-only"
	CreateReadonlyAPIKeyEndpoint   = "/auth/readonly-api-key"
	GetReadonlyAPIKeysEndpoint     = "/auth/readonly-api-keys"
	DeleteReadonlyAPIKeyEndpoint   = "/auth/readonly-api-key"
	ValidateReadonlyAPIKeyEndpoint = "/auth/validate-readonly-api-key"
	TradesEndpoint                 = "/data/trades"
	GetOrderBookEndpoint           = "/book"
	GetOrderBooksEndpoint          = "/books"
	GetOrderEndpoint               = "/data/order/"
	OrdersEndpoint                 = "/data/orders"
	PostOrderEndpoint              = "/order"
	PostOrdersEndpoint             = "/orders"
	CancelEndpoint                 = "/order"
	CancelOrdersEndpoint           = "/orders"
	CancelAllEndpoint              = "/cancel-all"
	CancelMarketOrdersEndpoint     = "/cancel-market-orders"
	MidPointEndpoint               = "/midpoint"
	MidPointsEndpoint              = "/midpoints"
	PriceEndpoint                  = "/price"
	GetPricesEndpoint              = "/prices"
	GetSpreadEndpoint              = "/spread"
	GetSpreadsEndpoint             = "/spreads"
	GetLastTradePriceEndpoint      = "/last-trade-price"
	GetLastTradesPricesEndpoint    = "/last-trades-prices"
	GetNotificationsEndpoint       = "/notifications"
	DropNotificationsEndpoint      = "/notifications"
	GetBalanceAllowanceEndpoint    = "/balance-allowance"
	UpdateBalanceAllowanceEndpoint = "/balance-allowance/update"
	IsOrderScoringEndpoint         = "/order-scoring"
	AreOrdersScoringEndpoint       = "/orders-scoring"
	GetTickSizeEndpoint            = "/tick-size"
	GetNegRiskEndpoint             = "/neg-risk"
	GetFeeRateEndpoint             = "/fee-rate"
	GetSamplingSimplifiedMarkets   = "/sampling-simplified-markets"
	GetSamplingMarkets             = "/sampling-markets"
	GetSimplifiedMarketsEndpoint   = "/simplified-markets"
	GetMarketsEndpoint             = "/markets"
	GetMarketEndpoint              = "/markets/"
	GetMarketTradesEventsEndpoint  = "/live-activity/events/"
	GetBuilderTradesEndpoint       = "/builder/trades"
	CreateRFQRequestEndpoint       = "/rfq/request"
	CancelRFQRequestEndpoint       = "/rfq/request"
	GetRFQRequestsEndpoint         = "/rfq/data/requests"
	CreateRFQQuoteEndpoint         = "/rfq/quote"
	CancelRFQQuoteEndpoint         = "/rfq/quote"
	GetRFQQuotesEndpoint           = "/rfq/data/quotes"
	GetRFQBestQuoteEndpoint        = "/rfq/data/best-quote"
	RFQRequestsAcceptEndpoint      = "/rfq/request/accept"
	RFQQuoteApproveEndpoint        = "/rfq/quote/approve"
	RFQConfigEndpoint              = "/rfq/config"
)

const (
	GetPositionsEndpoint = "/positions"
)

const (
	L1AuthUnavailable = "A private key is needed to interact with this endpoint!"
	L2AuthUnavailable = "API Credentials are needed to interact with this endpoint!"
)

const GammaMarketCacheSize = 1024
