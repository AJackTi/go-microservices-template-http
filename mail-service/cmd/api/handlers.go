package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
)

func (app *Config) SendMail(w http.ResponseWriter, r *http.Request) {
	var requestPayload SendMailRequest

	err := app.readJSON(w, r, &requestPayload)
	if err != nil {
		log.Println(err)
		app.errorJSON(w, err)
		return
	}

	if strings.TrimSpace(requestPayload.To) == "" || strings.TrimSpace(requestPayload.Subject) == "" || strings.TrimSpace(requestPayload.Message) == "" {
		app.errorJSON(w, errors.New("to, subject, and message are required"))
		return
	}

	msg := Message{
		From:    requestPayload.From,
		To:      requestPayload.To,
		Subject: requestPayload.Subject,
		Data:    requestPayload.Message,
	}

	err = app.Mailer.SendSMTPMessage(msg)
	if err != nil {
		log.Println(err)
		app.errorJSON(w, err, http.StatusBadGateway)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: fmt.Sprintf("sent to %s", requestPayload.To),
	}

	app.writeJSON(w, http.StatusAccepted, payload)
}
