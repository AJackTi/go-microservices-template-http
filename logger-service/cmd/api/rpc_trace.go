package main

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
)

type rpcTraceCarrier struct {
	payload *RPCPayload
}

func (c rpcTraceCarrier) Get(key string) string {
	switch strings.ToLower(key) {
	case "traceparent":
		return c.payload.TraceParent
	case "tracestate":
		return c.payload.TraceState
	case "baggage":
		return c.payload.Baggage
	default:
		return ""
	}
}

func (c rpcTraceCarrier) Set(key, value string) {
	switch strings.ToLower(key) {
	case "traceparent":
		c.payload.TraceParent = value
	case "tracestate":
		c.payload.TraceState = value
	case "baggage":
		c.payload.Baggage = value
	}
}

func (c rpcTraceCarrier) Keys() []string {
	return []string{"traceparent", "tracestate", "baggage"}
}

func extractTraceContext(ctx context.Context, payload RPCPayload) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, rpcTraceCarrier{payload: &payload})
}
