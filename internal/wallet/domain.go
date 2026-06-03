package wallet

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type WalletAddress struct {
	ID        uuid.UUID  `db:"id"`
	UserID    uuid.UUID  `db:"user_id"`
	Asset     string     `db:"asset"`
	Address   string     `db:"address"`
	CreatedAt *time.Time `db:"created_at"`
}

func NewID() (uuid.UUID, error) {
	return uuid.NewV7()
}

func generateMockAddress(asset string, userID uuid.UUID) (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("address random: %w", err)
	}
	return fmt.Sprintf("mock-%s-%s-%s",
		strings.ToLower(asset),
		userID.String()[:8],
		hex.EncodeToString(b),
	), nil
}
