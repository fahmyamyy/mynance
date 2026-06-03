package order

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mynance/internal/shared"
	"mynance/pkg/numeric"
	"mynance/pkg/validate"
)

type Handler struct {
	orderService Service
}

func NewHandler(
	orderService Service,
) *Handler {
	return &Handler{
		orderService: orderService,
	}
}

func (handler *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", handler.PlaceOrder)
	r.Get("/{id}", handler.GetOrder)
	r.Delete("/{id}", handler.CancelOrder)
	return r
}

type PlaceOrderRequest struct {
	UserID         string `json:"user_id" validate:"required,uuid"`
	Symbol         string `json:"symbol" validate:"required,max=20"`
	Side           string `json:"side" validate:"required,oneof=BUY SELL"`
	Price          string `json:"price" validate:"required"`
	Quantity       string `json:"quantity" validate:"required"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,max=100"`
}

func (r PlaceOrderRequest) Validate() error {
	return validate.Struct(r)
}

type OrderResponse struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	Symbol         string `json:"symbol"`
	Side           string `json:"side"`
	Price          string `json:"price"`
	Quantity       string `json:"quantity"`
	FilledQuantity string `json:"filled_quantity"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

func ToOrderResponse(o *Order) OrderResponse {
	resp := OrderResponse{
		ID:             o.ID.String(),
		UserID:         o.UserID.String(),
		Symbol:         o.Symbol,
		Side:           string(o.Side),
		Price:          numeric.String(o.Price),
		Quantity:       numeric.String(o.Quantity),
		FilledQuantity: numeric.String(o.FilledQuantity),
		Status:         string(o.Status),
	}
	if o.CreatedAt != nil {
		resp.CreatedAt = o.CreatedAt.Format(time.RFC3339)
	}
	if o.UpdatedAt != nil {
		resp.UpdatedAt = o.UpdatedAt.Format(time.RFC3339)
	}
	return resp
}

func (handler *Handler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	var req PlaceOrderRequest
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
	price, err := numeric.Parse(req.Price)
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid price")
		return
	}
	qty, err := numeric.Parse(req.Quantity)
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid quantity")
		return
	}

	o, err := handler.orderService.PlaceOrder(r.Context(), PlaceOrderCommand{
		UserID:         userID,
		Symbol:         req.Symbol,
		Side:           Side(req.Side),
		Price:          price,
		Quantity:       qty,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		if errors.Is(err, shared.ErrDuplicateIdempotencyKey) {
			w.WriteHeader(http.StatusOK)
			return
		}
		shared.HandleServiceError(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusCreated, ToOrderResponse(o))
}

func (handler *Handler) GetOrder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid order id")
		return
	}
	o, err := handler.orderService.GetOrder(r.Context(), id)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, ToOrderResponse(o))
}

func (handler *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid order id")
		return
	}
	o, err := handler.orderService.CancelOrder(r.Context(), id)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	shared.WriteJSON(w, http.StatusOK, ToOrderResponse(o))
}

func (handler *Handler) ListByUser(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	limit, offset := shared.ParsePagination(r)
	orders, err := handler.orderService.ListOrders(r.Context(), userID, limit, offset)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	resp := make([]OrderResponse, 0, len(orders))
	for _, o := range orders {
		resp = append(resp, ToOrderResponse(o))
	}
	shared.WriteJSON(w, http.StatusOK, resp)
}
