package account

import (
	"time"

	"github.com/google/uuid"
)

type Account struct {
	ID        uuid.UUID  `db:"id"`
	UserID    uuid.UUID  `db:"user_id"`
	Asset     string     `db:"asset"`
	CreatedAt *time.Time `db:"created_at"`
}

func NewAccountID() (uuid.UUID, error) {
	return uuid.NewV7()
}
