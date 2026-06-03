package order

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Side string

const (
	SideBuy  Side = "BUY"
	SideSell Side = "SELL"
)

type Status string

const (
	StatusOpen      Status = "OPEN"
	StatusPartial   Status = "PARTIAL"
	StatusFilled    Status = "FILLED"
	StatusCancelled Status = "CANCELLED"
)

type Order struct {
	ID             uuid.UUID      `db:"id"`
	UserID         uuid.UUID      `db:"user_id"`
	Symbol         string         `db:"symbol"`
	Side           Side           `db:"side"`
	Price          pgtype.Numeric `db:"price"`
	Quantity       pgtype.Numeric `db:"quantity"`
	FilledQuantity pgtype.Numeric `db:"filled_quantity"`
	Status         Status         `db:"status"`
	CreatedAt      *time.Time     `db:"created_at"`
	UpdatedAt      *time.Time     `db:"updated_at"`
}

func NewID() (uuid.UUID, error) {
	return uuid.NewV7()
}

func SplitSymbol(symbol string) (base, quote string, err error) {
	parts := strings.Split(symbol, "-")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid symbol %q (expected BASE-QUOTE)", symbol)
	}
	return parts[0], parts[1], nil
}

func IsTerminal(s Status) bool {
	return s == StatusFilled || s == StatusCancelled
}

func IsCancellable(s Status) bool {
	return s == StatusOpen || s == StatusPartial
}
