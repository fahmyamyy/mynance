package marketdata

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mynance/internal/eventbus"
)

func TestOnOrderRested_AddsToBook(t *testing.T) {
	s := NewService()
	s.OnOrderRested(eventbus.OrderRestedEvent{
		Symbol: "BTC-USDT", Side: "BUY", Price: 30000, Remaining: 1.0,
	})
	book := s.GetOrderBook("BTC-USDT")
	require.Len(t, book.Bids, 1)
	require.Equal(t, 30000.0, book.Bids[0].Price)
	require.Equal(t, 1.0, book.Bids[0].Quantity)
}

func TestOnTradeMatched_DeductsAndRecordsTrade(t *testing.T) {
	s := NewService()
	s.OnOrderRested(eventbus.OrderRestedEvent{
		Symbol: "BTC-USDT", Side: "SELL", Price: 30000, Remaining: 1.0,
	})
	s.OnTradeMatched(eventbus.TradeMatchedEvent{
		Symbol: "BTC-USDT", Price: 30000, Quantity: 0.4, Timestamp: time.Now(),
	})

	book := s.GetOrderBook("BTC-USDT")
	require.Len(t, book.Asks, 1)
	require.InDelta(t, 0.6, book.Asks[0].Quantity, 1e-9)

	trades := s.GetRecentTrades("BTC-USDT")
	require.Len(t, trades, 1)
	require.Equal(t, 0.4, trades[0].Quantity)
}

func TestRecentTrades_CappedAt100(t *testing.T) {
	s := NewService()
	for i := 0; i < 150; i++ {
		s.OnTradeMatched(eventbus.TradeMatchedEvent{
			Symbol: "BTC-USDT", Price: 30000, Quantity: 1, Timestamp: time.Now(),
		})
	}
	require.Len(t, s.GetRecentTrades("BTC-USDT"), 100)
}

func TestEmptyBook_ReturnsEmptyView(t *testing.T) {
	s := NewService()
	book := s.GetOrderBook("UNKNOWN")
	require.NotNil(t, book.Bids)
	require.NotNil(t, book.Asks)
	require.Len(t, book.Bids, 0)
}
