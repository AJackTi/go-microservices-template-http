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
	"time"

	_ "github.com/jackc/pgconn"
	_ "github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const webPort = "80"

var counts int64

type Config struct {
	DB         *sql.DB
	Models     data.Models
	HTTPClient *http.Client
	LoggerURL  string
}

func main() {
	log.Println("Starting authentication service")

	// connect to DB
	dsn := os.Getenv("DSN")
	if dsn == "" {
		log.Fatal("DSN must be set")
	}

	conn, err := connectToDB(dsn)
	if err != nil {
		log.Fatal(err)
	}

	// set up config
	app := Config{
		DB:         conn,
		Models:     data.New(conn),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		LoggerURL:  envOrDefault("LOGGER_URL", "http://logger-service-app/log"),
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", webPort),
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	err = srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func openDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	if err != nil {
		return nil, err
	}

	return db, nil
}

func connectToDB(dsn string) (*sql.DB, error) {
	var counts int
	for {
		connection, err := openDB(dsn)
		if err != nil {
			log.Println("Postgres not yet ready ...")
			counts++
			if counts > 10 {
				return nil, fmt.Errorf("connect to postgres: %w", err)
			}
			log.Println("Backing off for two seconds...")
			time.Sleep(2 * time.Second)
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
