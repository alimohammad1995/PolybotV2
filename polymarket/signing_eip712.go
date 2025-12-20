package polymarket

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

const (
	clobDomainName = "ClobAuthDomain"
	clobVersion    = "1"
	clobMessage    = "This message attests that I control the given wallet"
	orderDomain    = "Polymarket CTF Exchange"
	orderVersion   = "1"
)

func SignClobAuthMessage(signer *Signer, timestamp int64, nonce int64) (string, error) {
	message := apitypes.TypedDataMessage{
		"address":   signer.Address(),
		"timestamp": fmt.Sprintf("%d", timestamp),
		"nonce":     (*big.Int)(big.NewInt(nonce)),
		"message":   clobMessage,
	}

	typed := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
			},
			"ClobAuth": []apitypes.Type{
				{Name: "address", Type: "address"},
				{Name: "timestamp", Type: "string"},
				{Name: "nonce", Type: "uint256"},
				{Name: "message", Type: "string"},
			},
		},
		PrimaryType: "ClobAuth",
		Domain: apitypes.TypedDataDomain{
			Name:    clobDomainName,
			Version: clobVersion,
			ChainId: (*math.HexOrDecimal256)(big.NewInt(int64(signer.ChainID()))),
		},
		Message: message,
	}

	digest, err := hashTypedData(&typed)
	if err != nil {
		return "", err
	}
	return signer.SignHash(digest)
}

func BuildOrderTypedData(order Order, chainID int, exchange string) (*apitypes.TypedData, error) {
	message := apitypes.TypedDataMessage{
		"salt":          order.Salt,
		"maker":         order.Maker,
		"signer":        order.Signer,
		"taker":         order.Taker,
		"tokenId":       order.TokenID,
		"makerAmount":   order.MakerAmount,
		"takerAmount":   order.TakerAmount,
		"expiration":    order.Expiration,
		"nonce":         order.Nonce,
		"feeRateBps":    order.FeeRateBps,
		"side":          (*math.HexOrDecimal256)(big.NewInt(int64(order.Side))),
		"signatureType": (*math.HexOrDecimal256)(big.NewInt(int64(order.SignatureType))),
	}

	return &apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": []apitypes.Type{
				{Name: "name", Type: "string"},
				{Name: "version", Type: "string"},
				{Name: "chainId", Type: "uint256"},
				{Name: "verifyingContract", Type: "address"},
			},
			"Order": []apitypes.Type{
				{Name: "salt", Type: "uint256"},
				{Name: "maker", Type: "address"},
				{Name: "signer", Type: "address"},
				{Name: "taker", Type: "address"},
				{Name: "tokenId", Type: "uint256"},
				{Name: "makerAmount", Type: "uint256"},
				{Name: "takerAmount", Type: "uint256"},
				{Name: "expiration", Type: "uint256"},
				{Name: "nonce", Type: "uint256"},
				{Name: "feeRateBps", Type: "uint256"},
				{Name: "side", Type: "uint8"},
				{Name: "signatureType", Type: "uint8"},
			},
		},
		PrimaryType: "Order",
		Domain: apitypes.TypedDataDomain{
			Name:              orderDomain,
			Version:           orderVersion,
			ChainId:           (*math.HexOrDecimal256)(big.NewInt(int64(chainID))),
			VerifyingContract: common.HexToAddress(exchange).Hex(),
		},
		Message: message,
	}, nil
}

func hashTypedData(typed *apitypes.TypedData) ([]byte, error) {
	domainSeparator, err := typed.HashStruct("EIP712Domain", typed.Domain.Map())
	if err != nil {
		return nil, err
	}
	messageHash, err := typed.HashStruct(typed.PrimaryType, typed.Message)
	if err != nil {
		return nil, err
	}
	digest := crypto.Keccak256(
		[]byte{0x19, 0x01},
		[]byte(domainSeparator),
		[]byte(messageHash),
	)
	if len(digest) != 32 {
		return nil, fmt.Errorf("invalid digest length: %d", len(digest))
	}
	return digest, nil
}
