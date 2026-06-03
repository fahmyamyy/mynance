package idempotency

import (
	"time"

	"github.com/google/uuid"
)

type Scope string

const (
	ScopeOrder Scope = "ORDER"
	ScopeTrade Scope = "TRADE"
)

type IdempotencyKey struct {
	ID        uuid.UUID  `db:"id"`
	Key       string     `db:"key"`
	Scope     Scope      `db:"scope"`
	CreatedAt *time.Time `db:"created_at"`
}

func NewID() (uuid.UUID, error) {
	return uuid.NewV7()
}
