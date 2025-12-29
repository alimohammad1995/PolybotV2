package polymarket

import (
	"errors"
	"math/big"
)

const (
	SignatureEOA        uint8 = 0
	SignaturePolyProxy  uint8 = 1
	SignaturePolyGnosis uint8 = 2
)

var roundingConfig = map[string]RoundConfig{
	"0.1":    {Price: 1, Size: 2, Amount: 3},
	"0.01":   {Price: 2, Size: 2, Amount: 4},
	"0.001":  {Price: 3, Size: 2, Amount: 5},
	"0.0001": {Price: 4, Size: 2, Amount: 6},
}

type OrderBuilder struct {
	signer  *Signer
	sigType uint8
	funder  string
}

func NewOrderBuilder(signer *Signer, sigType uint8, funder string) *OrderBuilder {
	b := &OrderBuilder{signer: signer, sigType: sigType}
	if funder != "" {
		b.funder = funder
	} else if signer != nil {
		b.funder = signer.Address()
	}
	return b
}

func (b *OrderBuilder) getOrderAmounts(side OrderSide, size, price float64, roundConfig RoundConfig) (uint8, *big.Int, *big.Int, error) {
	rawPrice := RoundNormal(price, roundConfig.Price)

	switch side {
	case SideBuy:
		rawTakerAmt := RoundDown(size, roundConfig.Size)
		rawMakerAmt := rawTakerAmt * rawPrice
		if DecimalPlaces(rawMakerAmt) > roundConfig.Amount {
			rawMakerAmt = RoundUp(rawMakerAmt, roundConfig.Amount+4)
			if DecimalPlaces(rawMakerAmt) > roundConfig.Amount {
				rawMakerAmt = RoundDown(rawMakerAmt, roundConfig.Amount)
			}
		}
		makerAmount := big.NewInt(ToTokenDecimals(rawMakerAmt))
		takerAmount := big.NewInt(ToTokenDecimals(rawTakerAmt))
		return 0, makerAmount, takerAmount, nil
	case SideSell:
		rawMakerAmt := RoundDown(size, roundConfig.Size)
		rawTakerAmt := rawMakerAmt * rawPrice
		if DecimalPlaces(rawTakerAmt) > roundConfig.Amount {
			rawTakerAmt = RoundUp(rawTakerAmt, roundConfig.Amount+4)
			if DecimalPlaces(rawTakerAmt) > roundConfig.Amount {
				rawTakerAmt = RoundDown(rawTakerAmt, roundConfig.Amount)
			}
		}
		makerAmount := big.NewInt(ToTokenDecimals(rawMakerAmt))
		takerAmount := big.NewInt(ToTokenDecimals(rawTakerAmt))
		return 1, makerAmount, takerAmount, nil
	default:
		return 0, nil, nil, errors.New("order_args.side must be 'BUY' or 'SELL'")
	}
}

func (b *OrderBuilder) getMarketOrderAmounts(side OrderSide, amount, price float64, roundConfig RoundConfig) (uint8, *big.Int, *big.Int, error) {
	rawPrice := RoundNormal(price, roundConfig.Price)

	switch side {
	case SideBuy:
		rawMakerAmt := RoundDown(amount, roundConfig.Size)
		rawTakerAmt := rawMakerAmt / rawPrice
		if DecimalPlaces(rawTakerAmt) > roundConfig.Amount {
			rawTakerAmt = RoundUp(rawTakerAmt, roundConfig.Amount+4)
			if DecimalPlaces(rawTakerAmt) > roundConfig.Amount {
				rawTakerAmt = RoundDown(rawTakerAmt, roundConfig.Amount)
			}
		}
		makerAmount := big.NewInt(ToTokenDecimals(rawMakerAmt))
		takerAmount := big.NewInt(ToTokenDecimals(rawTakerAmt))
		return 0, makerAmount, takerAmount, nil
	case SideSell:
		rawMakerAmt := RoundDown(amount, roundConfig.Size)
		rawTakerAmt := rawMakerAmt * rawPrice
		if DecimalPlaces(rawTakerAmt) > roundConfig.Amount {
			rawTakerAmt = RoundUp(rawTakerAmt, roundConfig.Amount+4)
			if DecimalPlaces(rawTakerAmt) > roundConfig.Amount {
				rawTakerAmt = RoundDown(rawTakerAmt, roundConfig.Amount)
			}
		}
		makerAmount := big.NewInt(ToTokenDecimals(rawMakerAmt))
		takerAmount := big.NewInt(ToTokenDecimals(rawTakerAmt))
		return 1, makerAmount, takerAmount, nil
	default:
		return 0, nil, nil, errors.New("order_args.side must be 'BUY' or 'SELL'")
	}
}

