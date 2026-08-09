#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROJECT_NAME="go-microservices-phase1"

if [[ "${1:-}" == "--project" ]]; then
  PROJECT_NAME="${2:?missing project name}"
  shift 2
fi

COMPOSE=(docker compose --project-name "$PROJECT_NAME" -f "$ROOT_DIR/project/docker-compose.yml")

cleanup() {
  "${COMPOSE[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

cd "$ROOT_DIR"

echo "==> validating local source"
make verify-local

echo "==> checking tracked artifacts"
tracked_binaries="$({
  git ls-files --cached --others --exclude-standard \
    | rg '(^|/)(frontApp|brokerApp|authApp|loggerServiceApp|mailerApp|listenerApp)$' \
    || true
} | while IFS= read -r artifact; do
  if [[ -e "$artifact" ]]; then
    printf '%s\n' "$artifact"
  fi
done)"
if [[ -n "$tracked_binaries" ]]; then
  printf '%s\n' "$tracked_binaries"
  echo "tracked application binaries are forbidden" >&2
  exit 1
fi

echo "==> validating Compose"
"${COMPOSE[@]}" config --quiet

echo "==> removing any previous verification stack"
cleanup

echo "==> building images from source"
"${COMPOSE[@]}" build --pull

echo "==> starting clean stack"
"${COMPOSE[@]}" up -d --wait --wait-timeout 180

echo "==> running end-to-end smoke tests"
"$ROOT_DIR/scripts/smoke.sh" --project "$PROJECT_NAME"

echo "==> checking fatal log patterns"
fatal_logs="$({
  "${COMPOSE[@]}" logs --no-color \
    | rg -i 'exec format error|access_refused|panic:|fatal' \
    || true
} | rg -vi 'postgres[^|]*\|.*fatal: +the database system is (starting up|shutting down)' || true)"
if [[ -n "$fatal_logs" ]]; then
  printf '%s\n' "$fatal_logs"
  echo "fatal container log pattern detected" >&2
  exit 1
fi

echo "==> Phase 1 verification passed"
