package withdrawal

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"mynance/internal/auth"
	"mynance/internal/shared"
	"mynance/pkg/numeric"
	"mynance/pkg/validate"
)

type Handler struct {
	withdrawalService Service
	processor         Processor
}

func NewHandler(svc Service, processor Processor) *Handler {
	return &Handler{withdrawalService: svc, processor: processor}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Withdraw)
	r.Get("/", h.ListMine)
	return r
}

// WithdrawRequest is the same body shape in every environment. IdempotencyKey
// is enforced by the real processor; the sandbox processor ignores it and
// auto-generates one. Keeping the field on the DTO means FEs can keep sending
// it (or omit it) without forking the request type per environment.
type WithdrawRequest struct {
	Asset              string `json:"asset" validate:"required,max=20"`
	Network            string `json:"network" validate:"required,max=50"`
	DestinationAddress string `json:"destination_address" validate:"required,max=255"`
	Amount             string `json:"amount" validate:"required"`
	IdempotencyKey     string `json:"idempotency_key" validate:"omitempty,max=255"`
}

func (r WithdrawRequest) Validate() error { return validate.Struct(r) }

type WithdrawResponse struct {
	WithdrawalID       string `json:"withdrawal_id"`
	Asset              string `json:"asset"`
	NetworkID          string `json:"network_id"`
	DestinationAddress string `json:"destination_address"`
	Amount             string `json:"amount"`
	NewBalance         string `json:"new_balance"`
	Status             string `json:"status"`
	CreatedAt          string `json:"created_at"`
}

// WithdrawalListItem is the per-row shape on GET /withdrawals. Mirrors the
// deposit list contract — flat list, no pagination envelope. `new_balance`
// is omitted because it is only meaningful at the point of writing.
type WithdrawalListItem struct {
	ID                 string `json:"id"`
	Asset              string `json:"asset"`
	NetworkID          string `json:"network_id"`
	DestinationAddress string `json:"destination_address"`
	Amount             string `json:"amount"`
	Status             string `json:"status"`
	CreatedAt          string `json:"created_at"`
}

func toListItem(w *Withdrawal) WithdrawalListItem {
	item := WithdrawalListItem{
		ID:                 w.ID.String(),
		Asset:              w.Asset,
		NetworkID:          w.NetworkID.String(),
		DestinationAddress: w.DestinationAddress,
		Amount:             numeric.String(w.Amount),
		Status:             w.Status,
	}
	if w.CreatedAt != nil {
		item.CreatedAt = w.CreatedAt.Format(time.RFC3339)
	}
	return item
}

func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r.Context())
	if err != nil {
		shared.HTTPError(w, http.StatusUnauthorized, "unauthenticated")
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
	res, err := h.processor.Withdraw(r.Context(), WithdrawCommand{
		UserID:             userID,
		Asset:              req.Asset,
		NetworkName:        req.Network,
		DestinationAddress: req.DestinationAddress,
		Amount:             amount,
		IdempotencyKey:     req.IdempotencyKey,
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
		WithdrawalID:       res.Withdrawal.ID.String(),
		Asset:              res.Withdrawal.Asset,
		NetworkID:          res.Withdrawal.NetworkID.String(),
		DestinationAddress: res.Withdrawal.DestinationAddress,
		Amount:             numeric.String(res.Withdrawal.Amount),
		NewBalance:         res.NewBalance,
		Status:             res.Withdrawal.Status,
		CreatedAt:          createdAt,
	})
}

func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r.Context())
	if err != nil {
		shared.HTTPError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	limit, offset := shared.ParsePagination(r)
	list, err := h.withdrawalService.ListMine(r.Context(), userID, limit, offset)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	resp := make([]WithdrawalListItem, 0, len(list))
	for _, wd := range list {
		resp = append(resp, toListItem(wd))
	}
	shared.WriteJSON(w, http.StatusOK, resp)
}
