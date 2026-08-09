package main

import (
	"broker/event"
	"broker/logs"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type RequestPayload struct {
	Action string      `json:"action"`
	Auth   AuthPayload `json:"auth,omitempty"`
	Log    LogPayload  `json:"log,omitempty"`
	Mail   MailPayload `json:"mail,omitempty"`
}

type MailPayload struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Message string `json:"message"`
}

type AuthPayload struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LogPayload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func validateAuthPayload(payload AuthPayload) error {
	if strings.TrimSpace(payload.Email) == "" || strings.TrimSpace(payload.Password) == "" {
		return errors.New("email and password are required")
	}

	return nil
}

func validateLogPayload(payload LogPayload) error {
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Data) == "" {
		return errors.New("name and data are required")
	}

	return nil
}

func validateMailPayload(payload MailPayload) error {
	if strings.TrimSpace(payload.To) == "" || strings.TrimSpace(payload.Subject) == "" || strings.TrimSpace(payload.Message) == "" {
		return errors.New("to, subject, and message are required")
	}

	return nil
}

func (p RequestPayload) validate() error {
	switch p.Action {
	case "auth":
		return validateAuthPayload(p.Auth)
	case "log", "log-via-rpc", "log-event":
		return validateLogPayload(p.Log)
	case "mail":
		return validateMailPayload(p.Mail)
	default:
		return errors.New("unknown action")
	}
}

func (app *Config) Broker(w http.ResponseWriter, r *http.Request) {
	payload := jsonResponse{
		Error:   false,
		Message: "Hit the broker",
	}

	_ = app.writeJSON(w, http.StatusOK, payload)
}

func (app *Config) HandleSubmission(w http.ResponseWriter, r *http.Request) {
	var requestPayload RequestPayload

	err := app.readJSON(w, r, &requestPayload)
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	if err := requestPayload.validate(); err != nil {
		app.errorJSON(w, err)
		return
	}

	ctx := r.Context()

	switch requestPayload.Action {
	case "auth":
		app.authenticate(ctx, w, requestPayload.Auth)

	case "log":
		app.logItem(ctx, w, requestPayload.Log)

	case "mail":
		app.sendMail(ctx, w, requestPayload.Mail)

	case "log-via-rpc":
		app.logItemViaRPC(ctx, w, requestPayload.Log)

	case "log-event":
		app.logEventViaRabbit(w, requestPayload.Log)
	}
}

func (app *Config) logItem(ctx context.Context, w http.ResponseWriter, entry LogPayload) {
	jsonData, err := json.MarshalIndent(entry, "", "\t")
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, app.LoggerURL, bytes.NewBuffer(jsonData))
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := app.httpClient().Do(request)
	if err != nil {
		app.errorJSON(w, err)
		return
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		app.errorJSON(w, fmt.Errorf("unexpected status from logger service: %s", response.Status), http.StatusBadGateway)
		return
	}

	var payload jsonResponse
	payload.Error = false
	payload.Message = "logged"

	app.writeJSON(w, http.StatusAccepted, payload)
}

func (app *Config) authenticate(ctx context.Context, w http.ResponseWriter, a AuthPayload) {
	// create some json we'll send to the auth microservice
	jsonData, err := json.MarshalIndent(a, "", "\t")
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	// call the service
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, app.AuthURL, bytes.NewBuffer(jsonData))
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := app.httpClient().Do(request)
	if err != nil {
		app.errorJSON(w, err)
		return
	}
	defer response.Body.Close()

	// make sure we get back the correct status code
	if response.StatusCode == http.StatusUnauthorized {
		app.errorJSON(w, errors.New("invalid credentials"), http.StatusUnauthorized)
		return
	} else if response.StatusCode != http.StatusOK {
		app.errorJSON(w, fmt.Errorf("unexpected status from auth service: %s", response.Status), http.StatusBadGateway)
		return
	}

	// create a variable we'll read response.Body into
	var jsonFromService jsonResponse

	// decode the json from the auth service
	err = json.NewDecoder(response.Body).Decode(&jsonFromService)
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	if jsonFromService.Error {
		message := jsonFromService.Message
		if message == "" {
			message = "invalid credentials"
		}
		app.errorJSON(w, errors.New(message), http.StatusUnauthorized)
		return
	}

	var payload jsonResponse
	payload.Error = false
	payload.Message = "Authenticated!"
	payload.Data = jsonFromService.Data

	app.writeJSON(w, http.StatusAccepted, payload)
}

