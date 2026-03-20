package network

import (
	"sync"

	"github.com/gorilla/websocket"
)

// Peer wraps a WebSocket connection with a read channel and mutex-protected writes.
type Peer struct {
	conn     *websocket.Conn
	InCh     chan Message
	Hostname string
	mu       sync.Mutex
	closed   bool
}

// NewPeer wraps an existing WebSocket connection and starts the read loop.
func NewPeer(conn *websocket.Conn, hostname string) *Peer {
	p := &Peer{
		conn:     conn,
		InCh:     make(chan Message, 32),
		Hostname: hostname,
	}
	go p.readLoop()
	return p
}

// readLoop reads messages from WebSocket and sends them to InCh.
// When the connection drops, it closes InCh to signal the consumer.
func (p *Peer) readLoop() {
	defer close(p.InCh)
	for {
		_, raw, err := p.conn.ReadMessage()
		if err != nil {
			return
		}
		msg, err := DecodeMessage(raw)
		if err != nil {
			continue
		}
		p.InCh <- *msg
	}
}

// Send sends a typed message to the peer.
func (p *Peer) Send(msgType string, data interface{}) error {
	raw, err := EncodeMessage(msgType, data)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	return p.conn.WriteMessage(websocket.TextMessage, raw)
}

// SendTree sends the local tree to the peer.
func (p *Peer) SendTree(tree *PeerTree) error {
	return p.Send(MsgTree, tree)
}

// Close shuts down the connection.
func (p *Peer) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	_ = p.conn.WriteMessage(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = p.conn.Close()
}
