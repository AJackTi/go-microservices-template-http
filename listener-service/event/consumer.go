package event

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Consumer struct {
	conn      *amqp.Connection
	queueName string
	loggerURL string
	client    *http.Client
}

func NewConsumer(conn *amqp.Connection, loggerURL string, client *http.Client) (*Consumer, error) {
	consumer := Consumer{
		conn:      conn,
		loggerURL: loggerURL,
		client:    client,
	}

	err := consumer.setup()
	if err != nil {
		return nil, err
	}

	return &consumer, nil
}

func (consumer *Consumer) setup() error {
	channel, err := consumer.conn.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()

	return declareExchange(channel)
}

type Payload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func (consumer *Consumer) Listen(ctx context.Context, topics []string) error {
	ch, err := consumer.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	q, err := declareRandomQueue(ch)
	if err != nil {
		return err
	}

	for _, s := range topics {
		err = ch.QueueBind(q.Name,
			s,
			"logs_topic",
			false,
			nil)

		if err != nil {
			return err
		}
	}

	consumerTag := "listener-service"
	messages, err := ch.Consume(q.Name, consumerTag, true, false, false, false, nil)
	if err != nil {
		return err
	}

	fmt.Printf("Waiting for message [Exchange, Queue] [logs_topic, %s]\n", q.Name)

	for {
		select {
		case <-ctx.Done():
			if err := ch.Cancel(consumerTag, false); err != nil {
				log.Println("error cancelling RabbitMQ consumer:", err)
			}
			return nil
		case d, ok := <-messages:
			if !ok {
				if ctx.Err() != nil {
					return nil
				}
				return fmt.Errorf("rabbitmq consumer closed unexpectedly")
			}

			var payload Payload
			if err := json.Unmarshal(d.Body, &payload); err != nil {
				log.Println("error decoding RabbitMQ message:", err)
				continue
			}

			if err := consumer.handlePayload(ctx, payload); err != nil {
				log.Println(err)
			}
		}
	}
}

func (consumer *Consumer) handlePayload(ctx context.Context, payload Payload) error {
	switch payload.Name {
	case "log", "event":
		// log whatever we get
		return consumer.logEvent(ctx, payload)

	case "auth":
		// authenticate
		return nil

	// you can have as many cases as you want, as long as you write the logic

	default:
		return consumer.logEvent(ctx, payload)
	}
}

func (consumer *Consumer) logEvent(ctx context.Context, entry Payload) error {
	jsonData, err := json.MarshalIndent(entry, "", "\t")
	if err != nil {
		return err
	}

	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, consumer.loggerURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")

	client := consumer.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("unexpected status from logger service: %s", response.Status)
	}

	return nil
}
