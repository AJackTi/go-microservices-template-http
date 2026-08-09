package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Config struct {
	Mailer Mail
}

const webPort = "80"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTelemetry, err := setupTelemetry("mail-service")
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

	mailer, err := createMail()
	if err != nil {
		log.Fatal(err)
	}

	app := Config{
		Mailer: mailer,
	}

	log.Println("Starting mail service on port", webPort)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", webPort),
		Handler:           otelhttp.NewHandler(app.routes(), "mail-service"),
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

func createMail() (Mail, error) {
	port, err := strconv.Atoi(strings.TrimSpace(os.Getenv("MAIL_PORT")))
	if err != nil {
		return Mail{}, fmt.Errorf("invalid MAIL_PORT: %w", err)
	}
	if port <= 0 {
		return Mail{}, errors.New("MAIL_PORT must be greater than zero")
	}

	m := Mail{
		Domain:      strings.TrimSpace(os.Getenv("MAIL_DOMAIN")),
		Host:        strings.TrimSpace(os.Getenv("MAIL_HOST")),
		Port:        port,
		Username:    os.Getenv("MAIL_USERNAME"),
		Password:    os.Getenv("MAIL_PASSWORD"),
		Encryption:  strings.TrimSpace(os.Getenv("MAIL_ENCRYPTION")),
		FromName:    strings.TrimSpace(os.Getenv("FROM_NAME")),
		FromAddress: strings.TrimSpace(os.Getenv("FROM_ADDRESS")),
	}

	if m.Domain == "" || m.Host == "" || m.FromName == "" || m.FromAddress == "" {
		return Mail{}, errors.New("mail configuration is incomplete")
	}

	if m.Encryption == "" {
		m.Encryption = "none"
	}

	return m, nil
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
