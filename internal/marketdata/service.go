package marketdata

import (
	"sync"
	"time"

	"mynance/internal/eventbus"
)

const maxRecentTrades = 100

type BookLevel struct {
	Price    float64 `json:"price"`
	Quantity float64 `json:"quantity"`
}

type OrderBookView struct {
	Symbol string      `json:"symbol"`
	Bids   []BookLevel `json:"bids"`
	Asks   []BookLevel `json:"asks"`
}

type RecentTrade struct {
	Price     float64   `json:"price"`
	Quantity  float64   `json:"quantity"`
	Timestamp time.Time `json:"timestamp"`
}

type Service struct {
	mu     sync.RWMutex
	books  map[string]*OrderBookView
	trades map[string][]RecentTrade
}

func NewService() *Service {
	return &Service{
		books:  make(map[string]*OrderBookView),
		trades: make(map[string][]RecentTrade),
	}
}

func (s *Service) Subscribe(bus *eventbus.Bus) {
	bus.Subscribe(eventbus.EventTypeOrderRested, s.OnOrderRested)
	bus.Subscribe(eventbus.EventTypeTradeMatched, s.OnTradeMatched)
}

func (s *Service) OnOrderRested(e eventbus.Event) {
	evt, ok := e.(eventbus.OrderRestedEvent)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	book := s.bookFor(evt.Symbol)
	side := "asks"
	if evt.Side == "BUY" {
		side = "bids"
	}
	addLevel(book, side, evt.Price, evt.Remaining)
}

func (s *Service) OnTradeMatched(e eventbus.Event) {
	evt, ok := e.(eventbus.TradeMatchedEvent)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	book := s.bookFor(evt.Symbol)
	// Trade consumes from both sides; deduct quantity from any matching level.
	deductLevel(book, "asks", evt.Price, evt.Quantity)
	deductLevel(book, "bids", evt.Price, evt.Quantity)

	tr := RecentTrade{Price: evt.Price, Quantity: evt.Quantity, Timestamp: evt.Timestamp}
	s.trades[evt.Symbol] = append([]RecentTrade{tr}, s.trades[evt.Symbol]...)
	if len(s.trades[evt.Symbol]) > maxRecentTrades {
		s.trades[evt.Symbol] = s.trades[evt.Symbol][:maxRecentTrades]
	}
}

func (s *Service) GetOrderBook(symbol string) OrderBookView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.books[symbol]
	if !ok {
		return OrderBookView{Symbol: symbol, Bids: []BookLevel{}, Asks: []BookLevel{}}
	}
	out := OrderBookView{Symbol: symbol, Bids: make([]BookLevel, len(b.Bids)), Asks: make([]BookLevel, len(b.Asks))}
	copy(out.Bids, b.Bids)
	copy(out.Asks, b.Asks)
	return out
}

func (s *Service) GetRecentTrades(symbol string) []RecentTrade {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t := s.trades[symbol]
	out := make([]RecentTrade, len(t))
	copy(out, t)
	return out
}

func (s *Service) bookFor(symbol string) *OrderBookView {
	if b, ok := s.books[symbol]; ok {
		return b
	}
	b := &OrderBookView{Symbol: symbol, Bids: []BookLevel{}, Asks: []BookLevel{}}
	s.books[symbol] = b
	return b
}

func addLevel(book *OrderBookView, side string, price, qty float64) {
	levels := &book.Bids
	if side == "asks" {
		levels = &book.Asks
	}
	for i := range *levels {
		if (*levels)[i].Price == price {
			(*levels)[i].Quantity += qty
			return
		}
	}
	*levels = append(*levels, BookLevel{Price: price, Quantity: qty})
}

func deductLevel(book *OrderBookView, side string, price, qty float64) {
	levels := &book.Bids
	if side == "asks" {
		levels = &book.Asks
	}
	for i := 0; i < len(*levels); i++ {
		if (*levels)[i].Price == price {
			(*levels)[i].Quantity -= qty
			if (*levels)[i].Quantity <= 0 {
				*levels = append((*levels)[:i], (*levels)[i+1:]...)
			}
			return
		}
	}
}
