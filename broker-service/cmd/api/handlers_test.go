package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLogItemPostsPayloadAndReturnsAccepted(t *testing.T) {
	t.Parallel()

	var got LogPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request body: %v", err)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	app := &Config{
		LoggerURL:  server.URL,
		HTTPClient: server.Client(),
	}

	rr := httptest.NewRecorder()
	app.logItem(context.Background(), rr, LogPayload{Name: "log", Data: "payload"})

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, rr.Code)
	}

	if got != (LogPayload{Name: "log", Data: "payload"}) {
		t.Fatalf("unexpected payload: %#v", got)
	}
}

func TestLogItemReturnsBadGatewayWhenLoggerRejects(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	app := &Config{
		LoggerURL:  server.URL,
		HTTPClient: server.Client(),
	}

	rr := httptest.NewRecorder()
	app.logItem(context.Background(), rr, LogPayload{Name: "log", Data: "payload"})

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("expected status %d, got %d", http.StatusBadGateway, rr.Code)
	}
}

func TestHandleSubmissionRejectsUnknownAction(t *testing.T) {
	t.Parallel()

	app := &Config{}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/handle", bytes.NewBufferString(`{"action":"explode"}`))

	app.HandleSubmission(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestLogViaGRPCRejectsInvalidPayload(t *testing.T) {
	t.Parallel()

	app := &Config{GRPCAddr: "127.0.0.1:65535"}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/log-grpc", bytes.NewBufferString(`{"action":"log","log":{}}`))

	app.LogViaGRPC(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}
