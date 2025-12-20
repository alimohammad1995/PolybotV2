package polymarket

import (
	"math/big"
	"math/rand"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func normalizeAddress(address string) string {
	if address == "" {
		return ""
	}
	return common.HexToAddress(address).Hex()
}

func generateSeed() int64 {
	rand.Seed(time.Now().UnixNano())
	return time.Now().Unix() + rand.Int63n(1_000_000)
}

func strconvParseFloat(value string) (float64, error) {
	return strconv.ParseFloat(value, 64)
}

func parseBigInt(value string) (*big.Int, error) {
	if value == "" {
		return nil, nil
	}
	n := new(big.Int)
	_, ok := n.SetString(value, 10)
	if !ok {
		return nil, strconv.ErrSyntax
	}
	return n, nil
}
