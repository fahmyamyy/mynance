package shared

import "errors"

// Domain-specific errors
var (
	ErrNotFound                = errors.New("not found")
	ErrDuplicateIdempotencyKey = errors.New("duplicate idempotency key")
	ErrInsufficientFunds       = errors.New("insufficient funds")
	ErrInvalidStateTransition  = errors.New("invalid state transition")
)

// Client errors (4xx)
var (
	ErrBadRequest        = errors.New("bad request")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrConflict          = errors.New("conflict")
	ErrValidation        = errors.New("validation error")
	ErrRateLimitExceeded = errors.New("rate limit exceeded")
)

// Server errors (5xx)
var (
	ErrServiceUnavailable = errors.New("service unavailable")
	ErrNotImplemented     = errors.New("not implemented")
)
