package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"mynance/internal/domain"
)

type errorResponse struct {
	Error string `json:"error"`
}

func handleServiceError(w http.ResponseWriter, err error) {
	var status int

	switch {
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, domain.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, domain.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, domain.ErrBadRequest):
		status = http.StatusBadRequest
	case errors.Is(err, domain.ErrValidation):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, domain.ErrInsufficientFunds):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, domain.ErrConflict),
		errors.Is(err, domain.ErrInvalidStateTransition):
		status = http.StatusConflict
	case errors.Is(err, domain.ErrDuplicateIdempotencyKey):
		status = http.StatusOK
	case errors.Is(err, domain.ErrRateLimitExceeded):
		status = http.StatusTooManyRequests
	case errors.Is(err, domain.ErrServiceUnavailable):
		status = http.StatusServiceUnavailable
	default:
		status = http.StatusInternalServerError
	}

	if status == http.StatusOK {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: err.Error()})
}
