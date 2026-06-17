package deposit

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusConfirmed Status = "CONFIRMED"
	StatusRejected  Status = "REJECTED"
)

type Deposit struct {
	ID          uuid.UUID      `db:"id"`
	UserID      uuid.UUID      `db:"user_id"`
	Asset       string         `db:"asset"`
	NetworkID   uuid.UUID      `db:"network_id"`
	Address     string         `db:"address"`
	Amount      pgtype.Numeric `db:"amount"`
	TxHash      string         `db:"tx_hash"`
	Status      Status         `db:"status"`
	CreatedAt   *time.Time     `db:"created_at"`
	ConfirmedAt *time.Time     `db:"confirmed_at"`
}

func NewID() (uuid.UUID, error) {
	return uuid.NewV7()
}
