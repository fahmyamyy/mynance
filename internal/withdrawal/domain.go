package withdrawal

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const StatusCompleted = "COMPLETED"

type Withdrawal struct {
	ID                 uuid.UUID      `db:"id"`
	UserID             uuid.UUID      `db:"user_id"`
	AccountID          uuid.UUID      `db:"account_id"`
	Asset              string         `db:"asset"`
	NetworkID          uuid.UUID      `db:"network_id"`
	DestinationAddress string         `db:"destination_address"`
	Amount             pgtype.Numeric `db:"amount"`
	Status             string         `db:"status"`
	IdempotencyKey     string         `db:"idempotency_key"`
	CreatedAt          *time.Time     `db:"created_at"`
}

func NewID() (uuid.UUID, error) {
	return uuid.NewV7()
}
