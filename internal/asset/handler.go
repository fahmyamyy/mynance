package asset

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"mynance/internal/shared"
)

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListAssets)
	r.Get("/{symbol}", h.GetAsset)
	r.Get("/{symbol}/networks", h.ListNetworks)
	return r
}

type AssetResponse struct {
	Symbol        string `json:"symbol"`
	Name          string `json:"name"`
	Decimals      int    `json:"decimals"`
	MinDeposit    string `json:"min_deposit"`
	MinWithdrawal string `json:"min_withdrawal"`
	Enabled       bool   `json:"enabled"`
	CreatedAt     string `json:"created_at,omitempty"`
}

type NetworkResponse struct {
	ID               string `json:"id"`
	AssetSymbol      string `json:"asset_symbol"`
	Name             string `json:"name"`
	ChainID          string `json:"chain_id,omitempty"`
	AddressPattern   string `json:"address_pattern,omitempty"`
	WithdrawalFee    string `json:"withdrawal_fee"`
	MinConfirmations int    `json:"min_confirmations"`
	Enabled          bool   `json:"enabled"`
}

func toAssetResponse(a *Asset) AssetResponse {
	var createdAt string
	if a.CreatedAt != nil {
		createdAt = a.CreatedAt.Format(time.RFC3339)
	}
	return AssetResponse{
		Symbol:        a.Symbol,
		Name:          a.Name,
		Decimals:      a.Decimals,
		MinDeposit:    a.MinDeposit,
		MinWithdrawal: a.MinWithdrawal,
		Enabled:       a.Enabled,
		CreatedAt:     createdAt,
	}
}

func toNetworkResponse(n *Network) NetworkResponse {
	return NetworkResponse{
		ID:               n.ID.String(),
		AssetSymbol:      n.AssetSymbol,
		Name:             n.Name,
		ChainID:          n.ChainID,
		AddressPattern:   n.AddressPattern,
		WithdrawalFee:    n.WithdrawalFee,
		MinConfirmations: n.MinConfirmations,
		Enabled:          n.Enabled,
	}
}

func (h *Handler) ListAssets(w http.ResponseWriter, r *http.Request) {
	assets, err := h.service.ListAssets(r.Context())
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	out := make([]AssetResponse, 0, len(assets))
	for _, a := range assets {
		out = append(out, toAssetResponse(a))
	}
	shared.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) GetAsset(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	a, err := h.service.GetAsset(r.Context(), symbol)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, toAssetResponse(a))
}

func (h *Handler) ListNetworks(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	networks, err := h.service.ListNetworks(r.Context(), symbol)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	out := make([]NetworkResponse, 0, len(networks))
	for _, n := range networks {
		out = append(out, toNetworkResponse(n))
	}
	shared.WriteJSON(w, http.StatusOK, out)
}
