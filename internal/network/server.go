package network

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ServerResult is returned when a client successfully connects.
type ServerResult struct {
	Peer     *Peer
	Hostname string
}

// StartServer starts an HTTP server with WebSocket upgrade on the given port.
// It accepts a single client, validates the auth code, and returns the peer.
// The caller receives the result on the returned channel.
func StartServer(port int, code string) (*http.Server, chan ServerResult, error) {
	resultCh := make(chan ServerResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		// Wait for auth message
		_, raw, err := conn.ReadMessage()
		if err != nil {
			conn.Close()
			return
		}
		msg, err := DecodeMessage(raw)
		if err != nil || msg.Type != MsgAuth {
			conn.Close()
			return
		}

		var authData AuthData
		if err := json.Unmarshal(msg.Data, &authData); err != nil || authData.Code != code {
			// Send auth_fail and close
			failBytes, _ := EncodeMessage(MsgAuthFail, nil)
			_ = conn.WriteMessage(websocket.TextMessage, failBytes)
			conn.Close()
			return
		}

		// Auth success — send auth_ok with our hostname
		hostname := getHostname()
		okData := AuthOKData{Hostname: hostname}
		okBytes, _ := EncodeMessage(MsgAuthOK, okData)
		_ = conn.WriteMessage(websocket.TextMessage, okBytes)

		clientHostname := authData.Hostname
		if clientHostname == "" {
			clientHostname = "peer"
		}
		peer := NewPeer(conn, clientHostname)
		resultCh <- ServerResult{Peer: peer, Hostname: hostname}
	})

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, nil, err
	}

	srv := &http.Server{Handler: mux}

	go func() {
		if err := srv.Serve(listener); err != nil && err != http.ErrServerClosed {
			// Server stopped
		}
	}()

	return srv, resultCh, nil
}

// GetLocalIPs returns non-loopback IPv4 addresses.
func GetLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() && ipNet.IP.To4() != nil {
			ips = append(ips, ipNet.IP.String())
		}
	}
	return ips
}

// ShutdownServer gracefully shuts down the HTTP server.
func ShutdownServer(srv *http.Server) {
	if srv != nil {
		_ = srv.Shutdown(context.Background())
	}
}

func getHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
