package ledger

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mynance/internal/auth"
	"mynance/internal/shared"
	"mynance/pkg/numeric"
)

// LedgerService is the consumer-side interface — only the methods the handler
// actually invokes. Defined here per the project's "interface at consumer"
// convention so handler tests can satisfy it with a tiny fake.
type LedgerService interface {
	ListByUser(ctx context.Context, filter ListFilter) ([]*LedgerEntry, error)
	CountByUser(ctx context.Context, filter ListFilter) (int, error)
}

type Handler struct {
	ledgerService LedgerService
}

func NewHandler(svc LedgerService) *Handler {
	return &Handler{ledgerService: svc}
}

// Routes serves the authenticated user's own ledger.
func (h *Handler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListMine)
	return r
}

// AdminRoutes serves any user's ledger. The user_id query param is required —
// keeps result sets bounded and forces the admin UI to be explicit.
func (h *Handler) AdminRoutes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.AdminList)
	return r
}

// LedgerEntryResponse is the FE-facing shape. Internal account_id is dropped;
// user_id is dropped on /me but kept on the admin variant via a separate
// response (caller already knows the user_id since they passed it).
type LedgerEntryResponse struct {
	ID        string `json:"id"`
	Asset     string `json:"asset"`
	Amount    string `json:"amount"`
	EntryType string `json:"entry_type"`
	RefType   string `json:"ref_type"`
	RefID     string `json:"ref_id"`
	CreatedAt string `json:"created_at"`
}

type LedgerListResponse struct {
	Items  []LedgerEntryResponse `json:"items"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
	Total  int                   `json:"total"`
}

func toLedgerEntryResponse(e *LedgerEntry) LedgerEntryResponse {
	resp := LedgerEntryResponse{
		ID:        e.ID.String(),
		Asset:     e.Asset,
		Amount:    numeric.String(e.Amount),
		EntryType: string(e.EntryType),
		RefType:   string(e.RefType),
		RefID:     e.RefID.String(),
	}
	if e.CreatedAt != nil {
		resp.CreatedAt = e.CreatedAt.Format(time.RFC3339)
	}
	return resp
}

func (h *Handler) ListMine(w http.ResponseWriter, r *http.Request) {
	userID, err := auth.UserIDFromContext(r.Context())
	if err != nil {
		shared.HTTPError(w, http.StatusUnauthorized, "unauthenticated")
		return
	}
	filter, err := parseListFilter(r, userID)
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.respondList(w, r, filter)
}

func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	if !auth.IsAdmin(r.Context()) {
		shared.HTTPError(w, http.StatusForbidden, "admin required")
		return
	}
	raw := r.URL.Query().Get("user_id")
	if raw == "" {
		shared.HTTPError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	userID, err := uuid.Parse(raw)
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	filter, err := parseListFilter(r, userID)
	if err != nil {
		shared.HTTPError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.respondList(w, r, filter)
}

func (h *Handler) respondList(w http.ResponseWriter, r *http.Request, filter ListFilter) {
	entries, err := h.ledgerService.ListByUser(r.Context(), filter)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	total, err := h.ledgerService.CountByUser(r.Context(), filter)
	if err != nil {
		shared.HandleServiceError(w, err)
		return
	}
	items := make([]LedgerEntryResponse, 0, len(entries))
	for _, e := range entries {
		items = append(items, toLedgerEntryResponse(e))
	}
	shared.WriteJSON(w, http.StatusOK, LedgerListResponse{
		Items:  items,
		Limit:  filter.Limit,
		Offset: filter.Offset,
		Total:  total,
	})
}

// validEntryTypes is the closed set the API accepts. Anything outside this
// set is rejected so callers cannot fish for unknown enum values.
var validEntryTypes = map[EntryType]struct{}{
	EntryTypeReserve:  {},
	EntryTypeRelease:  {},
	EntryTypeTrade:    {},
	EntryTypeDeposit:  {},
	EntryTypeWithdraw: {},
}

func parseListFilter(r *http.Request, userID uuid.UUID) (ListFilter, error) {
	q := r.URL.Query()
	limit, offset := shared.ParsePagination(r)
	filter := ListFilter{
		UserID: userID,
		Asset:  strings.TrimSpace(q.Get("asset")),
		Limit:  limit,
		Offset: offset,
	}
	if raw := q.Get("entry_type"); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(strings.ToUpper(part))
			if part == "" {
				continue
			}
			et := EntryType(part)
			if _, ok := validEntryTypes[et]; !ok {
				return ListFilter{}, errBadEntryType(part)
			}
			filter.EntryTypes = append(filter.EntryTypes, et)
		}
	}
	if raw := q.Get("from"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return ListFilter{}, errBadTime("from")
		}
		filter.From = &t
	}
	if raw := q.Get("to"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return ListFilter{}, errBadTime("to")
		}
		filter.To = &t
	}
	return filter, nil
}

type filterError struct{ msg string }

func (e filterError) Error() string { return e.msg }

func errBadEntryType(v string) error { return filterError{msg: "invalid entry_type: " + v} }
func errBadTime(field string) error  { return filterError{msg: "invalid " + field + " (expected RFC3339)"} }
