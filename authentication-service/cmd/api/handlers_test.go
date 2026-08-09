package main

import (
	"authentication/data"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

type stubUserStore struct {
	gotEmail string
	calls    int32
	user     *data.User
	err      error
}

func (s *stubUserStore) GetByEmail(email string) (*data.User, error) {
	s.gotEmail = email
	atomic.AddInt32(&s.calls, 1)

	if s.err != nil {
		return nil, s.err
	}

	return s.user, nil
}

func TestAuthenticateRejectsBlankCredentials(t *testing.T) {
	t.Parallel()

	store := &stubUserStore{}
	app := &Config{Models: store}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/authenticate", bytes.NewBufferString(`{"email":" ","password":"secret"}`))

	app.Authenticate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}

	if atomic.LoadInt32(&store.calls) != 0 {
		t.Fatal("expected user store not to be called")
	}

	var resp jsonResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Message != "email and password are required" {
		t.Fatalf("unexpected message: %q", resp.Message)
	}
}

func TestAuthenticateReturnsUserAndLogs(t *testing.T) {
	t.Parallel()

	passwordHash := mustPasswordHash(t, "verysecret")
	store := &stubUserStore{
		user: &data.User{
			Email:    "admin@example.com",
			Password: passwordHash,
			Active:   1,
		},
	}

	var logged struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}
	var logCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&logged); err != nil {
			t.Errorf("decode log request: %v", err)
			return
		}

		logCalls.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	app := &Config{
		Models:     store,
		HTTPClient: server.Client(),
		LoggerURL:  server.URL,
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/authenticate", bytes.NewBufferString(`{"email":"  admin@example.com  ","password":"verysecret"}`))

	app.Authenticate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	if atomic.LoadInt32(&store.calls) != 1 {
		t.Fatalf("expected exactly one user lookup, got %d", store.calls)
	}

	if store.gotEmail != "admin@example.com" {
		t.Fatalf("expected trimmed email, got %q", store.gotEmail)
	}

	if logCalls.Load() != 1 {
		t.Fatalf("expected logger to be called once, got %d", logCalls.Load())
	}

	if logged.Name != "authentication" || logged.Data != "admin@example.com logged in" {
		t.Fatalf("unexpected log payload: %#v", logged)
	}

	var resp struct {
		Error   bool           `json:"error"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Error {
		t.Fatal("expected success response")
	}

	if resp.Data["email"] != "admin@example.com" {
		t.Fatalf("unexpected user payload: %#v", resp.Data)
	}
}

func TestAuthenticateRejectsInactiveUser(t *testing.T) {
	t.Parallel()

	passwordHash := mustPasswordHash(t, "verysecret")
	store := &stubUserStore{
		user: &data.User{
			Email:    "admin@example.com",
			Password: passwordHash,
			Active:   0,
		},
	}

	var logCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logCalls.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	app := &Config{
		Models:     store,
		HTTPClient: server.Client(),
		LoggerURL:  server.URL,
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/authenticate", bytes.NewBufferString(`{"email":"admin@example.com","password":"verysecret"}`))

	app.Authenticate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	if logCalls.Load() != 0 {
		t.Fatalf("expected logger not to be called, got %d", logCalls.Load())
	}

	var resp jsonResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Message != "account is inactive" {
		t.Fatalf("unexpected message: %q", resp.Message)
	}
}

func mustPasswordHash(t *testing.T, password string) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	return string(hash)
}
