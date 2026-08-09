package event

import (
	"context"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
)

type amqpCarrier struct {
	headers amqp.Table
}

func (c amqpCarrier) Get(key string) string {
	value, ok := c.headers[key]
	if !ok {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func (c amqpCarrier) Set(key, value string) {
	if c.headers == nil {
		c.headers = amqp.Table{}
	}
	c.headers[key] = value
}

func (c amqpCarrier) Keys() []string {
	keys := make([]string, 0, len(c.headers))
	for key := range c.headers {
		keys = append(keys, key)
	}
	return keys
}

func injectTraceContext(ctx context.Context, headers amqp.Table) amqp.Table {
	carrier := amqpCarrier{headers: headers}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	return carrier.headers
}

func extractTraceContext(ctx context.Context, headers amqp.Table) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, amqpCarrier{headers: headers})
}
