#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_NAME="go-microservices-phase4"

if [[ "${1:-}" == "--project" ]]; then
  PROJECT_NAME="${2:?missing project name}"
  shift 2
fi

COMPOSE=(docker compose --project-name "$PROJECT_NAME" -f "$ROOT_DIR/project/docker-compose.yml")
BROKER_URL="${BROKER_URL:-http://127.0.0.1:8080}"
MAILPIT_URL="${MAILPIT_URL:-http://127.0.0.1:8025}"
RUN_ID="phase4-$(date +%s)-$$"

log() {
  printf '==> %s\n' "$*"
}

wait_for_http() {
  local url="$1"
  local attempts="${2:-40}"

  for ((i = 1; i <= attempts; i++)); do
    if curl --fail --silent --show-error "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done

  echo "timed out waiting for $url" >&2
  return 1
}

post_json() {
  local path="$1"
  local payload="$2"
  curl --fail --silent --show-error \
    -H 'Content-Type: application/json' \
    --data "$payload" \
    "$BROKER_URL$path"
}

assert_json() {
  local response="$1"
  local expression="$2"
  jq --exit-status "$expression" <<<"$response" >/dev/null
}

poll_mongo() {
  local payload="$1"
  for ((i = 1; i <= 45; i++)); do
    local count
    count="$("${COMPOSE[@]}" exec -T mongo mongosh \
      --quiet \
      --username "${MONGO_USERNAME:-admin}" \
      --password "${MONGO_PASSWORD:-microservices}" \
      --authenticationDatabase admin \
      --eval "db.getSiblingDB('logs').logs.countDocuments({data: '$payload'})" | tail -n 1 | tr -d '\r')"
    if [[ "$count" =~ ^[1-9][0-9]*$ ]]; then
      return 0
    fi
    sleep 2
  done
  echo "event '$payload' did not reach MongoDB" >&2
  return 1
}

cleanup() {
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

cd "$ROOT_DIR"

log "validating local source"
make verify-local

log "validating Compose"
"${COMPOSE[@]}" config --quiet

log "removing any previous verification stack"
cleanup

log "building images from source"
"${COMPOSE[@]}" build --pull

log "starting clean stack"
"${COMPOSE[@]}" up -d --wait --wait-timeout 180

log "waiting for public endpoints"
wait_for_http "$BROKER_URL/ping"
wait_for_http "$MAILPIT_URL/"

log "checking baseline event delivery"
baseline_payload="baseline-$RUN_ID"
response="$(post_json "/handle" "{\"action\":\"log-event\",\"log\":{\"name\":\"event\",\"data\":\"$baseline_payload\"}}")"
assert_json "$response" '.error == false and .message == "logged via RabbitMQ"'
poll_mongo "$baseline_payload"

log "checking listener recovery after logger outage"
outage_payload="outage-$RUN_ID"
"${COMPOSE[@]}" stop logger-service-app
sleep 2
response="$(post_json "/handle" "{\"action\":\"log-event\",\"log\":{\"name\":\"event\",\"data\":\"$outage_payload\"}}")"
assert_json "$response" '.error == false and .message == "logged via RabbitMQ"'
"${COMPOSE[@]}" start logger-service-app
"${COMPOSE[@]}" up -d --wait --wait-timeout 180 logger-service-app
poll_mongo "$outage_payload"

log "Phase 4 verification passed"
