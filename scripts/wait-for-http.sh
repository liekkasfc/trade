#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <url> [timeout_seconds]" >&2
  exit 1
fi

URL="$1"
TIMEOUT_SECONDS="${2:-60}"
STARTED_AT="$(date +%s)"

until curl -fsS "$URL" >/dev/null 2>&1; do
  if (( $(date +%s) - STARTED_AT >= TIMEOUT_SECONDS )); then
    echo "timed out waiting for ${URL}" >&2
    exit 1
  fi
  sleep 1
done

echo "${URL} is ready"
