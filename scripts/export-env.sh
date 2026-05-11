#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ENV_FILE="${1:-$ROOT_DIR/.env}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "env file not found: $ENV_FILE" >&2
  exit 1
fi

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

while IFS= read -r line || [[ -n "$line" ]]; do
  line="${line%$'\r'}"
  if [[ -z "$(trim "$line")" ]] || [[ "$(trim "$line")" == \#* ]]; then
    continue
  fi

  key="${line%%=*}"
  value="${line#*=}"
  key="$(trim "$key")"

  if [[ -z "$key" ]]; then
    continue
  fi

  value="$(trim "$value")"
  if [[ "$value" =~ ^\".*\"$ ]] || [[ "$value" =~ ^\'.*\'$ ]]; then
    value="${value:1:${#value}-2}"
  fi

  export "$key=$value"
done < "$ENV_FILE"
