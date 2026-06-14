package account

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mynance/internal/auth"
	"mynance/internal/shared"
)

// Handler now serves the FE-facing /me/assets surface: a per-asset portfolio
// view of the authenticated user. Asset rows are auto-provisioned on signup,
// so there is no Create endpoint. List/Get/Balance are keyed by asset symbol
// rather than the internal account UUID — the FE never needs to see those.
type Handler struct {
	accountService Service
}

func NewHandler(accountService Service) *Handler {
	return &Handler{accountService: accountService}
}

func (handler *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", handler.ListMyAssets)
	r.Get("/{symbol}", handler.GetMyAsset)
	r.Get("/{symbol}/balance", handler.GetMyBalance)
	return r
}

// AdminRoutes exposes the same lookups but accepts an optional user_id query
// param so an admin can inspect any user's portfolio. Mounted separately.
func (handler *Handler) AdminRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", handler.adminListAccounts)
	return r
}

type AccountResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Asset     string `json:"asset"`
	Balance   string `json:"balance"`
	CreatedAt string `json:"created_at"`
}

type BalanceResponse struct {
	Asset   string `json:"asset"`
	Balance string `json:"balance"`
}

func ToAccountResponse(acct *Account, balance string) AccountResponse {
	var createdAt string
	if acct.CreatedAt != nil {
		createdAt = acct.CreatedAt.Format(time.RFC3339)
	}
	return AccountResponse{
		ID:        acct.ID.String(),
		UserID:    acct.UserID.String(),
		Asset:     acct.Asset,
		Balance:   balance,
		CreatedAt: createdAt,
	}
}

func (handler *Handler) ListMyAssets(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r.Context())
	if err != nil {
		shared.HTTPError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	limit, offset := shared.ParsePagination(r)
	accounts, err := handler.accountService.ListAccounts(r.Context(), &userID, limit, offset)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	resp := make([]AccountResponse, 0, len(accounts))
	for _, a := range accounts {
		bal, _ := handler.accountService.GetBalance(r.Context(), a.UserID, a.Asset)
		resp = append(resp, ToAccountResponse(a, bal))
	}
	shared.WriteJSON(w, http.StatusOK, resp)
}

func (handler *Handler) GetMyAsset(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r.Context())
	if err != nil {
		shared.HTTPError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	symbol := chi.URLParam(r, "symbol")
	acctID, err := handler.accountService.AccountID(r.Context(), userID, symbol)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	acct, err := handler.accountService.GetAccount(r.Context(), acctID)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	bal, _ := handler.accountService.GetBalance(r.Context(), userID, symbol)
	shared.WriteJSON(w, http.StatusOK, ToAccountResponse(acct, bal))
}

func (handler *Handler) GetMyBalance(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r.Context())
	if err != nil {
		shared.HTTPError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	symbol := chi.URLParam(r, "symbol")
	bal, err := handler.accountService.GetBalance(r.Context(), userID, symbol)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, BalanceResponse{Asset: symbol, Balance: bal})
}

func (handler *Handler) adminListAccounts(w http.ResponseWriter, r *http.Request) {
	if !auth.IsAdmin(r.Context()) {
		shared.HTTPError(w, http.StatusForbidden, "admin required")
		return
	}
	limit, offset := shared.ParsePagination(r)
	var userID *uuid.UUID
	if raw := r.URL.Query().Get("user_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			shared.HTTPError(w, http.StatusBadRequest, "invalid user_id")
			return
		}
		userID = &parsed
	}
	accounts, err := handler.accountService.ListAccounts(r.Context(), userID, limit, offset)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	resp := make([]AccountResponse, 0, len(accounts))
	for _, a := range accounts {
		bal, _ := handler.accountService.GetBalance(r.Context(), a.UserID, a.Asset)
		resp = append(resp, ToAccountResponse(a, bal))
	}
	shared.WriteJSON(w, http.StatusOK, resp)
}
