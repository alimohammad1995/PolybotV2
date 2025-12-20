package polymarket

import (
	"crypto/ecdsa"
	"encoding/hex"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

type Signer struct {
	privateKey *ecdsa.PrivateKey
	chainID    int
	address    common.Address
}

func NewSigner(privateKeyHex string, chainID int) (*Signer, error) {
	if privateKeyHex == "" {
		return nil, ErrMissingPrivateKey
	}
	trimmed := strings.TrimPrefix(privateKeyHex, "0x")
	keyBytes, err := hex.DecodeString(trimmed)
	if err != nil {
		return nil, err
	}
	privKey, err := crypto.ToECDSA(keyBytes)
	if err != nil {
		return nil, err
	}
	addr := crypto.PubkeyToAddress(privKey.PublicKey)
	return &Signer{privateKey: privKey, chainID: chainID, address: addr}, nil
}

func (s *Signer) Address() string {
	return s.address.Hex()
}

func (s *Signer) ChainID() int {
	return s.chainID
}

func (s *Signer) SignHash(hash []byte) (string, error) {
	sig, err := crypto.Sign(hash, s.privateKey)
	if err != nil {
		return "", err
	}
	return "0x" + hex.EncodeToString(sig), nil
}
