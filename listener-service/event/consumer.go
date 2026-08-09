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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
)

type Consumer struct {
	conn         *amqp.Connection
	queueName    string
	loggerURL    string
	client       *http.Client
	retryBackoff time.Duration
}

func NewConsumer(conn *amqp.Connection, loggerURL string, client *http.Client) (*Consumer, error) {
	consumer := Consumer{
		conn:         conn,
		loggerURL:    loggerURL,
		client:       client,
		retryBackoff: 250 * time.Millisecond,
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

	if err := ch.Qos(1, 0, false); err != nil {
		return err
	}

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
	messages, err := ch.Consume(q.Name, consumerTag, false, false, false, false, nil)
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

			if err := consumer.handleDelivery(ctx, d); err != nil {
				log.Println(err)
			}
		}
	}
}

func (consumer *Consumer) handleDelivery(ctx context.Context, delivery amqp.Delivery) error {
	ctx = extractTraceContext(ctx, delivery.Headers)
	ctx, span := otel.Tracer("listener-service").Start(ctx, "listener.rabbitmq.consume")
	defer span.End()
	span.SetAttributes(
		attribute.String("messaging.system", "rabbitmq"),
		attribute.String("messaging.destination", delivery.Exchange),
		attribute.String("messaging.rabbitmq.routing_key", delivery.RoutingKey),
		attribute.Bool("messaging.rabbitmq.redelivered", delivery.Redelivered),
	)

	var payload Payload
	if err := json.Unmarshal(delivery.Body, &payload); err != nil {
		if rejectErr := delivery.Reject(false); rejectErr != nil {
			span.RecordError(rejectErr)
			span.SetStatus(otelcodes.Error, rejectErr.Error())
			return fmt.Errorf("reject malformed RabbitMQ delivery: %w", rejectErr)
		}
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return fmt.Errorf("decode RabbitMQ message: %w", err)
	}

	if err := consumer.handlePayload(ctx, payload); err != nil {
		if nackErr := delivery.Nack(false, true); nackErr != nil {
			span.RecordError(nackErr)
			span.SetStatus(otelcodes.Error, nackErr.Error())
			return fmt.Errorf("nack RabbitMQ delivery: %w", nackErr)
		}

		if retryErr := consumer.waitBeforeRetry(ctx); retryErr != nil {
			span.RecordError(retryErr)
			span.SetStatus(otelcodes.Error, retryErr.Error())
			return retryErr
		}

		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return err
	}

	if ackErr := delivery.Ack(false); ackErr != nil {
		span.RecordError(ackErr)
		span.SetStatus(otelcodes.Error, ackErr.Error())
		return fmt.Errorf("ack RabbitMQ delivery: %w", ackErr)
	}

	span.SetStatus(otelcodes.Ok, "")
	return nil
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

func (consumer *Consumer) waitBeforeRetry(ctx context.Context) error {
	if consumer.retryBackoff <= 0 {
		return nil
	}

	timer := time.NewTimer(consumer.retryBackoff)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
