package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type LogEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]struct{}
}

func New() *Hub {
	return &Hub{clients: make(map[*websocket.Conn]struct{})}
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] upgrade: %v", err)
		return
	}
	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()

	// keepalive: read messages (ignore them) to detect disconnection
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				h.mu.Lock()
				delete(h.clients, conn)
				h.mu.Unlock()
				conn.Close()
				return
			}
		}
	}()
}

func (h *Hub) Broadcast(evt LogEvent) {
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for conn := range h.clients {
		conn.WriteMessage(websocket.TextMessage, data)
	}
}

func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
