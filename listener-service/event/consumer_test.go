package event

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestLogEventPostsPayload(t *testing.T) {
	t.Parallel()

	var got Payload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}

		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	consumer := &Consumer{
		loggerURL: server.URL,
		client:    &http.Client{Timeout: 10 * time.Second},
	}

	if err := consumer.logEvent(Payload{Name: "log", Data: "payload"}); err != nil {
		t.Fatalf("logEvent returned error: %v", err)
	}

	if got != (Payload{Name: "log", Data: "payload"}) {
		t.Fatalf("unexpected payload: %#v", got)
	}
}

func TestLogEventReturnsErrorOnUnexpectedStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	consumer := &Consumer{
		loggerURL: server.URL,
		client:    server.Client(),
	}

	if err := consumer.logEvent(Payload{Name: "log", Data: "payload"}); err == nil {
		t.Fatal("expected error")
	}
}
