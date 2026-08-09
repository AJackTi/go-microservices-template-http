package event

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	ottrace "go.opentelemetry.io/otel/trace"
)

func TestRabbitTraceContextRoundTrip(t *testing.T) {
	tp := sdktrace.NewTracerProvider()
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	ctx, span := otel.Tracer("broker-test").Start(context.Background(), "parent")
	defer span.End()

	headers := amqp.Table{}
	injectTraceContext(ctx, headers)

	extracted := extractTraceContext(context.Background(), headers)
	spanContext := ottrace.SpanContextFromContext(extracted)
	if !spanContext.IsValid() {
		t.Fatal("expected extracted span context to be valid")
	}

	if spanContext.TraceID() != span.SpanContext().TraceID() {
		t.Fatalf("unexpected trace id: got %s want %s", spanContext.TraceID(), span.SpanContext().TraceID())
	}
}
