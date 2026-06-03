package marketfeed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxRecentTrades = 100
	stalenessWindow = 60 * time.Second
	readDeadline    = 11 * time.Minute
)

type Client struct {
	symbols []string
	enabled bool

	mu     sync.RWMutex
	books  map[string]*OrderBookSnapshot
	tickers map[string]*TickerSnapshot
	trades map[string][]TradeSnapshot
}

func NewClient(symbols []string, enabled bool) *Client {
	return &Client{
		symbols: symbols,
		enabled: enabled,
		books:   make(map[string]*OrderBookSnapshot),
		tickers: make(map[string]*TickerSnapshot),
		trades:  make(map[string][]TradeSnapshot),
	}
}

func (c *Client) HasSymbol(symbol string) bool {
	for _, s := range c.symbols {
		if s == symbol {
			return true
		}
	}
	return false
}

func (c *Client) Enabled() bool { return c.enabled }

func (c *Client) Start(ctx context.Context) {
	if !c.enabled {
		slog.Info("marketfeed disabled")
		return
	}
	slog.Info("marketfeed starting", "symbols", c.symbols)
	attempt := 0
	for {
		if err := c.connectAndStream(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			backoff := time.Duration(1<<attempt) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			slog.Warn("marketfeed reconnecting", "backoff", backoff, "err", err)
			attempt++
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			continue
		}
		attempt = 0
	}
}

func (c *Client) buildURL() string {
	streams := []string{}
	for _, s := range c.symbols {
		b := toBinance(s)
		streams = append(streams,
			b+"@depth20@100ms",
			b+"@trade",
			b+"@ticker",
		)
	}
	return "wss://stream.binance.com:9443/stream?streams=" + strings.Join(streams, "/")
}

func (c *Client) connectAndStream(ctx context.Context) error {
	url := c.buildURL()
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(readDeadline))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(readDeadline))
	})
	conn.SetPingHandler(func(payload string) error {
		conn.SetReadDeadline(time.Now().Add(readDeadline))
		return conn.WriteControl(websocket.PongMessage, []byte(payload), time.Now().Add(5*time.Second))
	})

	slog.Info("marketfeed connected")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		c.dispatch(msg)
	}
}

type combinedEnvelope struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

func (c *Client) dispatch(msg []byte) {
	var env combinedEnvelope
	if err := json.Unmarshal(msg, &env); err != nil {
		return
	}
	parts := strings.Split(env.Stream, "@")
	if len(parts) < 2 {
		return
	}
	bSym := parts[0]
	symbol, err := fromBinance(bSym)
	if err != nil {
		return
	}
	streamType := parts[1]
	switch {
	case strings.HasPrefix(streamType, "depth20"):
		c.onDepth(symbol, env.Data)
	case streamType == "trade":
		c.onTrade(symbol, env.Data)
	case streamType == "ticker":
		c.onTicker(symbol, env.Data)
	}
}

type depthMsg struct {
	Bids [][]string `json:"bids"`
	Asks [][]string `json:"asks"`
}

func (c *Client) onDepth(symbol string, data []byte) {
	var d depthMsg
	if err := json.Unmarshal(data, &d); err != nil {
		return
	}
	snap := &OrderBookSnapshot{
		Symbol:    symbol,
		Bids:      parseLevels(d.Bids),
		Asks:      parseLevels(d.Asks),
		UpdatedAt: time.Now(),
	}
	c.mu.Lock()
	c.books[symbol] = snap
	c.mu.Unlock()
}

func parseLevels(raw [][]string) [][2]float64 {
	out := make([][2]float64, 0, len(raw))
	for _, lvl := range raw {
		if len(lvl) < 2 {
			continue
		}
		price, err1 := strconv.ParseFloat(lvl[0], 64)
		qty, err2 := strconv.ParseFloat(lvl[1], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		out = append(out, [2]float64{price, qty})
	}
	return out
}

type tradeMsg struct {
	Price        string `json:"p"`
	Quantity     string `json:"q"`
	TradeTime    int64  `json:"T"`
	IsBuyerMaker bool   `json:"m"`
}

func (c *Client) onTrade(symbol string, data []byte) {
	var t tradeMsg
	if err := json.Unmarshal(data, &t); err != nil {
		return
	}
	price, _ := strconv.ParseFloat(t.Price, 64)
	qty, _ := strconv.ParseFloat(t.Quantity, 64)
	trade := TradeSnapshot{
		Price:        price,
		Quantity:     qty,
		Timestamp:    time.UnixMilli(t.TradeTime),
		IsBuyerMaker: t.IsBuyerMaker,
	}
	c.mu.Lock()
	list := c.trades[symbol]
	list = append([]TradeSnapshot{trade}, list...)
	if len(list) > maxRecentTrades {
		list = list[:maxRecentTrades]
	}
	c.trades[symbol] = list
	c.mu.Unlock()
}

type tickerMsg struct {
	Open         string `json:"o"`
	High         string `json:"h"`
	Low          string `json:"l"`
	Close        string `json:"c"`
	Volume       string `json:"v"`
	PriceChange  string `json:"p"`
	PriceChangePct string `json:"P"`
}

func (c *Client) onTicker(symbol string, data []byte) {
	var t tickerMsg
	if err := json.Unmarshal(data, &t); err != nil {
		return
	}
	open, _ := strconv.ParseFloat(t.Open, 64)
	high, _ := strconv.ParseFloat(t.High, 64)
	low, _ := strconv.ParseFloat(t.Low, 64)
	cls, _ := strconv.ParseFloat(t.Close, 64)
	vol, _ := strconv.ParseFloat(t.Volume, 64)
	change, _ := strconv.ParseFloat(t.PriceChange, 64)
	pct, _ := strconv.ParseFloat(t.PriceChangePct, 64)
	snap := &TickerSnapshot{
		Symbol: symbol, Open: open, High: high, Low: low, Close: cls,
		Volume: vol, Change24h: change, ChangePct24h: pct, UpdatedAt: time.Now(),
	}
	c.mu.Lock()
	c.tickers[symbol] = snap
	c.mu.Unlock()
}

func (c *Client) GetOrderBook(symbol string) (OrderBookSnapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	b, ok := c.books[symbol]
	if !ok || time.Since(b.UpdatedAt) > stalenessWindow {
		return OrderBookSnapshot{}, false
	}
	out := *b
	out.Bids = append([][2]float64(nil), b.Bids...)
	out.Asks = append([][2]float64(nil), b.Asks...)
	return out, true
}

func (c *Client) GetTicker(symbol string) (TickerSnapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.tickers[symbol]
	if !ok || time.Since(t.UpdatedAt) > stalenessWindow {
		return TickerSnapshot{}, false
	}
	return *t, true
}

func (c *Client) GetTrades(symbol string) ([]TradeSnapshot, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.trades[symbol]
	if !ok {
		return nil, false
	}
	out := make([]TradeSnapshot, len(t))
	copy(out, t)
	return out, true
}

func (c *Client) Markets() []TickerSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]TickerSnapshot, 0, len(c.symbols))
	for _, sym := range c.symbols {
		if t, ok := c.tickers[sym]; ok {
			out = append(out, *t)
		}
	}
	return out
}
