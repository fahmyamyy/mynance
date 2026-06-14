package klines

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"mynance/internal/shared"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.Fetch)
	return r
}

func (h *Handler) Fetch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	symbol := q.Get("symbol")
	interval := q.Get("interval")
	if symbol == "" || interval == "" {
		shared.HTTPError(w, http.StatusBadRequest, "symbol and interval are required")
		return
	}

	startTime, _ := strconv.ParseInt(q.Get("start_time"), 10, 64)
	endTime, _ := strconv.ParseInt(q.Get("end_time"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("limit"))

	out, err := h.service.Fetch(r.Context(), Query{
		Symbol:    symbol,
		Interval:  interval,
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     limit,
	})
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	shared.WriteJSON(w, http.StatusOK, out)
}
