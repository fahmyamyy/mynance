package engine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func makeOrder(id string, side Side, price, qty float64) *Order {
	return &Order{
		ID:        id,
		UserID:    "u-" + id,
		Symbol:    "BTC-USDT",
		Side:      side,
		Price:     price,
		Quantity:  qty,
		Remaining: qty,
	}
}

// TestMatch_OwnerlessOrderMatches_PreservesOwnership asserts a partner
// (ownerless) resting order matches a user order like any other and that the
// emitted trade keeps the user id on one side and the empty id on the other.
func TestMatch_OwnerlessOrderMatches_PreservesOwnership(t *testing.T) {
	ob := NewOrderBook("BTC-USDT")
	partnerAsk := &Order{ID: "p1", UserID: "", Symbol: "BTC-USDT", Side: SideSell, Price: 30000, Quantity: 1.0, Remaining: 1.0}
	ob.Match(partnerAsk)

	userBuy := &Order{ID: "u1", UserID: "alice", Symbol: "BTC-USDT", Side: SideBuy, Price: 30000, Quantity: 1.0, Remaining: 1.0}
	trades := ob.Match(userBuy)

	require.Len(t, trades, 1)
	require.Equal(t, "alice", trades[0].BuyUserID)
	require.Equal(t, "", trades[0].SellUserID)
	require.Len(t, ob.Asks, 0)
}

func TestMatch_BuyMatchesBestAskAtAskPrice(t *testing.T) {
	ob := NewOrderBook("BTC-USDT")
	ob.Match(makeOrder("a1", SideSell, 30000, 1.0))

	trades := ob.Match(makeOrder("b1", SideBuy, 31000, 1.0))
	require.Len(t, trades, 1)
	require.Equal(t, 30000.0, trades[0].Price)
	require.Equal(t, 1.0, trades[0].Quantity)
	require.Len(t, ob.Asks, 0)
	require.Len(t, ob.Bids, 0)
}

func TestMatch_SellMatchesBestBidAtBidPrice(t *testing.T) {
	ob := NewOrderBook("BTC-USDT")
	ob.Match(makeOrder("b1", SideBuy, 30000, 1.0))

	trades := ob.Match(makeOrder("a1", SideSell, 29000, 1.0))
	require.Len(t, trades, 1)
	require.Equal(t, 30000.0, trades[0].Price)
}

func TestMatch_PartialFill_RemainderRests(t *testing.T) {
	ob := NewOrderBook("BTC-USDT")
	ob.Match(makeOrder("a1", SideSell, 30000, 0.4))

	trades := ob.Match(makeOrder("b1", SideBuy, 30000, 1.0))
	require.Len(t, trades, 1)
	require.Equal(t, 0.4, trades[0].Quantity)
	require.Len(t, ob.Bids, 1)
	require.Equal(t, 0.6, ob.Bids[0].Orders[0].Remaining)
}

func TestMatch_NoCross_OrderRests(t *testing.T) {
	ob := NewOrderBook("BTC-USDT")
	ob.Match(makeOrder("a1", SideSell, 31000, 1.0))

	trades := ob.Match(makeOrder("b1", SideBuy, 30000, 1.0))
	require.Len(t, trades, 0)
	require.Len(t, ob.Bids, 1)
	require.Len(t, ob.Asks, 1)
}

func TestMatch_FIFOWithinPriceLevel(t *testing.T) {
	ob := NewOrderBook("BTC-USDT")
	ob.Match(makeOrder("a1", SideSell, 30000, 0.5))
	ob.Match(makeOrder("a2", SideSell, 30000, 0.5))

	trades := ob.Match(makeOrder("b1", SideBuy, 30000, 0.5))
	require.Len(t, trades, 1)
	require.Equal(t, "a1", trades[0].SellOrderID)
	require.Len(t, ob.Asks[0].Orders, 1)
	require.Equal(t, "a2", ob.Asks[0].Orders[0].ID)
}

func TestMatch_TwoTradesFromOneIncoming(t *testing.T) {
	ob := NewOrderBook("BTC-USDT")
	ob.Match(makeOrder("a1", SideSell, 30000, 0.5))
	ob.Match(makeOrder("a2", SideSell, 30100, 0.5))

	trades := ob.Match(makeOrder("b1", SideBuy, 31000, 1.0))
	require.Len(t, trades, 2)
	require.Equal(t, 30000.0, trades[0].Price)
	require.Equal(t, 30100.0, trades[1].Price)
}

func TestCancel_RemovesOrder_PreservesSiblings(t *testing.T) {
	ob := NewOrderBook("BTC-USDT")
	ob.Match(makeOrder("b1", SideBuy, 30000, 1.0))
	ob.Match(makeOrder("b2", SideBuy, 30000, 1.0))

	_, ok := ob.Cancel("b1")
	require.True(t, ok)
	require.Len(t, ob.Bids[0].Orders, 1)
	require.Equal(t, "b2", ob.Bids[0].Orders[0].ID)
}

func TestCancel_NonexistentReturnsFalse(t *testing.T) {
	ob := NewOrderBook("BTC-USDT")
	_, ok := ob.Cancel("nope")
	require.False(t, ok)
}

func TestAddBid_DescendingInsert(t *testing.T) {
	ob := NewOrderBook("BTC-USDT")
	ob.Match(makeOrder("b1", SideBuy, 30000, 1.0))
	ob.Match(makeOrder("b2", SideBuy, 31000, 1.0))
	ob.Match(makeOrder("b3", SideBuy, 30500, 1.0))

	require.Len(t, ob.Bids, 3)
	require.Equal(t, 31000.0, ob.Bids[0].Price)
	require.Equal(t, 30500.0, ob.Bids[1].Price)
	require.Equal(t, 30000.0, ob.Bids[2].Price)
}

func TestAddAsk_AscendingInsert(t *testing.T) {
	ob := NewOrderBook("BTC-USDT")
	ob.Match(makeOrder("a1", SideSell, 31000, 1.0))
	ob.Match(makeOrder("a2", SideSell, 30000, 1.0))
	ob.Match(makeOrder("a3", SideSell, 30500, 1.0))

	require.Len(t, ob.Asks, 3)
	require.Equal(t, 30000.0, ob.Asks[0].Price)
	require.Equal(t, 30500.0, ob.Asks[1].Price)
	require.Equal(t, 31000.0, ob.Asks[2].Price)
}
