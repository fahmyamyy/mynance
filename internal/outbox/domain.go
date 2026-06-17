package outbox

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusProcessed Status = "PROCESSED"
)

type EventType string

const (
	EventTypeOrderPlaced    EventType = "ORDER_PLACED"
	EventTypeOrderCancelled EventType = "ORDER_CANCELLED"
	EventTypeTradeExecuted  EventType = "TRADE_EXECUTED"
)

type OutboxEvent struct {
	ID          uuid.UUID  `db:"id"`
	EventType   EventType  `db:"event_type"`
	Payload     []byte     `db:"payload"`
	Status      Status     `db:"status"`
	Retries     int        `db:"retries"`
	ProcessedAt *time.Time `db:"processed_at"`
	CreatedAt   *time.Time `db:"created_at"`
}

func NewID() (uuid.UUID, error) {
	return uuid.NewV7()
}
