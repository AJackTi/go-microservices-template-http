package main

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
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
	// try to connect to rabbitmq
	rabbitConn, err := connect()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	defer rabbitConn.Close()

	app := Config{
		Rabbit:     rabbitConn,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
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
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// start the server
	err = srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func connect() (*amqp.Connection, error) {
	var counts int64
	var backOff = 1 * time.Second
	var connection *amqp.Connection
	rabbitURL := os.Getenv("RABBIT_URL")
	if rabbitURL == "" {
		rabbitURL = "amqp://guest:guest@rabbitmq-app"
	}

	// don't continue until rabbit is ready
	for {
		c, err := amqp.Dial(rabbitURL)
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
		time.Sleep(backOff)
		continue
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
