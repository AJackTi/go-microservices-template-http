package main

import (
	"context"
	"errors"
	"log"
	"log-service/data"
	"strings"
	"time"
)

// RPCServer is the type for our RPC Server. Methods that take this as a receiver are avaiable
// over RPC, as long as they are exported.
type RPCServer struct {
}

// RPCPayload is the type for data we receive from RPC
type RPCPayload struct {
	Name string
	Data string
}

// LogInfo writes our payload to mongo
func (r *RPCServer) LogInfo(payload RPCPayload, resp *string) error {
	if strings.TrimSpace(payload.Name) == "" || strings.TrimSpace(payload.Data) == "" {
		return errors.New("name and data are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	collection := client.Database("logs").Collection("logs")
	_, err := collection.InsertOne(ctx, data.LogEntry{
		Name:      payload.Name,
		Data:      payload.Data,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		log.Println("error writing to mongo", err)
		return err
	}

	*resp = "Processed payload via RPC: " + payload.Name

	return nil
}
