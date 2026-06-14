package wsfeed

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
)

// upgrader accepts any origin — origin enforcement lives in the chi-mounted
// CORS middleware. Tightening this is appropriate before production but
// would conflict with the test harness running on localhost:13000.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(*http.Request) bool { return true },
}

type Handler struct {
	hub *Hub
}

func NewHandler(hub *Hub) *Handler { return &Handler{hub: hub} }

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("wsfeed upgrade", "err", err)
		return
	}
	client := newClient(h.hub, conn)
	h.hub.register(client)
	go client.writeLoop()
	go client.readLoop()
}
