package withdrawal

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mynance/internal/auth"
	"mynance/internal/shared"
	"mynance/pkg/numeric"
	"mynance/pkg/validate"
)

type Handler struct {
	withdrawalService Service
}

func NewHandler(svc Service) *Handler { return &Handler{withdrawalService: svc} }

type WithdrawRequest struct {
	Amount         string `json:"amount" validate:"required"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,max=255"`
}

func (r WithdrawRequest) Validate() error { return validate.Struct(r) }

type WithdrawResponse struct {
	WithdrawalID string `json:"withdrawal_id"`
	AccountID    string `json:"account_id"`
	Amount       string `json:"amount"`
	NewBalance   string `json:"new_balance"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
}

func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r.Context())
	if err != nil {
		shared.HTTPError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid account id")
		return
	}
	var req WithdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		shared.HTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	amount, err := numeric.Parse(req.Amount)
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid amount")
		return
	}
	res, err := h.withdrawalService.Withdraw(r.Context(), WithdrawCommand{
		UserID: userID, AccountID: accountID, Amount: amount, IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		if errors.Is(err, shared.ErrDuplicateIdempotencyKey) {
			w.WriteHeader(http.StatusOK)
			return
		}
		shared.HandleServiceError(w, err)
		return
	}
	createdAt := ""
	if res.Withdrawal.CreatedAt != nil {
		createdAt = res.Withdrawal.CreatedAt.Format(time.RFC3339)
	}
	shared.WriteJSON(w, http.StatusOK, WithdrawResponse{
		WithdrawalID: res.Withdrawal.ID.String(),
		AccountID:    res.Withdrawal.AccountID.String(),
		Amount:       numeric.String(res.Withdrawal.Amount),
		NewBalance:   res.NewBalance,
		Status:       res.Withdrawal.Status,
		CreatedAt:    createdAt,
	})
}
