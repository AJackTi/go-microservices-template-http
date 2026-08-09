package main

import (
	"context"
	"errors"
	"log"
	"log-service/data"
	"log-service/logs"
	"net"
	"time"

	"go.opentelemetry.io/otel"
	otelcodes "go.opentelemetry.io/otel/codes"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type LogServer struct {
	logs.UnimplementedLogServiceServer
	Models data.Models
}

func (l *LogServer) WriteLog(ctx context.Context, req *logs.LogRequest) (*logs.LogResponse, error) {
	_, span := otel.Tracer("logger-service").Start(ctx, "logger.grpc.write_log")
	defer span.End()

	input := req.GetLogEntry()
	if input == nil {
		err := errors.New("log entry is required")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return &logs.LogResponse{Result: "failed"}, err
	}

	if shouldFailRequest("grpc", input.Name+" "+input.Data) {
		err := status.Error(grpcCodes.Unavailable, "forced failure for grpc")
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return &logs.LogResponse{Result: "failed"}, err
	}

	// write the log
	logEntry := data.LogEntry{
		Name: input.Name,
		Data: input.Data,
	}

	err := l.Models.LogEntry.Insert(&logEntry)
	if err != nil {
		res := &logs.LogResponse{Result: "failed"}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return res, err
	}

	// return response
	res := &logs.LogResponse{Result: "logged"}
	span.SetStatus(otelcodes.Ok, "")
	return res, nil
}

func (app *Config) gRPCListen(ctx context.Context, listener net.Listener, server *grpc.Server) error {
	log.Printf("gRPC Server started on port %s", gRpcPort)

	go func() {
		<-ctx.Done()
		done := make(chan struct{})
		go func() {
			server.GracefulStop()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(10 * time.Second):
			server.Stop()
		}

		_ = listener.Close()
	}()

	if err := server.Serve(listener); err != nil {
		if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}

	return nil
}
