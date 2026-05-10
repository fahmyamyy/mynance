package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mynance/internal/domain"
	"mynance/internal/service"
	"mynance/pkg/validate"
)

type UserService interface {
	CreateUser(ctx context.Context, cmd service.CreateUserCommand) (*domain.User, error)
	GetUser(ctx context.Context, id uuid.UUID) (*domain.User, error)
	ListUsers(ctx context.Context, limit, offset int) ([]*domain.User, error)
	DeleteUser(ctx context.Context, id uuid.UUID) error
}

type UserHandler struct {
	userService UserService
}

func NewUserHandler(
	userService UserService,
) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

func (handler *UserHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", handler.CreateUser)
	r.Get("/", handler.ListUsers)
	r.Get("/{id}", handler.GetUser)
	r.Delete("/{id}", handler.DeleteUser)
	return r
}

type CreateUserRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Username string `json:"username" validate:"required,min=3,max=100"`
	FullName string `json:"full_name" validate:"required,max=255"`
	Password string `json:"password" validate:"required,min=8"`
}

func (r CreateUserRequest) Validate() error {
	return validate.Struct(r)
}

type UserResponse struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	FullName  string `json:"full_name"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func ToUserResponse(user *domain.User) UserResponse {
	var createdAt, updatedAt string
	if user.CreatedAt != nil {
		createdAt = user.CreatedAt.Format(time.RFC3339)
	}
	if user.UpdatedAt != nil {
		updatedAt = user.UpdatedAt.Format(time.RFC3339)
	}
	return UserResponse{
		ID:        user.ID.String(),
		Email:     user.Email,
		Username:  user.Username,
		FullName:  user.FullName,
		Status:    user.Status,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}

func (handler *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := handler.userService.CreateUser(r.Context(), service.CreateUserCommand{
		Email:    req.Email,
		Username: req.Username,
		FullName: req.FullName,
		Password: req.Password,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, ToUserResponse(user))
}

func (handler *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	user, err := handler.userService.GetUser(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, ToUserResponse(user))
}

func (handler *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePagination(r)

	users, err := handler.userService.ListUsers(r.Context(), limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	resp := make([]UserResponse, 0, len(users))
	for _, u := range users {
		resp = append(resp, ToUserResponse(u))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (handler *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		httpError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := handler.userService.DeleteUser(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
