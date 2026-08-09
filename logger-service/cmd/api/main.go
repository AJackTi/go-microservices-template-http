package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log-service/data"
	"log-service/logs"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"google.golang.org/grpc"
)

const (
	webPort  = "80"
	rpcPort  = "5001"
	gRpcPort = "50001"
)

var client *mongo.Client

type Config struct {
	Models data.Models
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// connect to mongo
	mongoClient, err := connectToMongo(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Fatal(err)
	}
	client = mongoClient

	// close connection
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err = client.Disconnect(ctx); err != nil {
			log.Fatal(err)
		}
	}()

	app := Config{
		Models: *data.New(client),
	}

	rpcServer := rpc.NewServer()
	if err = rpcServer.Register(new(RPCServer)); err != nil {
		log.Fatal(err)
	}

	rpcListener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%s", rpcPort))
	if err != nil {
		log.Fatal(err)
	}

	grpcListener, err := net.Listen("tcp", fmt.Sprintf(":%s", gRpcPort))
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	logs.RegisterLogServiceServer(grpcServer, &LogServer{Models: app.Models})

	// start web server
	log.Println("Starting service on port", webPort)
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", webPort),
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	wg.Add(3)
	go func() {
		defer wg.Done()
		if err := runHTTPServer(ctx, srv); err != nil {
			errCh <- err
		}
	}()

	go func() {
		defer wg.Done()
		if err := app.rpcListen(ctx, rpcListener, rpcServer); err != nil {
			errCh <- err
		}
	}()

	go func() {
		defer wg.Done()
		if err := app.gRPCListen(ctx, grpcListener, grpcServer); err != nil {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		stop()
		wg.Wait()
		log.Fatal(err)
	case <-ctx.Done():
		stop()
		wg.Wait()
	}
}

func runHTTPServer(ctx context.Context, srv *http.Server) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}

		err := <-errCh
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func (app *Config) rpcListen(ctx context.Context, listener net.Listener, server *rpc.Server) error {
	log.Println("Starting RPC server on port", rpcPort)
	defer listener.Close()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		rpcConn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}

		go server.ServeConn(rpcConn)
	}
}

func connectToMongo(ctx context.Context) (*mongo.Client, error) {
	mongoURL := os.Getenv("MONGO_URL")
	if mongoURL == "" {
		mongoURL = "mongodb://mongo:27017"
	}
	mongoUsername := os.Getenv("MONGO_USERNAME")
	if mongoUsername == "" {
		return nil, errors.New("MONGO_USERNAME must be set")
	}
	mongoPassword := os.Getenv("MONGO_PASSWORD")
	if mongoPassword == "" {
		return nil, errors.New("MONGO_PASSWORD must be set")
	}

	// create connection options
	clientOptions := options.Client().ApplyURI(mongoURL)
	clientOptions.SetAuth(options.Credential{
		Username: mongoUsername,
		Password: mongoPassword,
	})

	// connect
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	c, err := mongo.Connect(clientOptions)
	if err != nil {
		log.Println("Error connecting: ", err)
		return nil, err
	}

	if err = c.Ping(pingCtx, nil); err != nil {
		_ = c.Disconnect(context.Background())
		return nil, fmt.Errorf("ping mongo: %w", err)
	}

	log.Println("Connected to mongo!")

	return c, nil
}
