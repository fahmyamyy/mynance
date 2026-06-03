package ledger

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type EntryType string

const (
	EntryTypeReserve EntryType = "RESERVE"
	EntryTypeRelease EntryType = "RELEASE"
	EntryTypeTrade   EntryType = "TRADE"
)

type RefType string

const (
	RefTypeOrder RefType = "ORDER"
	RefTypeTrade RefType = "TRADE"
)

type LedgerEntry struct {
	ID        uuid.UUID      `db:"id"`
	UserID    uuid.UUID      `db:"user_id"`
	AccountID uuid.UUID      `db:"account_id"`
	Asset     string         `db:"asset"`
	Amount    pgtype.Numeric `db:"amount"`
	EntryType EntryType      `db:"entry_type"`
	RefType   RefType        `db:"ref_type"`
	RefID     uuid.UUID      `db:"ref_id"`
	CreatedAt *time.Time     `db:"created_at"`
}

func NewLedgerEntryID() (uuid.UUID, error) {
	return uuid.NewV7()
}
