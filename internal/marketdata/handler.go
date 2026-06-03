package marketdata

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"mynance/internal/shared"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) OrderBookRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/{symbol}", h.GetOrderBook)
	return r
}

func (h *Handler) TradesRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/{symbol}", h.GetRecentTrades)
	return r
}

func (h *Handler) GetOrderBook(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	shared.WriteJSON(w, http.StatusOK, h.service.GetOrderBook(symbol))
}

func (h *Handler) GetRecentTrades(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	shared.WriteJSON(w, http.StatusOK, h.service.GetRecentTrades(symbol))
}
