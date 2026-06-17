package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const DefaultTTL = 24 * time.Hour

var ErrInvalidToken = errors.New("invalid token")

type Claims struct {
	UserID uuid.UUID
	Role   string
}

type Signer struct {
	secret []byte
	ttl    time.Duration
}

func NewSigner(secret string) *Signer {
	return &Signer{secret: []byte(secret), ttl: DefaultTTL}
}

func (s *Signer) Sign(userID uuid.UUID, role string) (string, error) {
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"sub":  userID.String(),
		"role": role,
		"iat":  now.Unix(),
		"exp":  now.Add(s.ttl).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("auth.Sign: %w", err)
	}
	return signed, nil
}

func (s *Signer) Verify(token string) (Claims, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil || !parsed.Valid {
		return Claims{}, ErrInvalidToken
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, ErrInvalidToken
	}
	subStr, _ := mc["sub"].(string)
	id, err := uuid.Parse(subStr)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	role, _ := mc["role"].(string)
	if role == "" {
		return Claims{}, ErrInvalidToken
	}
	return Claims{UserID: id, Role: role}, nil
}
