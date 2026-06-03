package deposit

import (
	"encoding/json"
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
	depositService Service
}

func NewHandler(svc Service) *Handler { return &Handler{depositService: svc} }

func (h *Handler) AdminRoutes() chi.Router {
	r := chi.NewRouter()
	r.Post("/intake", h.Intake)
	r.Post("/{id}/confirm", h.Confirm)
	r.Post("/{id}/reject", h.Reject)
	return r
}

func (h *Handler) UserRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListMine)
	return r
}

type IntakeRequest struct {
	Address string `json:"address" validate:"required"`
	Asset   string `json:"asset" validate:"required,max=20"`
	Amount  string `json:"amount" validate:"required"`
	TxHash  string `json:"tx_hash" validate:"required,max=255"`
}

func (r IntakeRequest) Validate() error { return validate.Struct(r) }

type DepositResponse struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	Asset       string `json:"asset"`
	Address     string `json:"address"`
	Amount      string `json:"amount"`
	TxHash      string `json:"tx_hash"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	ConfirmedAt string `json:"confirmed_at,omitempty"`
}

func toResponse(d *Deposit) DepositResponse {
	resp := DepositResponse{
		ID: d.ID.String(), UserID: d.UserID.String(), Asset: d.Asset, Address: d.Address,
		Amount: numeric.String(d.Amount), TxHash: d.TxHash, Status: string(d.Status),
	}
	if d.CreatedAt != nil {
		resp.CreatedAt = d.CreatedAt.Format(time.RFC3339)
	}
	if d.ConfirmedAt != nil {
		resp.ConfirmedAt = d.ConfirmedAt.Format(time.RFC3339)
	}
	return resp
}

func (h *Handler) Intake(w http.ResponseWriter, r *http.Request) {
	var req IntakeRequest
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
	d, err := h.depositService.Intake(r.Context(), IntakeCommand{
		Address: req.Address, Asset: req.Asset, Amount: amount, TxHash: req.TxHash,
	})
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusCreated, toResponse(d))
}

func (h *Handler) Confirm(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid id")
		return
	}
	d, err := h.depositService.Confirm(r.Context(), id)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, toResponse(d))
}

func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid id")
		return
	}
	d, err := h.depositService.Reject(r.Context(), id)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, toResponse(d))
}

func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r.Context())
	if err != nil {
		shared.HTTPError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	limit, offset := shared.ParsePagination(r)
	list, err := h.depositService.ListMine(r.Context(), userID, limit, offset)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	resp := make([]DepositResponse, 0, len(list))
	for _, d := range list {
		resp = append(resp, toResponse(d))
	}
	shared.WriteJSON(w, http.StatusOK, resp)
}
