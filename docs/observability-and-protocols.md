# Observability and protocol comparison

This lab now emits OpenTelemetry traces from every main service.

How tracing works:

- HTTP services use `otelhttp` for inbound requests and outbound clients.
- gRPC uses `otelgrpc` on the broker client and logger server.
- RabbitMQ propagates W3C trace headers through message headers.
- net/rpc carries trace context in the gob payload itself.

Protocol comparison:

| Protocol | Strength | Tradeoff | Used for |
| --- | --- | --- | --- |
| HTTP | simple, familiar, debuggable | request/response only | browser, auth, mail, broker, logger |
| gRPC | strong contracts, fast, streaming-ready | harder to inspect by hand | broker → logger |
| net/rpc | minimal demo of legacy RPC | weak schema/compat story | broker → logger |
| RabbitMQ | decoupled, async, resilient | eventual consistency | broker → listener → logger |

Failure injection:

- `fail:http` in `name` or `data` forces the logger HTTP handler to fail.
- `fail:grpc` forces the logger gRPC server to fail.
- `fail:rpc` forces the logger RPC server to fail.
- `fail:all` forces any logger protocol to fail.

This lets you compare how each transport behaves when the same downstream service is unhealthy.
