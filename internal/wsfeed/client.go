package wsfeed

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait        = 10 * time.Second
	pongWait         = 60 * time.Second
	pingInterval     = (pongWait * 9) / 10
	maxMessageSize   = 1 << 14 // 16 KiB — subscribe ops only
	sendBufferLength = 256
)

// inboundOp is the only client-originated message shape.
type inboundOp struct {
	Op    string `json:"op"`              // "subscribe" | "unsubscribe"
	Topic string `json:"topic,omitempty"` // required for subscribe/unsubscribe
}

// Client owns a single WebSocket conn + its topic subscriptions + send queue.
type Client struct {
	hub  *Hub
	conn *websocket.Conn

	mu     sync.RWMutex
	topics map[string]struct{}

	out chan envelope
}

func newClient(hub *Hub, conn *websocket.Conn) *Client {
	return &Client{
		hub:    hub,
		conn:   conn,
		topics: make(map[string]struct{}),
		out:    make(chan envelope, sendBufferLength),
	}
}

func (c *Client) subscribed(topic string) bool {
	c.mu.RLock()
	_, ok := c.topics[topic]
	c.mu.RUnlock()
	return ok
}

func (c *Client) subscribe(topic string) {
	c.mu.Lock()
	c.topics[topic] = struct{}{}
	c.mu.Unlock()
}

func (c *Client) unsubscribe(topic string) {
	c.mu.Lock()
	delete(c.topics, topic)
	c.mu.Unlock()
}

// send queues an envelope for the writer goroutine. Returns false if the
// buffer is full (slow consumer); the caller logs and continues.
func (c *Client) send(env envelope) bool {
	select {
	case c.out <- env:
		return true
	default:
		return false
	}
}

// readLoop processes inbound subscribe/unsubscribe ops until the conn closes.
func (c *Client) readLoop() {
	defer c.close()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("wsfeed read", "err", err)
			}
			return
		}
		var op inboundOp
		if err := json.Unmarshal(msg, &op); err != nil {
			continue
		}
		switch op.Op {
		case "subscribe":
			if op.Topic != "" {
				c.subscribe(op.Topic)
			}
		case "unsubscribe":
			if op.Topic != "" {
				c.unsubscribe(op.Topic)
			}
		}
	}
}

// writeLoop drains the send queue + emits ping frames to keep the conn alive.
func (c *Client) writeLoop() {
	ticker := time.NewTicker(pingInterval)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case env, ok := <-c.out:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteJSON(env); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) close() {
	c.hub.unregister(c)
	// Closing `out` signals writeLoop to exit on next iteration.
	close(c.out)
}
