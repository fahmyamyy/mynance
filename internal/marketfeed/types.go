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
