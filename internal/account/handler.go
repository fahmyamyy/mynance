package account

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mynance/internal/shared"
	"mynance/pkg/validate"
)

type Handler struct {
	accountService Service
}

func NewHandler(
	accountService Service,
) *Handler {
	return &Handler{
		accountService: accountService,
	}
}

func (handler *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", handler.CreateAccount)
	r.Get("/", handler.ListAccounts)
	r.Get("/{id}", handler.GetAccount)
	r.Delete("/{id}", handler.DeleteAccount)
	r.Get("/{id}/balance", handler.GetBalance)
	return r
}

type CreateAccountRequest struct {
	UserID string `json:"user_id" validate:"required,uuid"`
	Asset  string `json:"asset" validate:"required,max=20"`
}

func (r CreateAccountRequest) Validate() error {
	return validate.Struct(r)
}

type AccountResponse struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Asset     string `json:"asset"`
	CreatedAt string `json:"created_at"`
}

type BalanceResponse struct {
	Balance string `json:"balance"`
}

func ToAccountResponse(acct *Account) AccountResponse {
	var createdAt string
	if acct.CreatedAt != nil {
		createdAt = acct.CreatedAt.Format(time.RFC3339)
	}
	return AccountResponse{
		ID:        acct.ID.String(),
		UserID:    acct.UserID.String(),
		Asset:     acct.Asset,
		CreatedAt: createdAt,
	}
}

func (handler *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		shared.HTTPError(w, http.StatusBadRequest, err.Error())
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	acct, err := handler.accountService.CreateAccount(r.Context(), userID, req.Asset)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}

	shared.WriteJSON(w, http.StatusCreated, ToAccountResponse(acct))
}

func (handler *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	acct, err := handler.accountService.GetAccount(r.Context(), id)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}

	shared.WriteJSON(w, http.StatusOK, ToAccountResponse(acct))
}

func (handler *Handler) ListAccounts(w http.ResponseWriter, r *http.Request) {
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
		resp = append(resp, ToAccountResponse(a))
	}
	shared.WriteJSON(w, http.StatusOK, resp)
}

func (handler *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	if err := handler.accountService.DeleteAccount(r.Context(), id); err != nil {
		shared.HandleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	acct, err := handler.accountService.GetAccountByID(r.Context(), accountID)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}

	balance, err := handler.accountService.GetBalance(r.Context(), acct.UserID, acct.Asset)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}

	shared.WriteJSON(w, http.StatusOK, BalanceResponse{Balance: balance})
}
