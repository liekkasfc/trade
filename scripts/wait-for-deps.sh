#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required to wait for local dependencies" >&2
  exit 1
fi

wait_for_postgres() {
  local timeout="${1:-60}"
  local started_at
  started_at="$(date +%s)"

  until docker compose exec -T postgres pg_isready -U postgres -d quantsaas >/dev/null 2>&1; do
    if (( $(date +%s) - started_at >= timeout )); then
      echo "postgres did not become ready within ${timeout}s" >&2
      exit 1
    fi
    sleep 1
  done
}

wait_for_redis() {
  local timeout="${1:-60}"
  local started_at
  started_at="$(date +%s)"

  until docker compose exec -T redis redis-cli ping >/dev/null 2>&1; do
    if (( $(date +%s) - started_at >= timeout )); then
      echo "redis did not become ready within ${timeout}s" >&2
      exit 1
    fi
    sleep 1
  done
}

wait_for_postgres "${1:-60}"
wait_for_redis "${1:-60}"

echo "postgres and redis are ready"