func (app *Config) sendMail(ctx context.Context, w http.ResponseWriter, msg MailPayload) {
	jsonData, err := json.MarshalIndent(msg, "", "\t")
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	// post to mail service
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, app.MailURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println(err)
		app.errorJSON(w, err)
		return
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := app.httpClient().Do(request)
	if err != nil {
		log.Println(err)
		app.errorJSON(w, err)
		return
	}
	defer response.Body.Close()

	// make sure we get back the right status code
	if response.StatusCode != http.StatusAccepted {
		err = fmt.Errorf("unexpected status from mail service: %s", response.Status)
		log.Println(err)
		app.errorJSON(w, err, http.StatusBadGateway)
		return
	}

	// send back json
	var payload jsonResponse
	payload.Error = false
	payload.Message = fmt.Sprintf("Message sent to %s", msg.To)

	app.writeJSON(w, http.StatusAccepted, payload)
}

func (app *Config) logEventViaRabbit(w http.ResponseWriter, l LogPayload) {
	err := app.pushToQueue(l.Name, l.Data)
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	var payload jsonResponse
	payload.Error = false
	payload.Message = "logged via RabbitMQ"

	app.writeJSON(w, http.StatusAccepted, payload)
}

func (app *Config) pushToQueue(name, msg string) error {
	emitter, err := event.NewEventEmitter(app.Rabbit)
	if err != nil {
		return err
	}

	payload := LogPayload{
		Name: name,
		Data: msg,
	}

	j, err := json.MarshalIndent(&payload, "", "\t")
	if err != nil {
		return err
	}

	return emitter.Push(string(j), "log.INFO")
}

type RPCPayload struct {
	Name string
	Data string
}

func (app *Config) logItemViaRPC(ctx context.Context, w http.ResponseWriter, l LogPayload) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", app.RPCAddr)
	if err != nil {
		app.errorJSON(w, err)
		return
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client := rpc.NewClient(conn)
	defer client.Close()

	rpcPayload := RPCPayload{
		Name: l.Name,
		Data: l.Data,
	}

	var result string
	err = client.Call("RPCServer.LogInfo", rpcPayload, &result)
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	payload := jsonResponse{
		Error:   false,
		Message: result,
	}

	app.writeJSON(w, http.StatusAccepted, payload)
}

func (app *Config) LogViaGRPC(w http.ResponseWriter, r *http.Request) {
	var requestPayload RequestPayload

	err := app.readJSON(w, r, &requestPayload)
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	if err := validateLogPayload(requestPayload.Log); err != nil {
		app.errorJSON(w, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, app.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		app.errorJSON(w, err)
		return
	}
	defer conn.Close()

	c := logs.NewLogServiceClient(conn)

	_, err = c.WriteLog(ctx, &logs.LogRequest{
		LogEntry: &logs.Log{
			Name: requestPayload.Log.Name,
			Data: requestPayload.Log.Data,
		},
	})
	if err != nil {
		app.errorJSON(w, err)
		return
	}

	var payload jsonResponse
	payload.Error = false
	payload.Message = "logged"

	app.writeJSON(w, http.StatusAccepted, payload)
}

func (app *Config) httpClient() *http.Client {
	if app.HTTPClient != nil {
		return app.HTTPClient
	}

	return &http.Client{Timeout: 10 * time.Second}
}
