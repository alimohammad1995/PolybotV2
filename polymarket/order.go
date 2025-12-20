package polymarket

import (
	"math/big"
)

type Order struct {
	Salt          *big.Int
	Maker         string
	Signer        string
	Taker         string
	TokenID       *big.Int
	MakerAmount   *big.Int
	TakerAmount   *big.Int
	Expiration    *big.Int
	Nonce         *big.Int
	FeeRateBps    *big.Int
	Side          uint8
	SignatureType uint8
}

func (o Order) ToMap() map[string]any {
	return map[string]any{
		"salt":          o.Salt,
		"maker":         o.Maker,
		"signer":        o.Signer,
		"taker":         o.Taker,
		"tokenId":       o.TokenID,
		"makerAmount":   o.MakerAmount,
		"takerAmount":   o.TakerAmount,
		"expiration":    o.Expiration,
		"nonce":         o.Nonce,
		"feeRateBps":    o.FeeRateBps,
		"side":          o.Side,
		"signatureType": o.SignatureType,
	}
}

func (s SignedOrder) ToMap() map[string]any {
	m := s.Order.ToMap()
	m["signature"] = s.Signature
	if side, ok := m["side"].(uint8); ok {
		if side == 0 {
			m["side"] = "BUY"
		} else {
			m["side"] = "SELL"
		}
	}
	return m
}

func (s SignedOrder) ToJSONMap() map[string]any {
	m := s.ToMap()

	m["expiration"] = s.Order.Expiration.String()
	m["nonce"] = s.Order.Nonce.String()
	m["feeRateBps"] = s.Order.FeeRateBps.String()
	m["makerAmount"] = s.Order.MakerAmount.String()
	m["takerAmount"] = s.Order.TakerAmount.String()
	m["tokenId"] = s.Order.TokenID.String()

	return m
}
