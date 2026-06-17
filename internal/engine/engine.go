package engine

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"mynance/internal/eventbus"
)

var (
	ErrSubmitChannelFull = errors.New("engine submit channel full")
	ErrUnknownSymbol     = errors.New("engine unknown symbol")
)

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

// shard is the per-symbol unit of execution: one OrderBook owned exclusively
// by one worker goroutine, fed by its own buffered command channel.
type shard struct {
	book  *OrderBook
	cmdCh chan Command
}

type Engine struct {
	mu      sync.Mutex // guards started
	shards  map[string]*shard
	bus     *eventbus.Bus
	started bool
}

// New constructs an engine for a fixed set of symbols. Each symbol gets its
// own OrderBook and buffered command channel up front; the shard map is never
// mutated after construction, so the hot path needs no locking.
func New(bus *eventbus.Bus, symbols []string, bufferSize int) *Engine {
	if bufferSize <= 0 {
		bufferSize = 1024
	}
	shards := make(map[string]*shard, len(symbols))
	for _, sym := range symbols {
		shards[sym] = &shard{
			book:  NewOrderBook(sym),
			cmdCh: make(chan Command, bufferSize),
		}
	}
	return &Engine{
		shards: shards,
		bus:    bus,
	}
}

// LoadOrder inserts a resting order without matching. Used for rehydration
// before Start. Orders for unconfigured symbols are ignored.
func (e *Engine) LoadOrder(o *Order) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started {
		slog.Warn("engine.LoadOrder called after Start; ignoring", "id", o.ID)
		return
	}
	sh, ok := e.shards[o.Symbol]
	if !ok {
		slog.Warn("engine.LoadOrder unknown symbol; ignoring", "symbol", o.Symbol, "id", o.ID)
		return
	}
	if o.Side == SideBuy {
		sh.book.addBid(o)
	} else {
		sh.book.addAsk(o)
	}
}

// Submit routes a command to the worker that owns its symbol. Returns
// ErrUnknownSymbol if the symbol is not configured, ErrSubmitChannelFull if
// that symbol's command channel is full.
func (e *Engine) Submit(cmd Command) error {
	symbol := commandSymbol(cmd)
	sh, ok := e.shards[symbol]
	if !ok {
		return ErrUnknownSymbol
	}
	select {
	case sh.cmdCh <- cmd:
		return nil
	default:
		return ErrSubmitChannelFull
	}
}

func commandSymbol(cmd Command) string {
	switch cmd.Kind {
	case CommandPlaceOrder:
		if cmd.Order != nil {
			return cmd.Order.Symbol
		}
	case CommandCancelOrder:
		if cmd.Cancel != nil {
			return cmd.Cancel.Symbol
		}
	}
	return ""
}

// Start launches one worker goroutine per configured symbol and blocks until
// every worker has exited after context cancellation.
func (e *Engine) Start(ctx context.Context) {
	e.mu.Lock()
	e.started = true
	e.mu.Unlock()

	slog.Info("engine started", "symbols", len(e.shards))
	var wg sync.WaitGroup
	for _, sh := range e.shards {
		wg.Add(1)
		go func(sh *shard) {
			defer wg.Done()
			e.runWorker(ctx, sh)
		}(sh)
	}
	wg.Wait()
	slog.Info("engine stopped")
}

// runWorker is the per-symbol command loop. It is the sole writer of its
// book and of its trade sequence counter, so neither needs locking.
func (e *Engine) runWorker(ctx context.Context, sh *shard) {
	var seq int
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-sh.cmdCh:
			e.handle(sh.book, &seq, cmd)
		}
	}
}

func (e *Engine) handle(book *OrderBook, seq *int, cmd Command) {
	switch cmd.Kind {
	case CommandPlaceOrder:
		e.handlePlace(book, seq, cmd.Order)
	case CommandCancelOrder:
		e.handleCancel(book, cmd.Cancel)
	}
}

func (e *Engine) handlePlace(book *OrderBook, seq *int, o *Order) {
	if o == nil {
		return
	}
	trades := book.Match(o)
	for i := range trades {
		(*seq)++
		t := trades[i]
		e.bus.Publish(eventbus.TradeMatchedEvent{
			Seq:         *seq,
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

func (e *Engine) handleCancel(book *OrderBook, c *CancelInfo) {
	if c == nil {
		return
	}
	res, ok := book.Cancel(c.OrderID)
	if !ok {
		return
	}
	e.bus.Publish(eventbus.OrderCancelledEvent{
		OrderID:   c.OrderID,
		Symbol:    c.Symbol,
		Side:      string(res.Side),
		Price:     res.Price,
		Remaining: res.Remaining,
		Timestamp: time.Now().UTC(),
	})
}
