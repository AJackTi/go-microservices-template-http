package main

import (
	"context"
	"errors"
	"log"
	"log-service/data"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RPCServer is the type for our RPC Server. Methods that take this as a receiver are avaiable
// over RPC, as long as they are exported.
type RPCServer struct {
}

// RPCPayload is the type for data we receive from RPC
type RPCPayload struct {
	Name        string
	Data        string
	TraceParent string
	TraceState  string
	Baggage     string
}

// LogInfo writes our payload to mongo
func (r *RPCServer) LogInfo(payload RPCPayload, resp *string) error {
	ctx := extractTraceContext(context.Background(), payload)
	ctx, span := otel.Tracer("logger-service").Start(ctx, "logger.rpc.log")
	defer span.End()

	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Data) == "" {
		err := errors.New("name and data are required")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}

	if shouldFailRequest("rpc", payload.Name+" "+payload.Data) {
		err := status.Error(grpcCodes.Unavailable, "forced failure for rpc")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	collection := client.Database("logs").Collection("logs")
	_, err := collection.InsertOne(ctx, data.LogEntry{
		Name:      payload.Name,
		Data:      payload.Data,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		log.Println("error writing to mongo", err)
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}

	*resp = "Processed payload via RPC: " + payload.Name
	span.SetStatus(otelcodes.Ok, "")

	return nil
}
