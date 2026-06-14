package simbot

import (
	"fmt"
	"sync"
)

// levelKey converts a float price to a stable string key. 8 decimals covers
// all assets traded on Binance spot.
func levelKey(price float64) string {
	return fmt.Sprintf("%.8f", price)
}

// restingOrder tracks one ownerless partner order placed by the ingester at a
// price level.
type restingOrder struct {
	orderID string
	qty     float64
}

// bookState tracks placed partner orders per (symbol, side, priceKey).
// Used to diff against the latest partner snapshot and decide which orders
// to cancel and which prices to add.
type bookState struct {
	mu    sync.Mutex
	bids  map[string]map[string]restingOrder // symbol → priceKey → resting
	asks  map[string]map[string]restingOrder
}

func newBookState() *bookState {
	return &bookState{
		bids: make(map[string]map[string]restingOrder),
		asks: make(map[string]map[string]restingOrder),
	}
}

func (s *bookState) sideMap(symbol, side string) map[string]restingOrder {
	if side == "BUY" {
		m, ok := s.bids[symbol]
		if !ok {
			m = make(map[string]restingOrder)
			s.bids[symbol] = m
		}
		return m
	}
	m, ok := s.asks[symbol]
	if !ok {
		m = make(map[string]restingOrder)
		s.asks[symbol] = m
	}
	return m
}
