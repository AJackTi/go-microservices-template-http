package main

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"
)

type rpcTracePayload struct {
	Name        string
	Data        string
	TraceParent string
	TraceState  string
	Baggage     string
}

type rpcTraceCarrier struct {
	payload *rpcTracePayload
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

func injectRPCTraceContext(ctx context.Context, payload *rpcTracePayload) {
	otel.GetTextMapPropagator().Inject(ctx, rpcTraceCarrier{payload: payload})
}
