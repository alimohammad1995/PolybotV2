package polymarket

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type HTTPClient struct {
	client *http.Client
}

func NewHTTPClient(timeout time.Duration) *HTTPClient {
	return &HTTPClient{client: &http.Client{Timeout: timeout}}
}

func (h *HTTPClient) Request(method, url string, headers map[string]string, body any) (any, error) {
	reqBody, contentType, err := buildRequestBody(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}
	applyDefaultHeaders(req, method)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyReader := io.Reader(resp.Body)
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		bodyReader = gz
	}
	payload, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("polymarket: http %d: %s", resp.StatusCode, string(payload))
	}

	var out any
	if err := json.Unmarshal(payload, &out); err != nil {
		return string(payload), nil
	}
	return out, nil
}

func buildRequestBody(body any) (io.Reader, string, error) {
	if body == nil {
		return nil, "", nil
	}
	switch v := body.(type) {
	case string:
		return bytes.NewReader([]byte(v)), "application/json", nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, "", err
		}
		return bytes.NewReader(data), "application/json", nil
	}
}

func applyDefaultHeaders(req *http.Request, method string) {
	req.Header.Set("User-Agent", "py_clob_client")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Content-Type", "application/json")
	if method == http.MethodGet {
		req.Header.Set("Accept-Encoding", "gzip")
	}
}
