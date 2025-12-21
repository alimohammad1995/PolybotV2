package main

import (
	"Polybot/polymarket"
	"os"
)

type PolymarketClient struct {
	client *polymarket.ClobClient
	creds  *polymarket.ApiCreds
}

func NewPolymarketClient() (*PolymarketClient, error) {
	client, err := polymarket.NewClobClient(
		os.Getenv("MAIN_ACCOUNT_PRIVATE_KEY"),
		polymarket.SignaturePolyGnosis,
		os.Getenv("MAIN_ACCOUNT_FUNDER_ADDRESS"),
	)

	if err != nil {
		return nil, err
	}

	cred, err := client.CreateOrDeriveAPICreds(0)
	if err != nil {
		return nil, err
	}
	client.SetAPICreds(cred)

	return &PolymarketClient{client, cred}, nil
}

func (receiver *PolymarketClient) GetClient() *polymarket.ClobClient {
	return receiver.client
}

func (receiver *PolymarketClient) GetCreds() map[string]string {
	return map[string]string{
		"apiKey":     receiver.creds.APIKey,
		"secret":     receiver.creds.APISecret,
		"passphrase": receiver.creds.APIPassphrase,
	}
}

func (receiver *PolymarketClient) Me() string {
	return os.Getenv("MAIN_ACCOUNT_FUNDER_ADDRESS")
}
