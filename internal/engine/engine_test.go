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
	eng := New(bus, 16)

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
	eng := New(bus, 16)

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
	eng := New(bus, 1)

	require.NoError(t, eng.Submit(Command{Kind: CommandPlaceOrder, Order: makeOrder("a1", SideSell, 30000, 1)}))
	err := eng.Submit(Command{Kind: CommandPlaceOrder, Order: makeOrder("a2", SideSell, 30000, 1)})
	require.ErrorIs(t, err, ErrSubmitChannelFull)
}

func TestEngine_LoadOrderRehydrates(t *testing.T) {
	bus := eventbus.New()
	eng := New(bus, 16)

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
	eng := New(bus, 2048)

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
