package main

import (
	"authentication/data"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgconn"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const webPort = "80"

type Config struct {
	DB         *sql.DB
	Models     userStore
	HTTPClient *http.Client
	LoggerURL  string
}

type userStore interface {
	GetByEmail(email string) (*data.User, error)
}

func main() {
	log.Println("Starting authentication service")

	// connect to DB
	dsn := os.Getenv("DSN")
	if dsn == "" {
		log.Fatal("DSN must be set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := setupTelemetry("authentication-service")
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

	conn, err := connectToDB(ctx, dsn)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		log.Fatal(err)
	}

	models := data.New(conn)

	// set up config
	app := Config{
		DB:         conn,
		Models:     &models.User,
		HTTPClient: newObservedHTTPClient(),
		LoggerURL:  envOrDefault("LOGGER_URL", "http://logger-service-app/log"),
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", webPort),
		Handler:           otelhttp.NewHandler(app.routes(), "authentication-service"),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	err = runHTTPServer(ctx, srv)
	if err != nil {
		log.Fatal(err)
	}
}

func openDB(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = db.PingContext(pingCtx)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func connectToDB(ctx context.Context, dsn string) (*sql.DB, error) {
	var counts int
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		connection, err := openDB(ctx, dsn)
		if err != nil {
			log.Println("Postgres not yet ready ...")
			counts++
			if counts > 10 {
				return nil, fmt.Errorf("connect to postgres: %w", err)
			}
			log.Println("Backing off for two seconds...")
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			continue
		}

		log.Println("Connected to Postgres!")
		return connection, nil
	}
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
