package polymarket

import "errors"

var (
	ErrMissingPrivateKey = errors.New("polymarket: private key is required")
	ErrLevel1Auth        = errors.New(L1AuthUnavailable)
	ErrLevel2Auth        = errors.New(L2AuthUnavailable)
)
