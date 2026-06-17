// Package klines proxies candlestick history from Binance and reshapes the
// response into the FE contract (object form with decimal-string values).
package klines

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Kline is the FE-facing candle shape. All numeric fields are decimal strings
// so the client can use a fixed-point library instead of trusting float JSON.
type Kline struct {
	OpenTime  int64  `json:"open_time"`
	CloseTime int64  `json:"close_time"`
	Open      string `json:"open"`
	High      string `json:"high"`
	Low       string `json:"low"`
	Close     string `json:"close"`
	Volume    string `json:"volume"`
}

// validIntervals maps the FE-allowed interval strings to the Binance form.
// FE → Binance is 1:1 today but we keep an explicit table so the contract
// can drift independently from upstream.
var validIntervals = map[string]string{
	"1m":  "1m",
	"5m":  "5m",
	"15m": "15m",
	"30m": "30m",
	"1h":  "1h",
	"4h":  "4h",
	"1d":  "1d",
	"1w":  "1w",
}

type Service struct {
	httpClient *http.Client
	baseURL    string
}

func NewService() *Service {
	return &Service{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    "https://api.binance.com",
	}
}

type Query struct {
	Symbol    string // FE form, e.g. "BTC-USDT"
	Interval  string
	StartTime int64 // 0 = unset
	EndTime   int64
	Limit     int
}

func (s *Service) Fetch(ctx context.Context, q Query) ([]Kline, error) {
	bSym := toBinanceSymbol(q.Symbol)
	if bSym == "" {
		return nil, fmt.Errorf("klines: invalid symbol %q", q.Symbol)
	}
	bInterval, ok := validIntervals[q.Interval]
	if !ok {
		return nil, fmt.Errorf("klines: invalid interval %q", q.Interval)
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 500
	}
	if limit > 1000 {
		limit = 1000
	}

	v := url.Values{}
	v.Set("symbol", bSym)
	v.Set("interval", bInterval)
	v.Set("limit", strconv.Itoa(limit))
	if q.StartTime > 0 {
		v.Set("startTime", strconv.FormatInt(q.StartTime, 10))
	}
	if q.EndTime > 0 {
		v.Set("endTime", strconv.FormatInt(q.EndTime, 10))
	}

	endpoint := s.baseURL + "/api/v3/klines?" + v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("klines: build request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("klines: http call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("klines: read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("klines: upstream %d: %s", resp.StatusCode, string(body))
	}

	// Binance returns tuples: [openTime, open, high, low, close, volume,
	// closeTime, quoteVolume, trades, takerBuyBase, takerBuyQuote, ignore].
	var raw [][]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("klines: decode: %w", err)
	}
	out := make([]Kline, 0, len(raw))
	for _, row := range raw {
		if len(row) < 7 {
			continue
		}
		k := Kline{
			OpenTime:  decodeInt(row[0]),
			Open:      decodeString(row[1]),
			High:      decodeString(row[2]),
			Low:       decodeString(row[3]),
			Close:     decodeString(row[4]),
			Volume:    decodeString(row[5]),
			CloseTime: decodeInt(row[6]),
		}
		out = append(out, k)
	}
	return out, nil
}

func decodeInt(v json.RawMessage) int64 {
	var n int64
	_ = json.Unmarshal(v, &n)
	return n
}

func decodeString(v json.RawMessage) string {
	var s string
	if err := json.Unmarshal(v, &s); err == nil {
		return s
	}
	// Fall back to raw representation for numeric forms.
	return strings.Trim(string(v), `"`)
}

// toBinanceSymbol "BTC-USDT" → "BTCUSDT". Returns "" on malformed input.
func toBinanceSymbol(s string) string {
	out := strings.ReplaceAll(s, "-", "")
	if out == "" || out == s && !strings.Contains(s, "-") {
		// FE always sends hyphenated form; reject anything else to keep the
		// contract tight and avoid accidental upstream symbol leakage.
		return ""
	}
	return strings.ToUpper(out)
}