func (b *OrderBuilder) CreateOrder(orderArgs *OrderArgs, options CreateOrderOptions) (*SignedOrder, error) {
	config, ok := roundingConfig[options.TickSize]
	if !ok {
		return nil, errors.New("invalid tick size")
	}
	side, makerAmount, takerAmount, err := b.getOrderAmounts(orderArgs.Side, orderArgs.Size, orderArgs.Price, config)
	if err != nil {
		return nil, err
	}
	order := &Order{
		Salt:          big.NewInt(generateSeed()),
		Maker:         b.funder,
		Signer:        b.signer.Address(),
		Taker:         normalizeAddress(orderArgs.Taker),
		TokenID:       nil,
		MakerAmount:   makerAmount,
		TakerAmount:   takerAmount,
		Expiration:    big.NewInt(orderArgs.Expiration),
		Nonce:         big.NewInt(orderArgs.Nonce),
		FeeRateBps:    big.NewInt(int64(orderArgs.FeeRateBps)),
		Side:          side,
		SignatureType: b.sigType,
	}
	if order.Taker == "" {
		order.Taker = ZeroAddress
	}

	contractCfg, err := GetContractConfig(b.signer.ChainID(), options.NegRisk)
	if err != nil {
		return nil, err
	}
	tokenID, err := parseBigInt(orderArgs.TokenID)
	if err != nil || tokenID == nil {
		return nil, errors.New("invalid token id")
	}
	order.TokenID = tokenID

	typed, err := BuildOrderTypedData(order, b.signer.ChainID(), contractCfg.Exchange)
	if err != nil {
		return nil, err
	}
	digest, err := hashTypedData(typed)
	if err != nil {
		return nil, err
	}
	signature, err := b.signer.SignHash(digest)
	if err != nil {
		return nil, err
	}
	return &SignedOrder{Order: order, Signature: signature}, nil
}

func (b *OrderBuilder) CreateMarketOrder(orderArgs MarketOrderArgs, options CreateOrderOptions) (SignedOrder, error) {
	config, ok := roundingConfig[options.TickSize]
	if !ok {
		return SignedOrder{}, errors.New("invalid tick size")
	}
	side, makerAmount, takerAmount, err := b.getMarketOrderAmounts(orderArgs.Side, orderArgs.Amount, orderArgs.Price, config)
	if err != nil {
		return SignedOrder{}, err
	}
	order := &Order{
		Salt:          big.NewInt(generateSeed()),
		Maker:         b.funder,
		Signer:        b.signer.Address(),
		Taker:         normalizeAddress(orderArgs.Taker),
		TokenID:       nil,
		MakerAmount:   makerAmount,
		TakerAmount:   takerAmount,
		Expiration:    big.NewInt(0),
		Nonce:         big.NewInt(orderArgs.Nonce),
		FeeRateBps:    big.NewInt(int64(orderArgs.FeeRateBps)),
		Side:          uint8(side),
		SignatureType: b.sigType,
	}
	if order.Taker == "" {
		order.Taker = ZeroAddress
	}

	contractCfg, err := GetContractConfig(b.signer.ChainID(), options.NegRisk)
	if err != nil {
		return SignedOrder{}, err
	}
	tokenID, err := parseBigInt(orderArgs.TokenID)
	if err != nil || tokenID == nil {
		return SignedOrder{}, errors.New("invalid token id")
	}
	order.TokenID = tokenID

	typed, err := BuildOrderTypedData(order, b.signer.ChainID(), contractCfg.Exchange)
	if err != nil {
		return SignedOrder{}, err
	}
	digest, err := hashTypedData(typed)
	if err != nil {
		return SignedOrder{}, err
	}
	signature, err := b.signer.SignHash(digest)
	if err != nil {
		return SignedOrder{}, err
	}
	return SignedOrder{Order: order, Signature: signature}, nil
}

func (b *OrderBuilder) CalculateBuyMarketPrice(positions []OrderSummary, amountToMatch float64, orderType OrderType) (float64, error) {
	if len(positions) == 0 {
		return 0, errors.New("no match")
	}
	sum := 0.0
	for i := len(positions) - 1; i >= 0; i-- {
		price, _ := strconvParseFloat(positions[i].Price)
		size, _ := strconvParseFloat(positions[i].Size)
		sum += size * price
		if sum >= amountToMatch {
			return price, nil
		}
	}
	if orderType == OrderTypeFOK {
		return 0, errors.New("no match")
	}
	price, _ := strconvParseFloat(positions[0].Price)
	return price, nil
}

func (b *OrderBuilder) CalculateSellMarketPrice(positions []OrderSummary, amountToMatch float64, orderType OrderType) (float64, error) {
	if len(positions) == 0 {
		return 0, errors.New("no match")
	}
	sum := 0.0
	for i := len(positions) - 1; i >= 0; i-- {
		size, _ := strconvParseFloat(positions[i].Size)
		sum += size
		if sum >= amountToMatch {
			price, _ := strconvParseFloat(positions[i].Price)
			return price, nil
		}
	}
	if orderType == OrderTypeFOK {
		return 0, errors.New("no match")
	}
	price, _ := strconvParseFloat(positions[0].Price)
	return price, nil
}
