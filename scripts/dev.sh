#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

require_command() {
  local command_name="$1"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    echo "${command_name} is required for local development" >&2
    exit 1
  fi
}

cleanup() {
  local exit_code="$?"
  if [[ -n "${SAAS_PID:-}" ]] && kill -0 "$SAAS_PID" >/dev/null 2>&1; then
    kill "$SAAS_PID" >/dev/null 2>&1 || true
    wait "$SAAS_PID" >/dev/null 2>&1 || true
  fi
  exit "$exit_code"
}

trap cleanup INT TERM EXIT

require_command docker
require_command go
require_command npm
require_command curl

"$ROOT_DIR/scripts/ensure-env.sh"
source "$ROOT_DIR/scripts/export-env.sh"

echo "starting postgres and redis with docker compose..."
docker compose up -d postgres redis
"$ROOT_DIR/scripts/wait-for-deps.sh"

if [[ ! -d "$ROOT_DIR/web-frontend/node_modules" ]]; then
  echo "installing frontend dependencies with npm ci..."
  (cd "$ROOT_DIR/web-frontend" && npm ci)
fi

echo "starting SaaS backend on http://127.0.0.1:${QUANTSAAS_SERVER_PORT:-18080} ..."
go run ./cmd/saas -config config.yaml &
SAAS_PID="$!"
"$ROOT_DIR/scripts/wait-for-http.sh" "http://127.0.0.1:${QUANTSAAS_SERVER_PORT:-18080}/healthz" 60 >/dev/null

echo "SaaS is ready."
echo "Backend is running on http://127.0.0.1:${QUANTSAAS_SERVER_PORT:-18080}"
echo "Frontend will run on http://${QUANTSAAS_WEB_HOST:-127.0.0.1}:${QUANTSAAS_WEB_PORT:-4173}"
echo "To enable the local agent later: make agent-config && edit config.agent.yaml && make agent"

cd "$ROOT_DIR/web-frontend"
npm run dev -- --host "${QUANTSAAS_WEB_HOST:-127.0.0.1}" --port "${QUANTSAAS_WEB_PORT:-4173}"
