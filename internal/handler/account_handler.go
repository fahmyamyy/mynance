package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mynance/internal/domain"
	"mynance/pkg/validate"
)

type AccountService interface {
	CreateAccount(ctx context.Context, userID uuid.UUID, asset string) (*domain.Account, error)
	GetAccount(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	GetAccountByID(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	ListAccounts(ctx context.Context, userID *uuid.UUID, limit, offset int) ([]*domain.Account, error)
	DeleteAccount(ctx context.Context, id uuid.UUID) error
	GetBalance(ctx context.Context, userID uuid.UUID, asset string) (string, error)
}

type AccountHandler struct {
	accountService AccountService
}

func NewAccountHandler(
	accountService AccountService,
) *AccountHandler {
	return &AccountHandler{
		accountService: accountService,
	}
}

func (handler *AccountHandler) Routes() chi.Router {
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

func ToAccountResponse(account *domain.Account) AccountResponse {
	var createdAt string
	if account.CreatedAt != nil {
		createdAt = account.CreatedAt.Format(time.RFC3339)
	}
	return AccountResponse{
		ID:        account.ID.String(),
		UserID:    account.UserID.String(),
		Asset:     account.Asset,
		CreatedAt: createdAt,
	}
}

func (handler *AccountHandler) CreateAccount(w http.ResponseWriter, r *http.Request) {
	var req CreateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}

	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		httpError(w, http.StatusBadRequest, "invalid user_id")
		return
	}

	account, err := handler.accountService.CreateAccount(r.Context(), userID, req.Asset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, ToAccountResponse(account))
}

func (handler *AccountHandler) GetAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	account, err := handler.accountService.GetAccount(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, ToAccountResponse(account))
}

func (handler *AccountHandler) ListAccounts(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)

	var userID *uuid.UUID
	if raw := r.URL.Query().Get("user_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			httpError(w, http.StatusBadRequest, "invalid user_id")
			return
		}
		userID = &parsed
	}

	accounts, err := handler.accountService.ListAccounts(r.Context(), userID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	resp := make([]AccountResponse, 0, len(accounts))
	for _, a := range accounts {
		resp = append(resp, ToAccountResponse(a))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (handler *AccountHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	if err := handler.accountService.DeleteAccount(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (handler *AccountHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	accountID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "invalid account id")
		return
	}

	account, err := handler.accountService.GetAccountByID(r.Context(), accountID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	balance, err := handler.accountService.GetBalance(r.Context(), account.UserID, account.Asset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, BalanceResponse{Balance: balance})
}
