package trade

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
	tradeService Service
}

func NewHandler(
	tradeService Service,
) *Handler {
	return &Handler{
		tradeService: tradeService,
	}
}

func (handler *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", handler.ExecuteTrade)
	return r
}

type ExecuteTradeRequest struct {
	Symbol         string `json:"symbol" validate:"required,max=20"`
	BuyOrderID     string `json:"buy_order_id" validate:"required,uuid"`
	SellOrderID    string `json:"sell_order_id" validate:"required,uuid"`
	Price          string `json:"price" validate:"required"`
	Quantity       string `json:"quantity" validate:"required"`
	IdempotencyKey string `json:"idempotency_key" validate:"required,max=100"`
}

func (r ExecuteTradeRequest) Validate() error {
	return validate.Struct(r)
}

type TradeResponse struct {
	ID          string `json:"id"`
	Symbol      string `json:"symbol"`
	BuyOrderID  string `json:"buy_order_id"`
	SellOrderID string `json:"sell_order_id"`
	BuyUserID   string `json:"buy_user_id"`
	SellUserID  string `json:"sell_user_id"`
	Price       string `json:"price"`
	Quantity    string `json:"quantity"`
	CreatedAt   string `json:"created_at"`
}

func ToTradeResponse(t *Trade) TradeResponse {
	resp := TradeResponse{
		ID:          t.ID.String(),
		Symbol:      t.Symbol,
		BuyOrderID:  t.BuyOrderID.String(),
		SellOrderID: t.SellOrderID.String(),
		BuyUserID:   t.BuyUserID.String(),
		SellUserID:  t.SellUserID.String(),
		Price:       numeric.String(t.Price),
		Quantity:    numeric.String(t.Quantity),
	}
	if t.CreatedAt != nil {
		resp.CreatedAt = t.CreatedAt.Format(time.RFC3339)
	}
	return resp
}

func (handler *Handler) ExecuteTrade(w http.ResponseWriter, r *http.Request) {
	var req ExecuteTradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := req.Validate(); err != nil {
		shared.HTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	buyID, err := uuid.Parse(req.BuyOrderID)
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid buy_order_id")
		return
	}
	sellID, err := uuid.Parse(req.SellOrderID)
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid sell_order_id")
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

	t, err := handler.tradeService.ExecuteTrade(r.Context(), ExecuteTradeCommand{
		Symbol:         req.Symbol,
		BuyOrderID:     buyID,
		SellOrderID:    sellID,
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
	shared.WriteJSON(w, http.StatusCreated, ToTradeResponse(t))
}
