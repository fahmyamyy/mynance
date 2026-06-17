package engine

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mynance/internal/eventbus"
)

func TestEngine_SubmitMatchEmitsTradeEvent(t *testing.T) {
	bus := eventbus.New()
	eng := New(bus, []string{"BTC-USDT"}, 16)

	var trades atomic.Int32
	done := make(chan struct{}, 1)
	bus.Subscribe(eventbus.EventTypeTradeMatched, func(e eventbus.Event) {
		trades.Add(1)
		done <- struct{}{}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	require.NoError(t, eng.Submit(Command{Kind: CommandPlaceOrder, Order: makeOrder("a1", SideSell, 30000, 1)}))
	require.NoError(t, eng.Submit(Command{Kind: CommandPlaceOrder, Order: makeOrder("b1", SideBuy, 30000, 1)}))

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("trade event not received")
	}
	require.Equal(t, int32(1), trades.Load())
}

func TestEngine_RestedOrderEmitsEvent(t *testing.T) {
	bus := eventbus.New()
	eng := New(bus, []string{"BTC-USDT"}, 16)

	rested := make(chan struct{}, 1)
	bus.Subscribe(eventbus.EventTypeOrderRested, func(e eventbus.Event) {
		rested <- struct{}{}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	require.NoError(t, eng.Submit(Command{Kind: CommandPlaceOrder, Order: makeOrder("a1", SideSell, 30000, 1)}))

	select {
	case <-rested:
	case <-time.After(time.Second):
		t.Fatal("rested event not received")
	}
}

func TestEngine_SubmitFullChannelReturnsError(t *testing.T) {
	bus := eventbus.New()
	eng := New(bus, []string{"BTC-USDT"}, 1)

	require.NoError(t, eng.Submit(Command{Kind: CommandPlaceOrder, Order: makeOrder("a1", SideSell, 30000, 1)}))
	err := eng.Submit(Command{Kind: CommandPlaceOrder, Order: makeOrder("a2", SideSell, 30000, 1)})
	require.ErrorIs(t, err, ErrSubmitChannelFull)
}

func TestEngine_UnknownSymbolRejected(t *testing.T) {
	bus := eventbus.New()
	eng := New(bus, []string{"BTC-USDT"}, 16)

	o := makeOrder("x1", SideBuy, 30000, 1)
	o.Symbol = "ETH-USDT" // not configured
	err := eng.Submit(Command{Kind: CommandPlaceOrder, Order: o})
	require.ErrorIs(t, err, ErrUnknownSymbol)
}

// TestEngine_DistinctSymbolsProgressIndependently asserts an ETH-USDT order
// matches via its own worker while a BTC-USDT backlog is still queued — and
// that running both shards concurrently is race-free under -race.
func TestEngine_DistinctSymbolsProgressIndependently(t *testing.T) {
	bus := eventbus.New()
	eng := New(bus, []string{"BTC-USDT", "ETH-USDT"}, 256)

	ethTrade := make(chan eventbus.TradeMatchedEvent, 1)
	bus.Subscribe(eventbus.EventTypeTradeMatched, func(e eventbus.Event) {
		evt := e.(eventbus.TradeMatchedEvent)
		if evt.Symbol == "ETH-USDT" {
			select {
			case ethTrade <- evt:
			default:
			}
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	// Queue a BTC-USDT backlog.
	for i := 0; i < 200; i++ {
		o := makeOrder("btc", SideSell, 30000, 1)
		_ = eng.Submit(Command{Kind: CommandPlaceOrder, Order: o})
	}

	// An ETH-USDT cross should match on its own worker without waiting for
	// the BTC backlog to drain.
	ethSell := &Order{ID: "eth-a1", UserID: "u1", Symbol: "ETH-USDT", Side: SideSell, Price: 2000, Quantity: 1, Remaining: 1}
	ethBuy := &Order{ID: "eth-b1", UserID: "u2", Symbol: "ETH-USDT", Side: SideBuy, Price: 2000, Quantity: 1, Remaining: 1}
	require.NoError(t, eng.Submit(Command{Kind: CommandPlaceOrder, Order: ethSell}))
	require.NoError(t, eng.Submit(Command{Kind: CommandPlaceOrder, Order: ethBuy}))

	select {
	case tr := <-ethTrade:
		require.Equal(t, "ETH-USDT", tr.Symbol)
		require.Equal(t, "eth-b1", tr.BuyOrderID)
	case <-time.After(time.Second):
		t.Fatal("ETH-USDT trade did not progress while BTC backlog pending")
	}
}

func TestEngine_LoadOrderRehydrates(t *testing.T) {
	bus := eventbus.New()
	eng := New(bus, []string{"BTC-USDT"}, 16)

	eng.LoadOrder(makeOrder("rest-a1", SideSell, 30000, 1))

	tradeCh := make(chan eventbus.TradeMatchedEvent, 1)
	bus.Subscribe(eventbus.EventTypeTradeMatched, func(e eventbus.Event) {
		tradeCh <- e.(eventbus.TradeMatchedEvent)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	require.NoError(t, eng.Submit(Command{Kind: CommandPlaceOrder, Order: makeOrder("b1", SideBuy, 30000, 1)}))

	select {
	case tr := <-tradeCh:
		require.Equal(t, "rest-a1", tr.SellOrderID)
		require.Equal(t, "b1", tr.BuyOrderID)
	case <-time.After(time.Second):
		t.Fatal("rehydration trade not received")
	}
}

func TestEngine_ConcurrentSubmits_NoRace(t *testing.T) {
	bus := eventbus.New()
	eng := New(bus, []string{"BTC-USDT"}, 2048)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go eng.Start(ctx)
	time.Sleep(10 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			side := SideBuy
			if i%2 == 0 {
				side = SideSell
			}
			_ = eng.Submit(Command{Kind: CommandPlaceOrder, Order: makeOrder("o", side, 30000, 1)})
		}(i)
	}
	wg.Wait()
}
