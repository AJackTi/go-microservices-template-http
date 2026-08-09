#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_NAME="go-microservices-phase5"
BROKER_HOST_PORT="${BROKER_HOST_PORT:-38080}"
FRONTEND_HOST_PORT="${FRONTEND_HOST_PORT:-38081}"
MAILPIT_HOST_PORT="${MAILPIT_HOST_PORT:-38025}"
export BROKER_HOST_PORT FRONTEND_HOST_PORT MAILPIT_HOST_PORT
export BROKER_URL="http://127.0.0.1:${BROKER_HOST_PORT}"
export FRONTEND_URL="http://127.0.0.1:${FRONTEND_HOST_PORT}"
export MAILPIT_URL="http://127.0.0.1:${MAILPIT_HOST_PORT}"

if [[ "${1:-}" == "--project" ]]; then
  PROJECT_NAME="${2:?missing project name}"
  shift 2
fi

COMPOSE=(docker compose --project-name "$PROJECT_NAME" -f "$ROOT_DIR/project/docker-compose.yml")
BROKER_URL="${BROKER_URL:-http://127.0.0.1:${BROKER_HOST_PORT}}"
FRONTEND_URL="${FRONTEND_URL:-http://127.0.0.1:${FRONTEND_HOST_PORT}}"
MAILPIT_URL="${MAILPIT_URL:-http://127.0.0.1:${MAILPIT_HOST_PORT}}"
RUN_ID="phase5-$(date +%s)-$$"

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

post_json_status() {
  local path="$1"
  local payload="$2"
  local body_file
  body_file="$(mktemp)"
  local status
  status="$(curl --silent --show-error \
    -H 'Content-Type: application/json' \
    --data "$payload" \
    --output "$body_file" \
    --write-out '%{http_code}' \
    "$BROKER_URL$path")"
  printf '%s\n' "$status"
  cat "$body_file"
  rm -f "$body_file"
}

assert_json() {
  local response="$1"
  local expression="$2"
  jq --exit-status "$expression" <<<"$response" >/dev/null
}

poll_mongo() {
  local payload="$1"
  for ((i = 1; i <= 30; i++)); do
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
wait_for_http "$FRONTEND_URL/"
wait_for_http "$MAILPIT_URL/"

curl --fail --silent --show-error "$FRONTEND_URL/" | grep -q "Test microservices"

log "checking success paths"
response="$(post_json "/handle" "{\"action\":\"log\",\"log\":{\"name\":\"$RUN_ID\",\"data\":\"http-$RUN_ID\"}}")"
assert_json "$response" '.error == false and .message == "logged"'

response="$(post_json "/handle" "{\"action\":\"log-via-rpc\",\"log\":{\"name\":\"$RUN_ID\",\"data\":\"rpc-$RUN_ID\"}}")"
assert_json "$response" '.error == false and (.message | startswith("Processed payload via RPC:"))'

response="$(post_json "/log-grpc" "{\"action\":\"log\",\"log\":{\"name\":\"$RUN_ID\",\"data\":\"grpc-$RUN_ID\"}}")"
assert_json "$response" '.error == false and .message == "logged"'

response="$(post_json "/handle" "{\"action\":\"log-event\",\"log\":{\"name\":\"event\",\"data\":\"event-$RUN_ID\"}}")"
assert_json "$response" '.error == false and .message == "logged via RabbitMQ"'
poll_mongo "event-$RUN_ID"

log "checking failure injection"
status_and_body="$(post_json_status "/handle" "{\"action\":\"log\",\"log\":{\"name\":\"$RUN_ID\",\"data\":\"fail:http\"}}")"
status="${status_and_body%%$'\n'*}"
body="${status_and_body#*$'\n'}"
[[ "$status" == "502" ]]
assert_json "$body" '.error == true'

status_and_body="$(post_json_status "/handle" "{\"action\":\"log-via-rpc\",\"log\":{\"name\":\"$RUN_ID\",\"data\":\"fail:rpc\"}}")"
status="${status_and_body%%$'\n'*}"
body="${status_and_body#*$'\n'}"
[[ "$status" == "502" ]]
assert_json "$body" '.error == true'

status_and_body="$(post_json_status "/log-grpc" "{\"action\":\"log\",\"log\":{\"name\":\"$RUN_ID\",\"data\":\"fail:grpc\"}}")"
status="${status_and_body%%$'\n'*}"
body="${status_and_body#*$'\n'}"
[[ "$status" == "502" ]]
assert_json "$body" '.error == true'

log "checking emitted trace logs"
logs="$("${COMPOSE[@]}" logs --no-color broker-service logger-service-app listener-service)"
grep -q 'service.name' <<<"$logs"

log "Phase 5 verification passed"
