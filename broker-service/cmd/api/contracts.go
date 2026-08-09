package main

import (
	"errors"
	"strings"
)

type SubmissionRequest struct {
	Action string    `json:"action"`
	Auth   AuthInput `json:"auth,omitempty"`
	Log    LogInput  `json:"log,omitempty"`
	Mail   MailInput `json:"mail,omitempty"`
}

type MailInput struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

type AuthInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LogInput struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

type RequestPayload = SubmissionRequest
type MailPayload = MailInput
type AuthPayload = AuthInput
type LogPayload = LogInput

func (r SubmissionRequest) Validate() error {
	switch r.Action {
	case "auth":
		return validateAuthInput(r.Auth)
	case "log", "log-via-rpc", "log-event":
		return validateLogInput(r.Log)
	case "mail":
		return validateMailInput(r.Mail)
	default:
		return errors.New("unknown action")
	}
}

func validateAuthInput(payload AuthInput) error {
	if strings.TrimSpace(payload.Email) == "" || strings.TrimSpace(payload.Password) == "" {
		return errors.New("email and password are required")
	}

	return nil
}

func validateLogInput(payload LogInput) error {
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Data) == "" {
		return errors.New("name and data are required")
	}

	return nil
}

func validateMailInput(payload MailInput) error {
	if strings.TrimSpace(payload.To) == "" || strings.TrimSpace(payload.Subject) == "" || strings.TrimSpace(payload.Message) == "" {
		return errors.New("to, subject, and message are required")
	}

	return nil
}
