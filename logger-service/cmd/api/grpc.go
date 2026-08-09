package main

import (
	"context"
	"errors"
	"log"
	"log-service/data"
	"log-service/logs"
	"net"
	"time"

	"google.golang.org/grpc"
)

type LogServer struct {
	logs.UnimplementedLogServiceServer
	Models data.Models
}

func (l *LogServer) WriteLog(ctx context.Context, req *logs.LogRequest) (*logs.LogResponse, error) {
	input := req.GetLogEntry()
	if input == nil {
		return &logs.LogResponse{Result: "failed"}, errors.New("log entry is required")
	}

	// write the log
	logEntry := data.LogEntry{
		Name: input.Name,
		Data: input.Data,
	}

	err := l.Models.LogEntry.Insert(&logEntry)
	if err != nil {
		res := &logs.LogResponse{Result: "failed"}
		return res, err
	}

	// return response
	res := &logs.LogResponse{Result: "logged"}
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
