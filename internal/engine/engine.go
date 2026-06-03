package engine

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"mynance/internal/eventbus"
)

var ErrSubmitChannelFull = errors.New("engine submit channel full")

type Command struct {
	Kind   CommandKind
	Order  *Order
	Cancel *CancelInfo
}

type CommandKind int

const (
	CommandPlaceOrder CommandKind = iota
	CommandCancelOrder
)

type CancelInfo struct {
	OrderID string
	Symbol  string
}

type Engine struct {
	mu     sync.Mutex
	books  map[string]*OrderBook
	bus    *eventbus.Bus
	cmdCh  chan Command
	seq    int
	loaded bool
}

func New(bus *eventbus.Bus, bufferSize int) *Engine {
	if bufferSize <= 0 {
		bufferSize = 1024
	}
	return &Engine{
		books: make(map[string]*OrderBook),
		bus:   bus,
		cmdCh: make(chan Command, bufferSize),
	}
}

func (e *Engine) book(symbol string) *OrderBook {
	if b, ok := e.books[symbol]; ok {
		return b
	}
	b := NewOrderBook(symbol)
	e.books[symbol] = b
	return b
}

// LoadOrder inserts a resting order without matching. Used for rehydration before Start.
func (e *Engine) LoadOrder(o *Order) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.loaded {
		slog.Warn("engine.LoadOrder called after Start; ignoring", "id", o.ID)
		return
	}
	b := e.book(o.Symbol)
	if o.Side == SideBuy {
		b.addBid(o)
	} else {
		b.addAsk(o)
	}
}

func (e *Engine) Submit(cmd Command) error {
	select {
	case e.cmdCh <- cmd:
		return nil
	default:
		return ErrSubmitChannelFull
	}
}

func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	e.loaded = true
	e.mu.Unlock()

	slog.Info("engine started")
	for {
		select {
		case <-ctx.Done():
			slog.Info("engine stopped")
			return
		case cmd := <-e.cmdCh:
			e.handle(cmd)
		}
	}
}

func (e *Engine) handle(cmd Command) {
	switch cmd.Kind {
	case CommandPlaceOrder:
		e.handlePlace(cmd.Order)
	case CommandCancelOrder:
		e.handleCancel(cmd.Cancel)
	}
}

func (e *Engine) handlePlace(o *Order) {
	if o == nil {
		return
	}
	b := e.book(o.Symbol)
	trades := b.Match(o)
	for i := range trades {
		e.seq++
		t := trades[i]
		e.bus.Publish(eventbus.TradeMatchedEvent{
			Seq:         e.seq,
			Symbol:      t.Symbol,
			BuyOrderID:  t.BuyOrderID,
			SellOrderID: t.SellOrderID,
			BuyUserID:   t.BuyUserID,
			SellUserID:  t.SellUserID,
			Price:       t.Price,
			Quantity:    t.Quantity,
			Timestamp:   t.Timestamp,
		})
	}
	if o.Remaining > 0 {
		e.bus.Publish(eventbus.OrderRestedEvent{
			OrderID:   o.ID,
			UserID:    o.UserID,
			Symbol:    o.Symbol,
			Side:      string(o.Side),
			Price:     o.Price,
			Remaining: o.Remaining,
			Timestamp: time.Now().UTC(),
		})
	}
}

func (e *Engine) handleCancel(c *CancelInfo) {
	if c == nil {
		return
	}
	b := e.book(c.Symbol)
	if b.Cancel(c.OrderID) {
		e.bus.Publish(eventbus.OrderCancelledEvent{
			OrderID:   c.OrderID,
			Symbol:    c.Symbol,
			Timestamp: time.Now().UTC(),
		})
	}
}
