package marketfeed

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"mynance/internal/shared"
)

type Handler struct {
	client *Client
}

func NewHandler(c *Client) *Handler { return &Handler{client: c} }

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.Markets)
	r.Get("/{symbol}/orderbook", h.OrderBook)
	r.Get("/{symbol}/trades", h.Trades)
	r.Get("/{symbol}/ticker", h.Ticker)
	return r
}

func (h *Handler) Markets(w http.ResponseWriter, r *http.Request) {
	if !h.client.Enabled() {
		shared.HTTPError(w, http.StatusServiceUnavailable, "marketfeed disabled")
		return
	}
	shared.WriteJSON(w, http.StatusOK, h.client.Markets())
}

func (h *Handler) OrderBook(w http.ResponseWriter, r *http.Request) {
	if !h.client.Enabled() {
		shared.HTTPError(w, http.StatusServiceUnavailable, "marketfeed disabled")
		return
	}
	sym := strings.ToUpper(chi.URLParam(r, "symbol"))
	if !h.client.HasSymbol(sym) {
		shared.HTTPError(w, http.StatusNotFound, "symbol not configured")
		return
	}
	snap, ok := h.client.GetOrderBook(sym)
	if !ok {
		shared.HTTPError(w, http.StatusServiceUnavailable, "marketfeed unavailable for "+sym)
		return
	}
	shared.WriteJSON(w, http.StatusOK, snap)
}

func (h *Handler) Trades(w http.ResponseWriter, r *http.Request) {
	if !h.client.Enabled() {
		shared.HTTPError(w, http.StatusServiceUnavailable, "marketfeed disabled")
		return
	}
	sym := strings.ToUpper(chi.URLParam(r, "symbol"))
	if !h.client.HasSymbol(sym) {
		shared.HTTPError(w, http.StatusNotFound, "symbol not configured")
		return
	}
	trades, _ := h.client.GetTrades(sym)
	if trades == nil {
		trades = []TradeSnapshot{}
	}
	shared.WriteJSON(w, http.StatusOK, trades)
}

func (h *Handler) Ticker(w http.ResponseWriter, r *http.Request) {
	if !h.client.Enabled() {
		shared.HTTPError(w, http.StatusServiceUnavailable, "marketfeed disabled")
		return
	}
	sym := strings.ToUpper(chi.URLParam(r, "symbol"))
	if !h.client.HasSymbol(sym) {
		shared.HTTPError(w, http.StatusNotFound, "symbol not configured")
		return
	}
	snap, ok := h.client.GetTicker(sym)
	if !ok {
		shared.HTTPError(w, http.StatusServiceUnavailable, "marketfeed unavailable for "+sym)
		return
	}
	shared.WriteJSON(w, http.StatusOK, snap)
}
