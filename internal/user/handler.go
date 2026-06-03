package user

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
	userService Service
}

func NewHandler(
	userService Service,
) *Handler {
	return &Handler{
		userService: userService,
	}
}

func (handler *Handler) Routes() chi.Router {
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

func ToUserResponse(u *User) UserResponse {
	var createdAt, updatedAt string
	if u.CreatedAt != nil {
		createdAt = u.CreatedAt.Format(time.RFC3339)
	}
	if u.UpdatedAt != nil {
		updatedAt = u.UpdatedAt.Format(time.RFC3339)
	}
	return UserResponse{
		ID:        u.ID.String(),
		Email:     u.Email,
		Username:  u.Username,
		FullName:  u.FullName,
		Status:    u.Status,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}

func (handler *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		shared.HTTPError(w, http.StatusBadRequest, err.Error())
		return
	}

	u, err := handler.userService.CreateUser(r.Context(), CreateUserCommand{
		Email:    req.Email,
		Username: req.Username,
		FullName: req.FullName,
		Password: req.Password,
	})
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}

	shared.WriteJSON(w, http.StatusCreated, ToUserResponse(u))
}

func (handler *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	u, err := handler.userService.GetUser(r.Context(), id)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}

	shared.WriteJSON(w, http.StatusOK, ToUserResponse(u))
}

func (handler *Handler) ListUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset := shared.ParsePagination(r)

	users, err := handler.userService.ListUsers(r.Context(), limit, offset)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}

	resp := make([]UserResponse, 0, len(users))
	for _, u := range users {
		resp = append(resp, ToUserResponse(u))
	}
	shared.WriteJSON(w, http.StatusOK, resp)
}

func (handler *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid user id")
		return
	}

	if err := handler.userService.DeleteUser(r.Context(), id); err != nil {
		shared.HandleServiceError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
