package polymarket

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

func BuildHMACSignature(secret string, timestamp int64, method, requestPath string, body any) (string, error) {
	decoded, err := base64.URLEncoding.DecodeString(secret)
	if err != nil {
		return "", err
	}
	message := strings.Builder{}
	message.WriteString(int64ToString(timestamp))
	message.WriteString(method)
	message.WriteString(requestPath)
	if body != nil {
		message.WriteString(normalizeBodyForSignature(body))
	}
	h := hmac.New(sha256.New, decoded)
	_, _ = h.Write([]byte(message.String()))
	sig := base64.URLEncoding.EncodeToString(h.Sum(nil))
	return sig, nil
}

func normalizeBodyForSignature(body any) string {
	switch v := body.(type) {
	case string:
		return v
	default:
		return strings.ReplaceAll(stringifyBody(body), "'", "\"")
	}
}
