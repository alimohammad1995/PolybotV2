package polymarket

import "time"

const (
	PolyAddress    = "POLY_ADDRESS"
	PolySignature  = "POLY_SIGNATURE"
	PolyTimestamp  = "POLY_TIMESTAMP"
	PolyNonce      = "POLY_NONCE"
	PolyAPIKey     = "POLY_API_KEY"
	PolyPassphrase = "POLY_PASSPHRASE"
)

func CreateLevel1Headers(signer *Signer, nonce int64) (map[string]string, error) {
	timestamp := time.Now().Unix()
	signature, err := SignClobAuthMessage(signer, timestamp, nonce)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		PolyAddress:   signer.Address(),
		PolySignature: signature,
		PolyTimestamp: int64ToString(timestamp),
		PolyNonce:     int64ToString(nonce),
	}, nil
}

func CreateLevel2Headers(signer *Signer, creds ApiCreds, req RequestArgs) (map[string]string, error) {
	timestamp := time.Now().Unix()
	body := req.Body
	if req.SerializedBody != "" {
		body = req.SerializedBody
	}
	signature, err := BuildHMACSignature(creds.APISecret, timestamp, req.Method, req.RequestPath, body)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		PolyAddress:    signer.Address(),
		PolySignature:  signature,
		PolyTimestamp:  int64ToString(timestamp),
		PolyAPIKey:     creds.APIKey,
		PolyPassphrase: creds.APIPassphrase,
	}, nil
}
