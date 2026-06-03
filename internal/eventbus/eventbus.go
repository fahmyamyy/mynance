package eventbus

import (
	"log/slog"
	"sync"
)

type Handler func(Event)

type Bus struct {
	mu       sync.RWMutex
	handlers map[EventType][]Handler
}

func New() *Bus {
	return &Bus{
		handlers: make(map[EventType][]Handler),
	}
}

func (b *Bus) Subscribe(t EventType, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[t] = append(b.handlers[t], h)
}

func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	hs := b.handlers[e.Type()]
	dispatch := make([]Handler, len(hs))
	copy(dispatch, hs)
	b.mu.RUnlock()

	for _, h := range dispatch {
		h := h
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("eventbus handler panic", "event", e.Type(), "panic", r)
				}
			}()
			h(e)
		}()
	}
}
