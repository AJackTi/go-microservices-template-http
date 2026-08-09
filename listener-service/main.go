package main

import (
	"context"
	"errors"
	"fmt"
	"listener/event"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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

	loggerURL := envOrDefault("LOGGER_URL", "http://logger-service-app/log")
	httpClient := &http.Client{Timeout: 10 * time.Second}

	// start listening for messages
	log.Println("Listening for and consuming RabbitMQ messages...")

	// create consumer
	consumer, err := event.NewConsumer(rabbitConn, loggerURL, httpClient)
	if err != nil {
		log.Fatal(err)
	}

	// watch the queue and consume events
	err = consumer.Listen(ctx, []string{"log.INFO", "log.WARNING", "log.ERROR"})
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
		return nil, fmt.Errorf("RABBIT_URL must be set")
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
