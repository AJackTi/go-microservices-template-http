# Go Microservices Communication Lab

A hands-on Go lab that runs the same workflow through HTTP, gRPC, net/rpc, and
RabbitMQ. The project is being hardened into an observable reference
implementation; it is not yet a production deployment template.

## What is included

- Go executables for gateway, authentication, logging, mail, event consumption,
  and the browser demo
- PostgreSQL, MongoDB, RabbitMQ, and Mailpit
- HTTP, gRPC, net/rpc, and asynchronous event examples
- Multi-architecture container builds from source
- Deterministic schema, seed data, health checks, and end-to-end smoke tests

## Quick start

Requirements: Go, Docker with Compose, curl, jq, and ripgrep.

    cp .env.example .env
    make doctor
    make demo

The demo verifies all workflows rather than only starting containers.

| Interface | URL |
| --- | --- |
| Browser demo | http://localhost:8081 |
| Gateway | http://localhost:8080 |
| Mailpit | http://localhost:8025 |

The local seeded credentials are:

    admin@example.com
    verysecret

These credentials and the values in .env.example are only for local
development.

## Runtime flow

    Browser -> Gateway
    Gateway -> Authentication -> PostgreSQL
    Gateway -> Logging -> MongoDB
    Gateway -> RabbitMQ -> Listener -> Logging
    Gateway -> Mail -> Mailpit

## Commands

    make help
    make verify-local
    make build
    make dev
    make smoke
    make verify-phase-1
    make logs
    make down
    make clean

make clean removes only containers and named volumes owned by this Compose
project.

## Project status

The current roadmap is:

1. Reproducible Golden Path and clean-room smoke verification
2. Correctness, dependency, and security remediation
3. Deep use-case modules and canonical contracts
4. Reliable event delivery and integration testing
5. OpenTelemetry, protocol comparison, and failure injection
6. CI/CD, signed releases, community documentation, and template ergonomics

See [docs/observability-and-protocols.md](docs/observability-and-protocols.md)
for the phase 5 tracing and protocol comparison notes.

## Attribution

See [UPSTREAM.md](UPSTREAM.md). The project is distributed under the
[MIT License](LICENSE.md).
