package event

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestLogEventPostsPayload(t *testing.T) {
	t.Parallel()

	var got Payload
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

	consumer := &Consumer{
		loggerURL: server.URL,
		client:    &http.Client{Timeout: 10 * time.Second},
	}

	if err := consumer.logEvent(context.Background(), Payload{Name: "log", Data: "payload"}); err != nil {
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

	if err := consumer.logEvent(context.Background(), Payload{Name: "log", Data: "payload"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestHandleDeliveryAcksSuccessfulPayload(t *testing.T) {
	t.Parallel()

	var got Payload
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

	acknowledger := &recordingAcknowledger{}
	consumer := &Consumer{
		loggerURL: server.URL,
		client:    server.Client(),
	}

	err := consumer.handleDelivery(context.Background(), amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  7,
		Body:         []byte(`{"name":"log","data":"payload"}`),
	})
	if err != nil {
		t.Fatalf("handleDelivery returned error: %v", err)
	}

	if got != (Payload{Name: "log", Data: "payload"}) {
		t.Fatalf("unexpected payload: %#v", got)
	}

	if acknowledger.ackedTag != 7 || acknowledger.nackedTag != 0 || acknowledger.rejectedTag != 0 {
		t.Fatalf("unexpected acknowledgements: %#v", acknowledger)
	}
}

func TestHandleDeliveryNacksWhenLoggerFails(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	acknowledger := &recordingAcknowledger{}
	consumer := &Consumer{
		loggerURL: server.URL,
		client:    server.Client(),
	}

	err := consumer.handleDelivery(context.Background(), amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  9,
		Body:         []byte(`{"name":"log","data":"payload"}`),
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if acknowledger.nackedTag != 9 || !acknowledger.nackedRequeue {
		t.Fatalf("expected delivery to be nacked and requeued, got %#v", acknowledger)
	}

	if acknowledger.ackedTag != 0 || acknowledger.rejectedTag != 0 {
		t.Fatalf("unexpected acknowledgements: %#v", acknowledger)
	}
}

func TestHandleDeliveryRejectsMalformedPayload(t *testing.T) {
	t.Parallel()

	acknowledger := &recordingAcknowledger{}
	consumer := &Consumer{}

	err := consumer.handleDelivery(context.Background(), amqp.Delivery{
		Acknowledger: acknowledger,
		DeliveryTag:  11,
		Body:         []byte(`{"name":`),
	})
	if err == nil {
		t.Fatal("expected error")
	}

	if acknowledger.rejectedTag != 11 || acknowledger.rejectedRequeue {
		t.Fatalf("expected malformed delivery to be rejected without requeue, got %#v", acknowledger)
	}

	if acknowledger.ackedTag != 0 || acknowledger.nackedTag != 0 {
		t.Fatalf("unexpected acknowledgements: %#v", acknowledger)
	}
}

type recordingAcknowledger struct {
	mu              sync.Mutex
	ackedTag        uint64
	ackedMultiple   bool
	nackedTag       uint64
	nackedMultiple  bool
	nackedRequeue   bool
	rejectedTag     uint64
	rejectedRequeue bool
}

func (r *recordingAcknowledger) Ack(tag uint64, multiple bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ackedTag = tag
	r.ackedMultiple = multiple
	return nil
}

func (r *recordingAcknowledger) Nack(tag uint64, multiple, requeue bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nackedTag = tag
	r.nackedMultiple = multiple
	r.nackedRequeue = requeue
	return nil
}

func (r *recordingAcknowledger) Reject(tag uint64, requeue bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rejectedTag = tag
	r.rejectedRequeue = requeue
	return nil
}
