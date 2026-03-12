package polymarket

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const MarketChannel = "market"
const UserChannel = "user"

type WSMessageCallback func(message []byte)

type WebSocketOrderBook struct {
	channelType     string
	messageCallback WSMessageCallback
	conn            *websocket.Conn
}

func NewWebSocketOrderBook(channelType string, callback WSMessageCallback) *WebSocketOrderBook {
	return &WebSocketOrderBook{
		channelType:     channelType,
		messageCallback: callback,
	}
}

func (w *WebSocketOrderBook) RunAsync(send map[string]any) {
	go func() {
		if err := w.Run(send); err != nil {
			log.Printf("websocket error: %v", err)
		}
	}()
}

func (w *WebSocketOrderBook) Run(send map[string]any) error {
	conn, resp, err := websocket.DefaultDialer.Dial(PolyWSEndpoint+w.channelType, nil)

	if err != nil {
		return fmt.Errorf("websocket handshake failed: %s res:%v", err, resp)
	}

	w.conn = conn
	defer conn.Close()

	if len(send) > 0 {
		_ = w.sendJSON(send)
	}

	go w.pingLoop()

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return err
		}

		if w.messageCallback != nil {
			w.messageCallback(message)
		}
	}
}

func (w *WebSocketOrderBook) SubscribeToTokenIDs(assetIDs []string) error {
	if w.channelType != MarketChannel {
		return fmt.Errorf("unsupported channel type: %s", w.channelType)
	}
	if w.conn == nil {
		return fmt.Errorf("websocket not connected")
	}
	payload := map[string]any{
		"assets_ids": assetIDs,
		"operation":  "subscribe",
	}
	return w.sendJSON(payload)
}

func (w *WebSocketOrderBook) UnsubscribeToTokenIDs(assetIDs []string) error {
	if w.channelType != MarketChannel {
		return fmt.Errorf("unsupported channel type: %s", w.channelType)
	}
	if w.conn == nil {
		return fmt.Errorf("websocket not connected")
	}
	payload := map[string]any{
		"assets_ids": assetIDs,
		"operation":  "unsubscribe",
	}
	return w.sendJSON(payload)
}

func (w *WebSocketOrderBook) sendJSON(payload map[string]any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return w.conn.WriteMessage(websocket.TextMessage, data)
}

func (w *WebSocketOrderBook) pingLoop() {
	for {
		time.Sleep(10 * time.Second)
		if w.conn == nil {
			return
		}
		_ = w.conn.WriteMessage(websocket.TextMessage, []byte("PING"))
	}
}
