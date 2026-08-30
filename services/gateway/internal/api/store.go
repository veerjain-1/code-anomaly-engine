// Package api provides REST handlers and an in-memory anomaly store
// for the gateway service.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Anomaly represents a detected code anomaly.
type Anomaly struct {
	ID         string  `json:"id"`
	Repo       string  `json:"repo"`
	CommitSHA  string  `json:"commit_sha"`
	FilePath   string  `json:"file_path"`
	StartLine  int     `json:"start_line"`
	EndLine    int     `json:"end_line"`
	Code       string  `json:"code"`
	IsAnomalous bool   `json:"is_anomalous"`
	Confidence float32 `json:"confidence"`
	Label      string  `json:"label"`
	LatencyMs  float32 `json:"latency_ms"`
	Timestamp  int64   `json:"timestamp"`
}

// AnomalyStore is a thread-safe in-memory store for detected anomalies.
type AnomalyStore struct {
	mu        sync.RWMutex
	anomalies []Anomaly
	byID      map[string]*Anomaly
	counter   atomic.Int64
}

// NewAnomalyStore creates a new empty anomaly store.
func NewAnomalyStore() *AnomalyStore {
	return &AnomalyStore{
		anomalies: make([]Anomaly, 0),
		byID:      make(map[string]*Anomaly),
	}
}

// Add inserts a new anomaly into the store.
func (s *AnomalyStore) Add(a Anomaly) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if a.ID == "" {
		a.ID = fmt.Sprintf("anomaly-%d", s.counter.Add(1))
	}
	if a.Timestamp == 0 {
		a.Timestamp = time.Now().UnixMilli()
	}

	s.anomalies = append(s.anomalies, a)
	s.byID[a.ID] = &s.anomalies[len(s.anomalies)-1]
}

// HandleList returns all anomalies, sorted by timestamp descending.
func (s *AnomalyStore) HandleList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Copy and sort by timestamp descending
	result := make([]Anomaly, len(s.anomalies))
	copy(result, s.anomalies)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp > result[j].Timestamp
	})

	// Optional limit
	limit := 100
	if len(result) > limit {
		result = result[:limit]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"anomalies": result,
		"total":     len(s.anomalies),
	})
}

// HandleGet returns a single anomaly by ID.
func (s *AnomalyStore) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	s.mu.RLock()
	defer s.mu.RUnlock()

	anomaly, ok := s.byID[id]
	if !ok {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anomaly)
}

// HandleStats returns aggregated statistics.
func (s *AnomalyStore) HandleStats(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := len(s.anomalies)
	anomalous := 0
	var totalLatency float32

	for _, a := range s.anomalies {
		if a.IsAnomalous {
			anomalous++
		}
		totalLatency += a.LatencyMs
	}

	avgLatency := float32(0)
	if total > 0 {
		avgLatency = totalLatency / float32(total)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"total_analyzed": total,
		"anomalies_found": anomalous,
		"clean":           total - anomalous,
		"anomaly_rate":    float64(anomalous) / float64(max(total, 1)),
		"avg_latency_ms":  avgLatency,
	})
}

// HandleMetrics serves Prometheus-compatible metrics.
func HandleMetrics(w http.ResponseWriter, r *http.Request) {
	// Basic Prometheus text format
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# HELP gateway_up Whether the gateway is running\n")
	fmt.Fprintf(w, "# TYPE gateway_up gauge\n")
	fmt.Fprintf(w, "gateway_up 1\n")
}
