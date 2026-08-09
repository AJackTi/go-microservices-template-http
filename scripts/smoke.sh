#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_NAME="go-microservices-lab"

if [[ "${1:-}" == "--project" ]]; then
  PROJECT_NAME="${2:?missing project name}"
  shift 2
fi

COMPOSE=(docker compose --project-name "$PROJECT_NAME" -f "$ROOT_DIR/project/docker-compose.yml")
BROKER_URL="${BROKER_URL:-http://127.0.0.1:8080}"
FRONTEND_URL="${FRONTEND_URL:-http://127.0.0.1:8081}"
MAILPIT_URL="${MAILPIT_URL:-http://127.0.0.1:8025}"
RUN_ID="smoke-$(date +%s)-$$"

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
  echo "RabbitMQ event did not reach MongoDB" >&2
  return 1
}

poll_mailpit() {
  local subject="$1"
  for ((i = 1; i <= 30; i++)); do
    if curl --fail --silent --show-error "$MAILPIT_URL/api/v1/messages" \
      | jq --exit-status --arg subject "$subject" '.messages[]? | select(.Subject == $subject)' >/dev/null; then
      return 0
    fi
    sleep 2
  done
  echo "mail with subject '$subject' did not reach Mailpit" >&2
  return 1
}

log "waiting for public endpoints"
wait_for_http "$BROKER_URL/ping"
wait_for_http "$FRONTEND_URL/"
wait_for_http "$MAILPIT_URL/"

log "checking frontend"
curl --fail --silent --show-error "$FRONTEND_URL/" | grep -q "Test microservices"

log "checking broker"
response="$(post_json "/" '{}')"
assert_json "$response" '.error == false and .message == "Hit the broker"'

log "checking authentication"
response="$(post_json "/handle" '{"action":"auth","auth":{"email":"admin@example.com","password":"verysecret"}}')"
assert_json "$response" '.error == false and .data.email == "admin@example.com"'

log "checking HTTP log adapter"
response="$(post_json "/handle" "{\"action\":\"log\",\"log\":{\"name\":\"$RUN_ID\",\"data\":\"http-$RUN_ID\"}}")"
assert_json "$response" '.error == false and .message == "logged"'

log "checking net/rpc log adapter"
response="$(post_json "/handle" "{\"action\":\"log-via-rpc\",\"log\":{\"name\":\"$RUN_ID\",\"data\":\"rpc-$RUN_ID\"}}")"
assert_json "$response" '.error == false and (.message | startswith("Processed payload via RPC:"))'

log "checking gRPC log adapter"
response="$(post_json "/log-grpc" "{\"action\":\"log\",\"log\":{\"name\":\"$RUN_ID\",\"data\":\"grpc-$RUN_ID\"}}")"
assert_json "$response" '.error == false and .message == "logged"'

log "checking RabbitMQ delivery"
event_payload="event-$RUN_ID"
response="$(post_json "/handle" "{\"action\":\"log-event\",\"log\":{\"name\":\"event\",\"data\":\"$event_payload\"}}")"
assert_json "$response" '.error == false and .message == "logged via RabbitMQ"'
poll_mongo "$event_payload"

log "checking mail delivery"
mail_subject="Mail $RUN_ID"
response="$(post_json "/handle" "{\"action\":\"mail\",\"mail\":{\"from\":\"sender@example.test\",\"to\":\"receiver@example.test\",\"subject\":\"$mail_subject\",\"message\":\"hello from $RUN_ID\"}}")"
assert_json "$response" '.error == false'
poll_mailpit "$mail_subject"

log "checking seeded database"
admin_count="$("${COMPOSE[@]}" exec -T postgres psql \
  --username "${POSTGRES_USER:-microservices}" \
  --dbname "${POSTGRES_DB:-users}" \
  --tuples-only \
  --no-align \
  --command "SELECT count(*) FROM users WHERE email = 'admin@example.com' AND user_active = 1;" \
  | tr -d '[:space:]')"
[[ "$admin_count" == "1" ]]

log "checking container state"
expected_services=(
  authentication-service
  broker-service
  front-end
  listener-service
  logger-service-app
  mailer-service
  mailpit
  mongo
  postgres
  rabbitmq-app
)
for service in "${expected_services[@]}"; do
  container_id="$("${COMPOSE[@]}" ps --quiet "$service")"
  [[ -n "$container_id" ]]
  state="$(docker inspect --format '{{.State.Status}}' "$container_id")"
  restarts="$(docker inspect --format '{{.RestartCount}}' "$container_id")"
  [[ "$state" == "running" ]]
  [[ "$restarts" == "0" ]]
done

log "all smoke scenarios passed"
