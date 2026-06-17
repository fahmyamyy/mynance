// Package wsfeed implements the FE-facing WebSocket pub/sub.
//
// The hub fans out events to connected clients filtered by topic. Topics
// are opaque strings (e.g. "orderbook.BTC-USDT", "kline.BTC-USDT.1m");
// publishers write to the hub via Publish(topic, payload), and clients
// opt in via {"op":"subscribe","topic":"..."} on their WS connection.
//
// Each topic maintains the subscriber set; clients only receive payloads
// for topics they've subscribed to. Publish is non-blocking — slow clients
// have their send buffer dropped rather than back-pressuring the engine.
package wsfeed

import (
	"sync"
)

// Hub is the central topic registry + dispatcher. Safe for concurrent use.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*Client]struct{})}
}

func (h *Hub) register(c *Client) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(c *Client) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// Publish dispatches an envelope to every client subscribed to topic.
// Returns the number of clients that received it. Drops the message for
// any client whose send buffer is full (slow consumer protection).
func (h *Hub) Publish(topic string, data any) int {
	env := envelope{Topic: topic, Data: data}
	h.mu.RLock()
	defer h.mu.RUnlock()
	sent := 0
	for c := range h.clients {
		if !c.subscribed(topic) {
			continue
		}
		if c.send(env) {
			sent++
		}
	}
	return sent
}

// envelope is the wire shape pushed to every subscriber.
type envelope struct {
	Topic string `json:"topic"`
	Data  any    `json:"data"`
}
