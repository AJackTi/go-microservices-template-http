package main

import (
	"context"
	"net/http"
)

func (app *Config) Broker(w http.ResponseWriter, r *http.Request) {
	result := app.workflow().Broker()
	_ = app.writeJSON(w, result.Status, result.Payload)
}

func (app *Config) HandleSubmission(w http.ResponseWriter, r *http.Request) {
	var requestPayload SubmissionRequest

	if err := app.readJSON(w, r, &requestPayload); err != nil {
		app.errorJSON(w, err)
		return
	}

	result, err := app.workflow().Submit(r.Context(), requestPayload)
	if err != nil {
		app.errorJSON(w, err, statusForWorkflowError(err))
		return
	}

	_ = app.writeJSON(w, result.Status, result.Payload)
}

func (app *Config) logItem(ctx context.Context, w http.ResponseWriter, entry LogPayload) {
	result, err := app.workflow().Log(ctx, entry)
	if err != nil {
		app.errorJSON(w, err, statusForWorkflowError(err))
		return
	}

	_ = app.writeJSON(w, result.Status, result.Payload)
}

func (app *Config) authenticate(ctx context.Context, w http.ResponseWriter, a AuthPayload) {
	result, err := app.workflow().Authenticate(ctx, a)
	if err != nil {
		app.errorJSON(w, err, statusForWorkflowError(err))
		return
	}

	_ = app.writeJSON(w, result.Status, result.Payload)
}

func (app *Config) sendMail(ctx context.Context, w http.ResponseWriter, msg MailPayload) {
	result, err := app.workflow().SendMail(ctx, msg)
	if err != nil {
		app.errorJSON(w, err, statusForWorkflowError(err))
		return
	}

	_ = app.writeJSON(w, result.Status, result.Payload)
}

func (app *Config) logEventViaRabbit(w http.ResponseWriter, l LogPayload) {
	result, err := app.workflow().LogViaRabbit(context.Background(), l)
	if err != nil {
		app.errorJSON(w, err, statusForWorkflowError(err))
		return
	}

	_ = app.writeJSON(w, result.Status, result.Payload)
}

func (app *Config) logItemViaRPC(ctx context.Context, w http.ResponseWriter, l LogPayload) {
	result, err := app.workflow().LogViaRPC(ctx, l)
	if err != nil {
		app.errorJSON(w, err, statusForWorkflowError(err))
		return
	}

	_ = app.writeJSON(w, result.Status, result.Payload)
}

func (app *Config) LogViaGRPC(w http.ResponseWriter, r *http.Request) {
	var requestPayload SubmissionRequest

	if err := app.readJSON(w, r, &requestPayload); err != nil {
		app.errorJSON(w, err)
		return
	}

	result, err := app.workflow().LogViaGRPC(r.Context(), requestPayload.Log)
	if err != nil {
		app.errorJSON(w, err, statusForWorkflowError(err))
		return
	}

	_ = app.writeJSON(w, result.Status, result.Payload)
}
