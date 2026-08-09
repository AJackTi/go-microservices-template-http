package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Mailer Mail
}

const webPort = "80"

func main() {
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
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	err = srv.ListenAndServe()
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
