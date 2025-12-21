package main

import (
	"bytes"
	"encoding/json"
)

func decodeJSON(message []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(message))
	decoder.UseNumber()
	return decoder.Decode(out)
}
