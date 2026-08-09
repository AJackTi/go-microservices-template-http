package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const webPort = "8080"

type Config struct {
	Rabbit     *amqp.Connection
	HTTPClient *http.Client
	AuthURL    string
	LoggerURL  string
	MailURL    string
	RPCAddr    string
	GRPCAddr   string
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := setupTelemetry("broker-service")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTelemetry(shutdownCtx); err != nil {
			log.Println("error shutting down telemetry:", err)
		}
	}()

	// try to connect to rabbitmq
	rabbitConn, err := connect(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Println(err)
		os.Exit(1)
	}
	defer rabbitConn.Close()

	app := Config{
		Rabbit:     rabbitConn,
		HTTPClient: newObservedHTTPClient(),
		AuthURL:    envOrDefault("AUTH_URL", "http://authentication-service/authenticate"),
		LoggerURL:  envOrDefault("LOGGER_URL", "http://logger-service-app/log"),
		MailURL:    envOrDefault("MAIL_URL", "http://mailer-service/send"),
		RPCAddr:    envOrDefault("RPC_ADDR", "logger-service-app:5001"),
		GRPCAddr:   envOrDefault("GRPC_ADDR", "logger-service-app:50001"),
	}

	log.Printf("Starting broker service on port %s\n", webPort)

	// define http server
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", webPort),
		Handler:           otelhttp.NewHandler(app.routes(), "broker-service"),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// start the server
	err = runHTTPServer(ctx, srv)
	if err != nil {
		log.Fatal(err)
	}
}

func connect(ctx context.Context) (*amqp.Connection, error) {
	var counts int64
	var backOff = 1 * time.Second
	var connection *amqp.Connection
	rabbitURL := os.Getenv("RABBIT_URL")
	if rabbitURL == "" {
		return nil, errors.New("RABBIT_URL must be set")
	}

	// don't continue until rabbit is ready
	dialer := net.Dialer{Timeout: 5 * time.Second}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		c, err := amqp.DialConfig(rabbitURL, amqp.Config{
			Dial: func(network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, addr)
			},
		})
		if err != nil {
			fmt.Println("RabbitMQ not yet ready...")
			counts++
		} else {
			log.Println("Connected to RabbitMQ!")
			connection = c
			break
		}

		if counts > 5 {
			fmt.Println(err)
			return nil, err
		}

		backOff = time.Duration(math.Pow(float64(counts), 2)) * time.Second
		log.Println("backing off...")
		timer := time.NewTimer(backOff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
			continue
		}
	}

	return connection, nil
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
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
