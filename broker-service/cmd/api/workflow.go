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
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	amqp "github.com/rabbitmq/amqp091-go"
)

type SubmissionResult struct {
	Status  int
	Payload jsonResponse
}

type workflowError struct {
	status int
	err    error
}

func (e workflowError) Error() string {
	return e.err.Error()
}

func (e workflowError) Unwrap() error {
	return e.err
}

func downstreamError(err error) error {
	return workflowError{
		status: http.StatusBadGateway,
		err:    err,
	}
}

func statusForWorkflowError(err error) int {
	var workflowErr workflowError
	if errors.As(err, &workflowErr) {
		return workflowErr.status
	}

	return http.StatusBadRequest
}

type SubmissionWorkflow struct {
	Rabbit     *amqp.Connection
	HTTPClient *http.Client
	AuthURL    string
	LoggerURL  string
	MailURL    string
	RPCAddr    string
	GRPCAddr   string
}

func (app *Config) workflow() *SubmissionWorkflow {
	return &SubmissionWorkflow{
		Rabbit:     app.Rabbit,
		HTTPClient: app.HTTPClient,
		AuthURL:    app.AuthURL,
		LoggerURL:  app.LoggerURL,
		MailURL:    app.MailURL,
		RPCAddr:    app.RPCAddr,
		GRPCAddr:   app.GRPCAddr,
	}
}

func (w *SubmissionWorkflow) Broker() SubmissionResult {
	return SubmissionResult{
		Status: http.StatusOK,
		Payload: jsonResponse{
			Error:   false,
			Message: "Hit the broker",
		},
	}
}

func (w *SubmissionWorkflow) Submit(ctx context.Context, request SubmissionRequest) (SubmissionResult, error) {
	if err := request.Validate(); err != nil {
		return SubmissionResult{}, err
	}

	switch request.Action {
	case "auth":
		return w.Authenticate(ctx, request.Auth)
	case "log":
		return w.Log(ctx, request.Log)
	case "mail":
		return w.SendMail(ctx, request.Mail)
	case "log-via-rpc":
		return w.LogViaRPC(ctx, request.Log)
	case "log-event":
		return w.LogViaRabbit(ctx, request.Log)
	default:
		return SubmissionResult{}, errors.New("unknown action")
	}
}

