package eventbus

import "time"

type EventType string

const (
	EventTypeOrderRested    EventType = "ORDER_RESTED"
	EventTypeOrderCancelled EventType = "ORDER_CANCELLED"
	EventTypeTradeMatched   EventType = "TRADE_MATCHED"
)

type Event interface {
	Type() EventType
}

type OrderRestedEvent struct {
	OrderID   string
	UserID    string
	Symbol    string
	Side      string
	Price     float64
	Remaining float64
	Timestamp time.Time
}

func (OrderRestedEvent) Type() EventType { return EventTypeOrderRested }

type OrderCancelledEvent struct {
	OrderID   string
	Symbol    string
	Side      string
	Price     float64
	Remaining float64
	Timestamp time.Time
}

func (OrderCancelledEvent) Type() EventType { return EventTypeOrderCancelled }

type TradeMatchedEvent struct {
	// Seq is monotonic per symbol (each engine worker owns its own counter);
	// it is NOT globally ordered across symbols. Consumers must rely only on
	// per-symbol ordering and on the globally-unique order IDs for uniqueness.
	Seq         int
	Symbol      string
	BuyOrderID  string
	SellOrderID string
	BuyUserID   string
	SellUserID  string
	Price       float64
	Quantity    float64
	Timestamp   time.Time
}

func (TradeMatchedEvent) Type() EventType { return EventTypeTradeMatched }
