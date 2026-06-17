package shared

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

func HTTPError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func HandleServiceError(w http.ResponseWriter, err error) {
	var status int

	switch {
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, ErrBadRequest):
		status = http.StatusBadRequest
	case errors.Is(err, ErrValidation):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, ErrInsufficientFunds):
		status = http.StatusUnprocessableEntity
	case errors.Is(err, ErrConflict),
		errors.Is(err, ErrInvalidStateTransition):
		status = http.StatusConflict
	case errors.Is(err, ErrDuplicateIdempotencyKey):
		status = http.StatusOK
	case errors.Is(err, ErrRateLimitExceeded):
		status = http.StatusTooManyRequests
	case errors.Is(err, ErrServiceUnavailable):
		status = http.StatusServiceUnavailable
	case errors.Is(err, ErrNotImplemented):
		status = http.StatusNotImplemented
	default:
		status = http.StatusInternalServerError
	}

	if status == http.StatusOK {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: err.Error()})
}

func ParsePagination(r *http.Request) (limit, offset int) {
	limit = 50
	offset = 0
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 100 {
		limit = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && v >= 0 {
		offset = v
	}
	return
}
