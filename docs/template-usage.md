# Template usage

This repo is meant to be copied and customized.

Suggested edit list:

- Rename services if your domain uses different terms.
- Update `.env.example` with local defaults for your machine.
- Adjust `BROKER_HOST_PORT`, `FRONTEND_HOST_PORT`, and `MAILPIT_HOST_PORT` if
  those ports are already busy on your system.
- Update the README quick start and attribution links.
- Keep the verification scripts in sync with any new public workflows.

The repo already separates HTTP, gRPC, net/rpc, and RabbitMQ seams so each
transport can evolve without collapsing the others.

