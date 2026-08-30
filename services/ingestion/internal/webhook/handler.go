// Package webhook handles incoming GitHub webhook events, validates
// signatures, parses payloads, and extracts code diffs for analysis.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/veerjain-1/code-anomaly-engine/services/ingestion/internal/kafka"
	"github.com/veerjain-1/code-anomaly-engine/services/ingestion/internal/parser"
)

// Handler processes GitHub webhook events.
type Handler struct {
	producer *kafka.Producer
	secret   string
}

// NewHandler creates a new webhook handler.
func NewHandler(producer *kafka.Producer, secret string) *Handler {
	return &Handler{
		producer: producer,
		secret:   secret,
	}
}

// PushEvent represents a GitHub push event payload (subset).
type PushEvent struct {
	Ref        string   `json:"ref"`
	Before     string   `json:"before"`
	After      string   `json:"after"`
	Repository RepoInfo `json:"repository"`
	Commits    []Commit `json:"commits"`
}

// PullRequestEvent represents a GitHub pull_request event payload (subset).
type PullRequestEvent struct {
	Action      string      `json:"action"`
	PullRequest PullRequest `json:"pull_request"`
	Repository  RepoInfo    `json:"repository"`
}

// PullRequest contains PR details.
type PullRequest struct {
	Number   int    `json:"number"`
	Title    string `json:"title"`
	DiffURL  string `json:"diff_url"`
	Head     Branch `json:"head"`
}

// Branch represents a git branch reference.
type Branch struct {
	SHA string `json:"sha"`
	Ref string `json:"ref"`
}

// RepoInfo contains repository metadata.
type RepoInfo struct {
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
}

// Commit represents a single commit in a push event.
type Commit struct {
	ID       string   `json:"id"`
	Message  string   `json:"message"`
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Modified []string `json:"modified"`
	URL      string   `json:"url"`
}

// CodeSnippet is a function-level code extract ready for ML inference.
type CodeSnippet struct {
	Code      string `json:"code"`
	Repo      string `json:"repo"`
	CommitSHA string `json:"commit_sha"`
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Timestamp int64  `json:"timestamp"`
}

// HandleWebhook processes incoming GitHub webhook requests.
func (h *Handler) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("Failed to read request body", "error", err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate webhook signature if secret is configured
	if h.secret != "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if !h.verifySignature(body, signature) {
			slog.Warn("Invalid webhook signature")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	eventType := r.Header.Get("X-GitHub-Event")
	deliveryID := r.Header.Get("X-GitHub-Delivery")

	slog.Info("Received webhook",
		"event", eventType,
		"delivery_id", deliveryID,
	)

	var snippets []CodeSnippet

	switch eventType {
	case "push":
		snippets, err = h.handlePush(body)
	case "pull_request":
		snippets, err = h.handlePullRequest(body)
	case "ping":
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"pong"}`))
		return
	default:
		slog.Info("Ignoring unsupported event type", "event", eventType)
		w.WriteHeader(http.StatusOK)
		return
	}

	if err != nil {
		slog.Error("Failed to process webhook", "event", eventType, "error", err)
		http.Error(w, "Processing error", http.StatusInternalServerError)
		return
	}

	// Publish snippets to Kafka
	published := 0
	for _, snippet := range snippets {
		data, err := json.Marshal(snippet)
		if err != nil {
			slog.Error("Failed to marshal snippet", "error", err)
			continue
		}
		if err := h.producer.Publish(r.Context(), data); err != nil {
			slog.Error("Failed to publish to Kafka", "error", err)
			continue
		}
		published++
	}

	duration := time.Since(start)
	slog.Info("Webhook processed",
		"event", eventType,
		"snippets_extracted", len(snippets),
		"snippets_published", published,
		"duration_ms", duration.Milliseconds(),
	)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"status":    "ok",
		"snippets":  published,
		"latency_ms": duration.Milliseconds(),
	})
}

// handlePush processes a push event and extracts code snippets from commits.
func (h *Handler) handlePush(body []byte) ([]CodeSnippet, error) {
	var event PushEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("unmarshal push event: %w", err)
	}

	var snippets []CodeSnippet
	now := time.Now().UnixMilli()

	for _, commit := range event.Commits {
		// Extract from modified files (most interesting for anomaly detection)
		allFiles := append(commit.Modified, commit.Added...)

		for _, filePath := range allFiles {
			// Only analyze code files
			if !isCodeFile(filePath) {
				continue
			}

			// For push events, we extract the full commit diff later via API
			// For now, create a placeholder snippet that the gateway will enrich
			snippets = append(snippets, CodeSnippet{
				Code:      "", // Will be enriched by gateway via GitHub API
				Repo:      event.Repository.FullName,
				CommitSHA: commit.ID,
				FilePath:  filePath,
				StartLine: 0,
				EndLine:   0,
				Timestamp: now,
			})
		}
	}

	return snippets, nil
}

// handlePullRequest processes a pull request event.
func (h *Handler) handlePullRequest(body []byte) ([]CodeSnippet, error) {
	var event PullRequestEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("unmarshal PR event: %w", err)
	}

	// Only process opened/synchronize actions
	if event.Action != "opened" && event.Action != "synchronize" {
		return nil, nil
	}

	// For PRs, we'll fetch the diff from the diff URL
	// For now, emit a marker snippet that the gateway will enrich
	return []CodeSnippet{{
		Repo:      event.Repository.FullName,
		CommitSHA: event.PullRequest.Head.SHA,
		FilePath:  fmt.Sprintf("PR#%d", event.PullRequest.Number),
		Timestamp: time.Now().UnixMilli(),
	}}, nil
}

// verifySignature validates the HMAC-SHA256 signature from GitHub.
func (h *Handler) verifySignature(payload []byte, signature string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(payload)
	expected := mac.Sum(nil)

	return hmac.Equal(sig, expected)
}

// isCodeFile checks if a file path corresponds to a source code file
// that should be analyzed for defects.
func isCodeFile(path string) bool {
	codeExtensions := []string{
		".c", ".cpp", ".cc", ".h", ".hpp",
		".go", ".rs",
		".py",
		".js", ".ts", ".jsx", ".tsx",
		".java",
	}
	lower := strings.ToLower(path)
	for _, ext := range codeExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// ParseDiff extracts function-level snippets from a unified diff string.
// Exported so it can be called by the gateway for diff enrichment.
func ParseDiff(diff string, repo string, commitSHA string) []CodeSnippet {
	parsed := parser.ExtractSnippets(diff, repo, commitSHA)
	var snippets []CodeSnippet
	for _, p := range parsed {
		snippets = append(snippets, CodeSnippet{
			Code:      p.Code,
			Repo:      p.Repo,
			CommitSHA: p.CommitSHA,
			FilePath:  p.FilePath,
			StartLine: p.StartLine,
			EndLine:   p.EndLine,
			Timestamp: time.Now().UnixMilli(),
		})
	}
	return snippets
}
