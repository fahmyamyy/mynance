package wsfeed

import (
	"time"

	"mynance/internal/eventbus"
)

// EngineBridge wires matching-engine events to ws topics.
// The hub holds no business knowledge — this file is the seam.
type EngineBridge struct {
	hub *Hub
}

func NewEngineBridge(hub *Hub) *EngineBridge { return &EngineBridge{hub: hub} }

func (b *EngineBridge) Subscribe(bus *eventbus.Bus) {
	bus.Subscribe(eventbus.EventTypeTradeMatched, b.onTradeMatched)
	bus.Subscribe(eventbus.EventTypeOrderRested, b.onOrderRested)
	bus.Subscribe(eventbus.EventTypeOrderCancelled, b.onOrderCancelled)
}

// FE shapes — flat, snake_case, decimal-friendly numeric (floats here are
// already engine-side floats; FE displays them or stringifies as needed).
type tradePayload struct {
	Symbol    string    `json:"symbol"`
	Price     float64   `json:"price"`
	Quantity  float64   `json:"quantity"`
	Timestamp time.Time `json:"timestamp"`
}

type bookDeltaPayload struct {
	Symbol   string  `json:"symbol"`
	Side     string  `json:"side"`     // "BUY" | "SELL"
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"` // signed: + add liquidity, − remove
	Kind     string  `json:"kind"`     // "rest" | "cancel" | "trade"
}

func (b *EngineBridge) onTradeMatched(e eventbus.Event) {
	evt, ok := e.(eventbus.TradeMatchedEvent)
	if !ok {
		return
	}
	b.hub.Publish("trades."+evt.Symbol, tradePayload{
		Symbol:    evt.Symbol,
		Price:     evt.Price,
		Quantity:  evt.Quantity,
		Timestamp: evt.Timestamp,
	})
	// A trade consumes liquidity from both sides at evt.Price.
	b.hub.Publish("orderbook."+evt.Symbol, bookDeltaPayload{
		Symbol: evt.Symbol, Side: "BUY", Price: evt.Price, Quantity: -evt.Quantity, Kind: "trade",
	})
	b.hub.Publish("orderbook."+evt.Symbol, bookDeltaPayload{
		Symbol: evt.Symbol, Side: "SELL", Price: evt.Price, Quantity: -evt.Quantity, Kind: "trade",
	})
}

func (b *EngineBridge) onOrderRested(e eventbus.Event) {
	evt, ok := e.(eventbus.OrderRestedEvent)
	if !ok {
		return
	}
	b.hub.Publish("orderbook."+evt.Symbol, bookDeltaPayload{
		Symbol: evt.Symbol, Side: evt.Side, Price: evt.Price, Quantity: evt.Remaining, Kind: "rest",
	})
}

func (b *EngineBridge) onOrderCancelled(e eventbus.Event) {
	evt, ok := e.(eventbus.OrderCancelledEvent)
	if !ok || evt.Remaining <= 0 {
		return
	}
	b.hub.Publish("orderbook."+evt.Symbol, bookDeltaPayload{
		Symbol: evt.Symbol, Side: evt.Side, Price: evt.Price, Quantity: -evt.Remaining, Kind: "cancel",
	})
}
