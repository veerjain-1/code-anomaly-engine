package ws

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func waitFor(t *testing.T, done <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal(message)
	}
}

// A no-op unregister event is a barrier: Run must finish handling the previous
// registration before it can receive another event. Sending on register alone
// only proves that Run received the client, not that it updated the map yet.
func syncHub(t *testing.T, h *Hub) {
	t.Helper()
	select {
	case h.unregister <- &Client{}:
	case <-time.After(3 * time.Second):
		t.Fatal("hub did not process the event barrier")
	}
}

func TestCloseStopsRun(t *testing.T) {
	for _, before := range []bool{false, true} {
		name := "after-start"
		if before {
			name = "before-start"
		}
		t.Run(name, func(t *testing.T) {
			h := NewHub()
			done := make(chan struct{})
			if before {
				h.Close()
			}
			go func() { h.Run(); close(done) }()
			if !before {
				syncHub(t, h)
			}
			h.Close()
			h.Close()
			waitFor(t, done, "Run did not return after Close")
			h.Broadcast([]byte("ignored after close"))
		})
	}
}

func TestCloseUnblocksFullBroadcastQueue(t *testing.T) {
	h := NewHub()
	for i := 0; i < cap(h.broadcast); i++ {
		h.Broadcast([]byte("queued"))
	}
	sent := make(chan struct{})
	go func() { h.Broadcast([]byte("overflow")); close(sent) }()
	closed := make(chan struct{})
	go func() { h.Close(); close(closed) }()
	// On a broken implementation, starting Run during cleanup drains the queue,
	// releasing the blocked sender and Close instead of leaving both stuck.
	defer func() { go h.Run() }()
	waitFor(t, closed, "Close blocked behind a full broadcast queue")
	waitFor(t, sent, "Broadcast remained blocked after Close")
}

func TestCloseDisconnectsClientAndRejectsNewConnections(t *testing.T) {
	h := NewHub()
	done := make(chan struct{})
	go func() { h.Run(); close(done) }()
	defer h.Close()
	registered := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.HandleWebSocket(w, r)
		once.Do(func() { close(registered) })
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	waitFor(t, registered, "websocket handler did not finish registration")
	syncHub(t, h)
	if h.ClientCount() != 1 {
		t.Fatal("client was not registered")
	}
	h.Broadcast([]byte("hello"))
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, message, err := conn.ReadMessage(); err != nil || string(message) != "hello" {
		t.Fatalf("broadcast message=%q error=%v", message, err)
	}
	h.Close()
	waitFor(t, done, "Run did not exit with an active client")
	if h.ClientCount() != 0 {
		t.Fatal("closed hub retained a client")
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Fatal("client connection remained open")
	}
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("client connection was not closed promptly")
	}
	w := httptest.NewRecorder()
	h.HandleWebSocket(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("closed hub returned HTTP %d, want 503", w.Code)
	}
}

func TestConcurrentBroadcastAndClose(t *testing.T) {
	for i := 0; i < 30; i++ {
		h := NewHub()
		runDone := make(chan struct{})
		go func() { h.Run(); close(runDone) }()
		// A stalled recipient exercises buffer eviction while Close is competing.
		client := &Client{hub: h, send: make(chan []byte, 1)}
		h.register <- client
		var wg sync.WaitGroup
		for j := 0; j < 8; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for k := 0; k < 64; k++ {
					h.Broadcast([]byte("message"))
				}
			}()
		}
		for j := 0; j < 3; j++ {
			wg.Add(1)
			go func() { defer wg.Done(); h.Close() }()
		}
		allDone := make(chan struct{})
		go func() { wg.Wait(); close(allDone) }()
		waitFor(t, allDone, "concurrent hub operations did not finish")
		waitFor(t, runDone, "concurrent shutdown did not stop Run")
		if h.ClientCount() != 0 {
			t.Fatal("shutdown retained clients")
		}
	}
}

func TestRegistrationRacesShutdown(t *testing.T) {
	for i := 0; i < 10; i++ {
		h := NewHub()
		runDone := make(chan struct{})
		go func() { h.Run(); close(runDone) }()
		server := httptest.NewServer(http.HandlerFunc(h.HandleWebSocket))
		start := make(chan struct{})
		var wg sync.WaitGroup
		for j := 0; j < 8; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
				conn, response, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
				if response != nil && response.Body != nil {
					response.Body.Close()
				}
				if err == nil {
					conn.Close()
				}
			}()
		}
		close(start)
		h.Close()
		clientsDone := make(chan struct{})
		go func() { wg.Wait(); close(clientsDone) }()
		waitFor(t, clientsDone, "registration/disconnection callers did not finish")
		waitFor(t, runDone, "registration race did not stop Run")
		server.Close()
		if h.ClientCount() != 0 {
			t.Fatal("registration race retained a client after shutdown")
		}
	}
}
