package user

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoleUser  = "USER"
	RoleAdmin = "ADMIN"
)

type User struct {
	ID           uuid.UUID  `db:"id"`
	Email        string     `db:"email"`
	Username     string     `db:"username"`
	FullName     string     `db:"full_name"`
	PasswordHash string     `db:"password_hash"`
	Status       string     `db:"status"` // ACTIVE | SUSPENDED | CLOSED
	Role         string     `db:"role"`   // USER | ADMIN
	DeletedAt    *time.Time `db:"deleted_at"`
	CreatedAt    *time.Time `db:"created_at"`
	UpdatedAt    *time.Time `db:"updated_at"`
}

func NewUserID() (uuid.UUID, error) {
	return uuid.NewV7()
}
