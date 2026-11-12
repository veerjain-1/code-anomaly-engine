// Package ws implements a WebSocket hub for broadcasting real-time
// anomaly detection results to connected dashboard clients.
package ws

import (
	"log/slog"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins for dev
}

// Client represents a connected WebSocket client.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Hub maintains the set of active WebSocket clients and broadcasts
// messages to all connected clients.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	done       chan struct{}
	closeOnce  sync.Once
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		done:       make(chan struct{}),
	}
}

// Run starts the hub's event loop. Must be called in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case <-h.done:
			return
		case client := <-h.register:
			h.mu.Lock()
			select {
			case <-h.done:
				close(client.send)
				if client.conn != nil {
					client.conn.Close()
				}
				h.mu.Unlock()
				return
			default:
			}
			h.clients[client] = true
			h.mu.Unlock()
			slog.Info("WebSocket client connected", "total", h.ClientCount())
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				h.removeClient(client)
			}
			h.mu.Unlock()
			slog.Info("WebSocket client disconnected", "total", h.ClientCount())
		case message := <-h.broadcast:
			// Keep eviction and shutdown under the same lock; sends never block.
			h.mu.Lock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					h.removeClient(client)
				}
			}
			h.mu.Unlock()
		}
	}
}

// Broadcast queues a message, or returns when the hub is shutting down.
func (h *Hub) Broadcast(message []byte) {
	select {
	case <-h.done:
		return
	default:
	}
	// Never hold the client-map lock while waiting for queue capacity.
	select {
	case h.broadcast <- message:
	case <-h.done:
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Close signals the event loop to stop and disconnects all clients.
// It is safe to call repeatedly, concurrently, or before Run.
func (h *Hub) Close() {
	h.closeOnce.Do(func() {
		close(h.done)
		h.mu.Lock()
		defer h.mu.Unlock()
		for client := range h.clients {
			h.removeClient(client)
		}
	})
}

// removeClient requires h.mu to be held for writing and the client to be registered.
func (h *Hub) removeClient(client *Client) {
	delete(h.clients, client)
	close(client.send)
	if client.conn != nil {
		client.conn.Close()
	}
}

// HandleWebSocket upgrades HTTP connections to WebSocket and registers
// the client with the hub.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	select {
	case <-h.done:
		http.Error(w, "WebSocket hub is shutting down", http.StatusServiceUnavailable)
		return
	default:
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("WebSocket upgrade failed", "error", err)
		return
	}
	client := &Client{hub: h, conn: conn, send: make(chan []byte, 256)}
	select {
	case h.register <- client:
	case <-h.done:
		conn.Close()
		return
	}
	unregister := func() {
		conn.Close()
		select {
		case h.unregister <- client:
		case <-h.done:
		}
	}
	// Write pump
	go func() {
		defer unregister()
		for message := range client.send {
			if err := conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}()
	// Read pump (keep alive + handle client disconnect)
	go func() {
		defer unregister()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}
