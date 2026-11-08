package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const payloadLimit int64 = 25 << 20

type trackedBody struct {
	io.Reader
	read   int64
	closed bool
}

func (b *trackedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += int64(n)
	return n, err
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errors.New("fixture read error")
}

func TestWebhookPayloadLimit(t *testing.T) {
	for _, tc := range []struct {
		name          string
		size          int64
		contentLength int64
		chunked       bool
		wantStatus    int
	}{
		{"below limit", payloadLimit - 1, payloadLimit - 1, false, http.StatusOK},
		{"at limit", payloadLimit, payloadLimit, false, http.StatusOK},
		{"above limit", payloadLimit + 64, payloadLimit + 64, false, http.StatusRequestEntityTooLarge},
		{"unknown length chunked", payloadLimit + 64, -1, true, http.StatusRequestEntityTooLarge},
		{"underreported length", payloadLimit + 64, 1, false, http.StatusRequestEntityTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: io.LimitReader(zeroReader{}, tc.size)}
			req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
			req.Body = body
			req.ContentLength = tc.contentLength
			if tc.chunked {
				req.TransferEncoding = []string{"chunked"}
			}
			req.Header.Set("X-GitHub-Event", "ping")
			response := httptest.NewRecorder()
			NewHandler(nil, "").HandleWebhook(response, req)

			if response.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, tc.wantStatus)
			}
			if body.read > payloadLimit+1 {
				t.Errorf("consumed %d bytes, want at most %d", body.read, payloadLimit+1)
			}
			if !body.closed {
				t.Error("request body was not closed")
			}
			if tc.wantStatus == http.StatusOK && response.Body.String() != `{"status":"pong"}` {
				t.Errorf("unexpected ping response: %s", response.Body.String())
			}
		})
	}
}

func TestWebhookReadFailureClosesBody(t *testing.T) {
	body := &trackedBody{Reader: failingReader{}}
	req := httptest.NewRequest(http.MethodPost, "/webhook", nil)
	req.Body = body
	response := httptest.NewRecorder()
	NewHandler(nil, "").HandleWebhook(response, req)
	if response.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", response.Code)
	}
	if !body.closed {
		t.Error("body must also close after a read failure")
	}
}

func TestWebhookSignatureBehavior(t *testing.T) {
	payload := `{"zen":"fixture"}`
	secret := "fixture-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	valid := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	for _, tc := range []struct {
		name       string
		signature  string
		wantStatus int
	}{
		{"valid signature", valid, http.StatusOK},
		{"missing signature", "", http.StatusUnauthorized},
		{"invalid signature", "sha256=" + strings.Repeat("0", 64), http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(payload)}
			req := httptest.NewRequest(http.MethodPost, "/webhook", body)
			req.Header.Set("X-GitHub-Event", "ping")
			req.Header.Set("X-Hub-Signature-256", tc.signature)
			response := httptest.NewRecorder()
			NewHandler(nil, secret).HandleWebhook(response, req)
			if response.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, tc.wantStatus)
			}
			if !body.closed {
				t.Error("request body was not closed")
			}
		})
	}
}
