package marketfeed

import "time"

type OrderBookSnapshot struct {
	Symbol    string       `json:"symbol"`
	Bids      [][2]float64 `json:"bids"`
	Asks      [][2]float64 `json:"asks"`
	UpdatedAt time.Time    `json:"-"`
}

type TickerSnapshot struct {
	Symbol       string    `json:"symbol"`
	Base         string    `json:"base"`
	Quote        string    `json:"quote"`
	Open         float64   `json:"open"`
	High         float64   `json:"high"`
	Low          float64   `json:"low"`
	Close        float64   `json:"close"`
	Volume       float64   `json:"volume"`
	Change24h    float64   `json:"change_24h"`
	ChangePct24h float64   `json:"change_pct_24h"`
	UpdatedAt    time.Time `json:"-"`
}

type TradeSnapshot struct {
	Price         float64   `json:"price"`
	Quantity      float64   `json:"qty"`
	Timestamp     time.Time `json:"timestamp"`
	IsBuyerMaker  bool      `json:"is_buyer_maker"`
}

// KlineSnapshot matches the FE candle contract — decimal-string fields so
// downstream consumers don't lose precision through float JSON.
type KlineSnapshot struct {
	Symbol    string `json:"symbol"`
	Interval  string `json:"interval"`
	OpenTime  int64  `json:"open_time"`
	CloseTime int64  `json:"close_time"`
	Open      string `json:"open"`
	High      string `json:"high"`
	Low       string `json:"low"`
	Close     string `json:"close"`
	Volume    string `json:"volume"`
	Closed    bool   `json:"closed"` // true once the candle is final
}
