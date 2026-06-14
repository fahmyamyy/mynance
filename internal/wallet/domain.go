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
	NetworkID uuid.UUID  `db:"network_id"`
	Address   string     `db:"address"`
	CreatedAt *time.Time `db:"created_at"`
}

func NewID() (uuid.UUID, error) {
	return uuid.NewV7()
}

// generateMockAddress mints a fake deposit address that satisfies the
// network's regex (so the same value can be reused for withdrawal-target
// validation). The format is determined by network name, not asset, because
// the same asset (e.g. USDT) lives on multiple chains with very different
// address shapes.
func generateMockAddress(networkName string) (string, error) {
	switch networkName {
	case "Bitcoin":
		b := make([]byte, 20)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("address random: %w", err)
		}
		return "bc1" + base58Like(b, 39), nil
	case "ERC20", "BEP20":
		b := make([]byte, 20)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("address random: %w", err)
		}
		return "0x" + hex.EncodeToString(b), nil
	case "TRON":
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("address random: %w", err)
		}
		return "T" + base58Like(b, 33), nil
	case "Solana":
		b := make([]byte, 24)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("address random: %w", err)
		}
		return base58Like(b, 44), nil
	default:
		// Fallback used by older clients or unknown chains — won't pass strict
		// pattern checks but keeps the API responsive.
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return "", fmt.Errorf("address random: %w", err)
		}
		return "mock-" + strings.ToLower(networkName) + "-" + hex.EncodeToString(b), nil
	}
}

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// base58Like maps random bytes into a base58-style charset, padding/truncating
// to length n. Not RFC-compliant; just produces something that matches the
// permissive regex patterns we ship.
func base58Like(seed []byte, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = base58Alphabet[int(seed[i%len(seed)])%len(base58Alphabet)]
		// Cheap diffusion so the trailing characters aren't all identical.
		seed[i%len(seed)] = byte((int(seed[i%len(seed)]) + i*31) & 0xff)
	}
	return string(out)
}
