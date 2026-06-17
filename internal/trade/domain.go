package trade

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Trade struct {
	ID          uuid.UUID      `db:"id"`
	Symbol      string         `db:"symbol"`
	BuyOrderID  uuid.UUID      `db:"buy_order_id"`
	SellOrderID uuid.UUID      `db:"sell_order_id"`
	BuyUserID   uuid.UUID      `db:"buy_user_id"`
	SellUserID  uuid.UUID      `db:"sell_user_id"`
	Price       pgtype.Numeric `db:"price"`
	Quantity    pgtype.Numeric `db:"quantity"`
	CreatedAt   *time.Time     `db:"created_at"`
}

func NewID() (uuid.UUID, error) {
	return uuid.NewV7()
}

type UserTrade struct {
	ID                 uuid.UUID
	Symbol             string
	Side               string
	Price              pgtype.Numeric
	Quantity           pgtype.Numeric
	CounterpartyUserID uuid.UUID
	CreatedAt          *time.Time
}
