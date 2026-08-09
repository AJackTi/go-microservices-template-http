package event

import (
	"bytes"
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

func (consumer *Consumer) Listen(topics []string) error {
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

	messages, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	if err != nil {
		return err
	}

	forever := make(chan bool)
	go func() {
		for d := range messages {
			var payload Payload
			if err := json.Unmarshal(d.Body, &payload); err != nil {
				log.Println("error decoding RabbitMQ message:", err)
				continue
			}

			go consumer.handlePayload(payload)
		}
	}()

	fmt.Printf("Waiting for message [Exchange, Queue] [logs_topic, %s]\n", q.Name)
	<-forever

	return nil
}

func (consumer *Consumer) handlePayload(payload Payload) {
	switch payload.Name {
	case "log", "event":
		// log whatever we get
		err := consumer.logEvent(payload)
		if err != nil {
			log.Println(err)
		}

	case "auth":
		// authenticate

	// you can have as many cases as you want, as long as you write the logic

	default:
		err := consumer.logEvent(payload)
		if err != nil {
			log.Println(err)
		}
	}
}

func (consumer *Consumer) logEvent(entry Payload) error {
	jsonData, err := json.MarshalIndent(entry, "", "\t")
	if err != nil {
		return err
	}

	request, err := http.NewRequest("POST", consumer.loggerURL, bytes.NewBuffer(jsonData))
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
