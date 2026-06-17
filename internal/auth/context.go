package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type userIDKey struct{}
type roleKey struct{}

var ErrNoUserInContext = errors.New("no user_id in context")

func WithUser(ctx context.Context, userID uuid.UUID, role string) context.Context {
	ctx = context.WithValue(ctx, userIDKey{}, userID)
	ctx = context.WithValue(ctx, roleKey{}, role)
	return ctx
}

func UserIDFromContext(ctx context.Context) (uuid.UUID, error) {
	v, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	if !ok {
		return uuid.Nil, ErrNoUserInContext
	}
	return v, nil
}

func RoleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(roleKey{}).(string)
	return v
}

func IsAdmin(ctx context.Context) bool {
	return RoleFromContext(ctx) == "ADMIN"
}
