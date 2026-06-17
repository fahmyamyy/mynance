package wallet

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"mynance/internal/auth"
	"mynance/internal/shared"
	"mynance/pkg/validate"
)

type Handler struct {
	walletService Service
}

func NewHandler(svc Service) *Handler {
	return &Handler{walletService: svc}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Create)
	r.Get("/", h.List)
	return r
}

type CreateWalletRequest struct {
	Asset   string `json:"asset" validate:"required,max=20"`
	Network string `json:"network" validate:"required,max=50"`
}

func (r CreateWalletRequest) Validate() error { return validate.Struct(r) }

type WalletResponse struct {
	ID        string `json:"id"`
	Asset     string `json:"asset"`
	NetworkID string `json:"network_id"`
	Address   string `json:"address"`
	CreatedAt string `json:"created_at"`
}

func toResponse(w *WalletAddress) WalletResponse {
	resp := WalletResponse{
		ID:        w.ID.String(),
		Asset:     w.Asset,
		NetworkID: w.NetworkID.String(),
		Address:   w.Address,
	}
	if w.CreatedAt != nil {
		resp.CreatedAt = w.CreatedAt.Format(time.RFC3339)
	}
	return resp
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r.Context())
	if err != nil {
		shared.HTTPError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	var req CreateWalletRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		shared.HTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	addr, err := h.walletService.GetOrCreateAddress(r.Context(), userID, req.Asset, req.Network)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, toResponse(addr))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r.Context())
	if err != nil {
		shared.HTTPError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	list, err := h.walletService.ListMyAddresses(r.Context(), userID)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	resp := make([]WalletResponse, 0, len(list))
	for _, a := range list {
		resp = append(resp, toResponse(a))
	}
	shared.WriteJSON(w, http.StatusOK, resp)
}
