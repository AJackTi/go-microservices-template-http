package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (app *Config) Authenticate(w http.ResponseWriter, r *http.Request) {
	var requestPayload struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	err := app.readJSON(w, r, &requestPayload)
	if err != nil {
		app.errorJSON(w, err, http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(requestPayload.Email) == "" || strings.TrimSpace(requestPayload.Password) == "" {
		app.errorJSON(w, errors.New("email and password are required"), http.StatusBadRequest)
		return
	}

	// validate the user against the database
	email := strings.TrimSpace(requestPayload.Email)
	user, err := app.Models.GetByEmail(email)
	if err != nil {
		app.errorJSON(w, errors.New("invalid credentials"), http.StatusUnauthorized)
		return
	}

	valid, err := user.PasswordMatches(requestPayload.Password)
	if err != nil || !valid {
		app.errorJSON(w, errors.New("invalid credentials"), http.StatusUnauthorized)
		return
	}

	if user.Active != 1 {
		app.errorJSON(w, errors.New("account is inactive"), http.StatusUnauthorized)
		return
	}

	// log authentication
	err = app.logRequest(r.Context(), "authentication", fmt.Sprintf("%s logged in", user.Email))
	if err != nil {
		app.errorJSON(w, err, http.StatusServiceUnavailable)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: fmt.Sprintf("Logged in user %s", user.Email),
		Data:    user,
	}

	app.writeJSON(w, http.StatusOK, payload)
}

func (app *Config) logRequest(ctx context.Context, name, data string) error {
	var entry struct {
		Name string `json:"name"`
		Data string `json:"data"`
	}

	entry.Name = name
	entry.Data = data

	jsonData, err := json.MarshalIndent(entry, "", "\t")
	if err != nil {
		return err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, app.LoggerURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	client := app.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status from logger service: %s", response.Status)
	}

	return nil
}
