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

// KlineHandler receives every kline tick parsed from Binance for any
// (symbol, interval) the client is subscribed to.
type KlineHandler func(k KlineSnapshot)

// TickerHandler fires on every ticker tick (~1/sec per symbol from Binance).
type TickerHandler func(t TickerSnapshot)

// TradeHandler fires on every aggregated trade arrival.
type TradeHandler func(symbol string, t TradeSnapshot)

type Client struct {
	symbols   []string
	intervals []string
	enabled   bool

	mu     sync.RWMutex
	books  map[string]*OrderBookSnapshot
	tickers map[string]*TickerSnapshot
	trades map[string][]TradeSnapshot

	handlersMu     sync.RWMutex
	klineHandlers  []KlineHandler
	tickerHandlers []TickerHandler
	tradeHandlers  []TradeHandler
}

func NewClient(symbols []string, enabled bool) *Client {
	return &Client{
		symbols:   symbols,
		intervals: []string{"1m", "5m", "15m", "1h", "4h", "1d"},
		enabled:   enabled,
		books:     make(map[string]*OrderBookSnapshot),
		tickers:   make(map[string]*TickerSnapshot),
		trades:    make(map[string][]TradeSnapshot),
	}
}

func (c *Client) SubscribeKline(h KlineHandler) {
	c.handlersMu.Lock()
	c.klineHandlers = append(c.klineHandlers, h)
	c.handlersMu.Unlock()
}

func (c *Client) SubscribeTicker(h TickerHandler) {
	c.handlersMu.Lock()
	c.tickerHandlers = append(c.tickerHandlers, h)
	c.handlersMu.Unlock()
}

func (c *Client) SubscribeTrade(h TradeHandler) {
	c.handlersMu.Lock()
	c.tradeHandlers = append(c.tradeHandlers, h)
	c.handlersMu.Unlock()
}

func (c *Client) emitKline(k KlineSnapshot) {
	c.handlersMu.RLock()
	hs := c.klineHandlers
	c.handlersMu.RUnlock()
	for _, h := range hs {
		h(k)
	}
}

func (c *Client) emitTicker(t TickerSnapshot) {
	c.handlersMu.RLock()
	hs := c.tickerHandlers
	c.handlersMu.RUnlock()
	for _, h := range hs {
		h(t)
	}
}

func (c *Client) emitTrade(symbol string, t TradeSnapshot) {
	c.handlersMu.RLock()
	hs := c.tradeHandlers
	c.handlersMu.RUnlock()
	for _, h := range hs {
		h(symbol, t)
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
		for _, iv := range c.intervals {
			streams = append(streams, b+"@kline_"+iv)
		}
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
	case strings.HasPrefix(streamType, "kline_"):
		interval := strings.TrimPrefix(streamType, "kline_")
		c.onKline(symbol, interval, env.Data)
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
	c.emitTrade(symbol, trade)
}

// Binance @ticker payload mixes lowercase ("o","h","l","c","v","p") with uppercase
// keys ("O","C","P","L") for distinct fields. encoding/json matches struct tags
// case-insensitively, causing collisions — so decode into a raw map and read by
// exact key.
func (c *Client) onTicker(symbol string, data []byte) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		slog.Warn("ticker unmarshal", "err", err)
		return
	}
	base, quote := splitSymbol(symbol)
	snap := &TickerSnapshot{
		Symbol:       symbol,
		Base:         base,
		Quote:        quote,
		Open:         tickerFloat(raw, "o"),
		High:         tickerFloat(raw, "h"),
		Low:          tickerFloat(raw, "l"),
		Close:        tickerFloat(raw, "c"),
		Volume:       tickerFloat(raw, "v"),
		Change24h:    tickerFloat(raw, "p"),
		ChangePct24h: tickerFloat(raw, "P"),
		UpdatedAt:    time.Now(),
	}
	c.mu.Lock()
	c.tickers[symbol] = snap
	c.mu.Unlock()
	c.emitTicker(*snap)
}

// onKline parses a Binance kline-stream payload (combined-stream envelope's
// data is the full kline event, of which the "k" sub-object holds the candle
// fields). Same case-collision rationale as onTicker — uppercase "T", "L", "V"
// belong to distinct event-level fields, so the sub-object is decoded via raw
// map to keep cases distinct.
func (c *Client) onKline(symbol, interval string, data []byte) {
	var env struct {
		K json.RawMessage `json:"k"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	if len(env.K) == 0 {
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(env.K, &raw); err != nil {
		return
	}
	snap := KlineSnapshot{
		Symbol:    symbol,
		Interval:  interval,
		OpenTime:  klineInt(raw, "t"),
		CloseTime: klineInt(raw, "T"),
		Open:      klineStr(raw, "o"),
		High:      klineStr(raw, "h"),
		Low:       klineStr(raw, "l"),
		Close:     klineStr(raw, "c"),
		Volume:    klineStr(raw, "v"),
		Closed:    klineBool(raw, "x"),
	}
	c.emitKline(snap)
}

func klineInt(raw map[string]json.RawMessage, key string) int64 {
	v, ok := raw[key]
	if !ok {
		return 0
	}
	var n int64
	_ = json.Unmarshal(v, &n)
	return n
}

func klineStr(raw map[string]json.RawMessage, key string) string {
	v, ok := raw[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	return strings.Trim(string(v), `"`)
}

func klineBool(raw map[string]json.RawMessage, key string) bool {
	v, ok := raw[key]
	if !ok {
		return false
	}
	var b bool
	_ = json.Unmarshal(v, &b)
	return b
}

func tickerFloat(raw map[string]json.RawMessage, key string) float64 {
	v, ok := raw[key]
	if !ok {
		return 0
	}
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	var f float64
	_ = json.Unmarshal(v, &f)
	return f
}

// RawSnapshot returns the raw bid/ask level slices for callers that don't
// need the full OrderBookSnapshot envelope (e.g. partnerfeed).
func (c *Client) RawSnapshot(symbol string) (bids [][2]float64, asks [][2]float64, ok bool) {
	snap, ok := c.GetOrderBook(symbol)
	if !ok {
		return nil, nil, false
	}
	return snap.Bids, snap.Asks, true
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
