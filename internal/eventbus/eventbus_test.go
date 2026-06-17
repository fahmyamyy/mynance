package eventbus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublish_MultipleSubscribers_AllReceive(t *testing.T) {
	bus := New()
	var a, b atomic.Int32
	done := make(chan struct{}, 2)

	bus.Subscribe(EventTypeTradeMatched, func(Event) { a.Add(1); done <- struct{}{} })
	bus.Subscribe(EventTypeTradeMatched, func(Event) { b.Add(1); done <- struct{}{} })

	bus.Publish(TradeMatchedEvent{})

	for i := 0; i < 2; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for handler")
		}
	}
	require.Equal(t, int32(1), a.Load())
	require.Equal(t, int32(1), b.Load())
}

func TestPublish_NoSubscribers_NoError(t *testing.T) {
	bus := New()
	bus.Publish(TradeMatchedEvent{})
}

func TestPublish_PanickingHandler_DoesNotStopOthers(t *testing.T) {
	bus := New()
	var ok atomic.Int32
	done := make(chan struct{}, 1)

	bus.Subscribe(EventTypeTradeMatched, func(Event) { panic("boom") })
	bus.Subscribe(EventTypeTradeMatched, func(Event) { ok.Add(1); done <- struct{}{} })

	bus.Publish(TradeMatchedEvent{})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("second handler not called")
	}
	require.Equal(t, int32(1), ok.Load())
}

func TestPublish_NonBlocking(t *testing.T) {
	bus := New()
	bus.Subscribe(EventTypeTradeMatched, func(Event) { time.Sleep(200 * time.Millisecond) })

	start := time.Now()
	bus.Publish(TradeMatchedEvent{})
	elapsed := time.Since(start)

	require.Less(t, elapsed, 50*time.Millisecond)
}

func TestPublish_ConcurrentSubscribeAndPublish(t *testing.T) {
	bus := New()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			bus.Subscribe(EventTypeTradeMatched, func(Event) {})
		}()
		go func() {
			defer wg.Done()
			bus.Publish(TradeMatchedEvent{})
		}()
	}
	wg.Wait()
}