func (w *SubmissionWorkflow) Authenticate(ctx context.Context, input AuthInput) (SubmissionResult, error) {
	if err := validateAuthInput(input); err != nil {
		return SubmissionResult{}, err
	}

	jsonData, err := json.MarshalIndent(input, "", "\t")
	if err != nil {
		return SubmissionResult{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, w.AuthURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return SubmissionResult{}, err
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := w.httpClient().Do(request)
	if err != nil {
		return SubmissionResult{}, downstreamError(err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized {
		return SubmissionResult{
			Status: http.StatusUnauthorized,
			Payload: jsonResponse{
				Error:   true,
				Message: "invalid credentials",
			},
		}, nil
	}

	if response.StatusCode != http.StatusOK {
		return SubmissionResult{}, downstreamError(fmt.Errorf("unexpected status from auth service: %s", response.Status))
	}

	var jsonFromService jsonResponse
	if err := json.NewDecoder(response.Body).Decode(&jsonFromService); err != nil {
		return SubmissionResult{}, err
	}

	if jsonFromService.Error {
		message := jsonFromService.Message
		if message == "" {
			message = "invalid credentials"
		}
		return SubmissionResult{
			Status: http.StatusUnauthorized,
			Payload: jsonResponse{
				Error:   true,
				Message: message,
			},
		}, nil
	}

	return SubmissionResult{
		Status: http.StatusAccepted,
		Payload: jsonResponse{
			Error:   false,
			Message: "Authenticated!",
			Data:    jsonFromService.Data,
		},
	}, nil
}

func (w *SubmissionWorkflow) Log(ctx context.Context, input LogInput) (SubmissionResult, error) {
	if err := validateLogInput(input); err != nil {
		return SubmissionResult{}, err
	}

	jsonData, err := json.MarshalIndent(input, "", "\t")
	if err != nil {
		return SubmissionResult{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, w.LoggerURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return SubmissionResult{}, err
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := w.httpClient().Do(request)
	if err != nil {
		return SubmissionResult{}, downstreamError(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		return SubmissionResult{}, downstreamError(fmt.Errorf("unexpected status from logger service: %s", response.Status))
	}

	return SubmissionResult{
		Status: http.StatusAccepted,
		Payload: jsonResponse{
			Error:   false,
			Message: "logged",
		},
	}, nil
}

func (w *SubmissionWorkflow) SendMail(ctx context.Context, input MailInput) (SubmissionResult, error) {
	if err := validateMailInput(input); err != nil {
		return SubmissionResult{}, err
	}

	jsonData, err := json.MarshalIndent(input, "", "\t")
	if err != nil {
		return SubmissionResult{}, err
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, w.MailURL, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Println(err)
		return SubmissionResult{}, downstreamError(err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := w.httpClient().Do(request)
	if err != nil {
		log.Println(err)
		return SubmissionResult{}, downstreamError(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		err = fmt.Errorf("unexpected status from mail service: %s", response.Status)
		log.Println(err)
		return SubmissionResult{}, downstreamError(err)
	}

	return SubmissionResult{
		Status: http.StatusAccepted,
		Payload: jsonResponse{
			Error:   false,
			Message: fmt.Sprintf("Message sent to %s", input.To),
		},
	}, nil
}

func (w *SubmissionWorkflow) LogViaRabbit(ctx context.Context, input LogInput) (SubmissionResult, error) {
	if err := validateLogInput(input); err != nil {
		return SubmissionResult{}, err
	}

	emitter, err := event.NewEventEmitter(w.Rabbit)
	if err != nil {
		return SubmissionResult{}, downstreamError(err)
	}

	payload := LogInput{
		Name: input.Name,
		Data: input.Data,
	}

	j, err := json.MarshalIndent(&payload, "", "\t")
	if err != nil {
		return SubmissionResult{}, err
	}

	if err := emitter.Push(string(j), "log.INFO"); err != nil {
		return SubmissionResult{}, downstreamError(err)
	}

	return SubmissionResult{
		Status: http.StatusAccepted,
		Payload: jsonResponse{
			Error:   false,
			Message: "logged via RabbitMQ",
		},
	}, nil
}

func (w *SubmissionWorkflow) LogViaRPC(ctx context.Context, input LogInput) (SubmissionResult, error) {
	if err := validateLogInput(input); err != nil {
		return SubmissionResult{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", w.RPCAddr)
	if err != nil {
		return SubmissionResult{}, downstreamError(err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	client := rpc.NewClient(conn)
	defer client.Close()

	rpcPayload := struct {
		Name string
		Data string
	}{
		Name: input.Name,
		Data: input.Data,
	}

	var result string
	if err := client.Call("RPCServer.LogInfo", rpcPayload, &result); err != nil {
		return SubmissionResult{}, downstreamError(err)
	}

	return SubmissionResult{
		Status: http.StatusAccepted,
		Payload: jsonResponse{
			Error:   false,
			Message: result,
		},
	}, nil
}

func (w *SubmissionWorkflow) LogViaGRPC(ctx context.Context, input LogInput) (SubmissionResult, error) {
	if err := validateLogInput(input); err != nil {
		return SubmissionResult{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, w.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		return SubmissionResult{}, downstreamError(err)
	}
	defer conn.Close()

	c := logs.NewLogServiceClient(conn)

	_, err = c.WriteLog(ctx, &logs.LogRequest{
		LogEntry: &logs.Log{
			Name: input.Name,
			Data: input.Data,
		},
	})
	if err != nil {
		return SubmissionResult{}, downstreamError(err)
	}

	return SubmissionResult{
		Status: http.StatusAccepted,
		Payload: jsonResponse{
			Error:   false,
			Message: "logged",
		},
	}, nil
}

func (w *SubmissionWorkflow) httpClient() *http.Client {
	if w.HTTPClient != nil {
		return w.HTTPClient
	}

	return &http.Client{Timeout: 10 * time.Second}
}
