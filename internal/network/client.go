package network

import (
	"encoding/json"
	"fmt"

	"github.com/gorilla/websocket"
)

// Connect dials a WebSocket server and performs the auth handshake.
// Returns the connected Peer and the server's hostname, or an error.
func Connect(url, code string) (*Peer, string, error) {
	wsURL := fmt.Sprintf("ws://%s/ws", url)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("connection failed: %w", err)
	}

	// Send auth
	authData := AuthData{Code: code}
	authBytes, _ := EncodeMessage(MsgAuth, authData)
	if err := conn.WriteMessage(websocket.TextMessage, authBytes); err != nil {
		conn.Close()
		return nil, "", fmt.Errorf("auth send failed: %w", err)
	}

	// Wait for auth response
	_, raw, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, "", fmt.Errorf("auth response failed: %w", err)
	}

	msg, err := DecodeMessage(raw)
	if err != nil {
		conn.Close()
		return nil, "", fmt.Errorf("invalid auth response: %w", err)
	}

	if msg.Type == MsgAuthFail {
		conn.Close()
		return nil, "", fmt.Errorf("authentication failed: invalid code")
	}

	if msg.Type != MsgAuthOK {
		conn.Close()
		return nil, "", fmt.Errorf("unexpected response: %s", msg.Type)
	}

	var okData AuthOKData
	if err := json.Unmarshal(msg.Data, &okData); err != nil {
		conn.Close()
		return nil, "", fmt.Errorf("invalid auth_ok data: %w", err)
	}

	peer := NewPeer(conn, okData.Hostname)
	return peer, okData.Hostname, nil
}
