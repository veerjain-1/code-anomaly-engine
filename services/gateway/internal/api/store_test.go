package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type stalledWriter struct {
	*httptest.ResponseRecorder
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *stalledWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return w.ResponseRecorder.Write(p)
}

func TestHandlersDoNotBlockIngestionWhileWriting(t *testing.T) {
	for _, name := range []string{"list", "get", "missing", "stats"} {
		t.Run(name, func(t *testing.T) {
			store := NewAnomalyStore()
			store.Add(Anomaly{ID: "first", Timestamp: 1})
			handlers := map[string]http.HandlerFunc{
				"list": store.HandleList, "get": store.HandleGet, "missing": store.HandleGet, "stats": store.HandleStats,
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.SetPathValue("id", "first")
			if name == "missing" {
				req.SetPathValue("id", "missing")
			}
			writer := &stalledWriter{
				ResponseRecorder: httptest.NewRecorder(),
				started:          make(chan struct{}),
				release:          make(chan struct{}),
			}
			handled := make(chan struct{})
			go func() {
				defer close(handled)
				handlers[name](writer, req)
			}()
			defer func() { close(writer.release); <-handled }()
			select {
			case <-writer.started:
			case <-time.After(5 * time.Second):
				t.Fatal("handler never started writing")
			}

			added := make(chan struct{})
			go func() {
				store.Add(Anomaly{ID: "second", Timestamp: 2})
				close(added)
			}()
			// The writer remains stalled until cleanup. Ingestion must not wait for it.
			select {
			case <-added:
			case <-time.After(time.Second):
				t.Error("slow response writer blocked anomaly ingestion")
			}
		})
	}
}

func TestHandleListKeepsSortedLimitAndTotal(t *testing.T) {
	store := NewAnomalyStore()
	for i := 1; i <= 105; i++ {
		store.Add(Anomaly{Timestamp: int64(i)})
	}
	w := httptest.NewRecorder()
	store.HandleList(w, httptest.NewRequest(http.MethodGet, "/api/anomalies", nil))
	var result struct {
		Anomalies []Anomaly `json:"anomalies"`
		Total     int       `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 105 || len(result.Anomalies) != 100 {
		t.Fatalf("total=%d returned=%d", result.Total, len(result.Anomalies))
	}
	for i, a := range result.Anomalies {
		if a.Timestamp != int64(105-i) {
			t.Fatalf("unexpected timestamp at index %d: %d", i, a.Timestamp)
		}
	}
}
