#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
INSTANCES_DIR="${BACKEND_DIR}/.instances"
COMPOSE_FILE="${BACKEND_DIR}/docker-compose.instance.yml"

usage() {
  cat <<'EOF'
Usage:
  scripts/instance-local.sh up --name <instance>
  scripts/instance-local.sh down --name <instance> [--purge]
  scripts/instance-local.sh logs --name <instance>
  scripts/instance-local.sh status --name <instance>

Optional flags for `up`:
  --api-port <port>
  --postgres-port <port>
  --redis-port <port>
  --minio-port <port>
  --minio-console-port <port>
  --jwt-access-expiry <duration>
  --jwt-refresh-expiry <duration>
  --cors-origins <csv>

Example:
  scripts/instance-local.sh up --name test2 --api-port 18081
EOF
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

ensure_free_port() {
  local port="$1"
  if ss -ltn "( sport = :${port} )" | grep -q LISTEN; then
    echo "Port ${port} is already in use" >&2
    exit 1
  fi
}

random_port() {
  local min="$1"
  local max="$2"
  while true; do
    local candidate
    candidate=$(shuf -i "${min}-${max}" -n 1)
    if ! ss -ltn "( sport = :${candidate} )" | grep -q LISTEN; then
      echo "${candidate}"
      return
    fi
  done
}

project_name_for() {
  local instance_name="$1"
  echo "zentra_${instance_name}"
}

write_env_file() {
  local env_file="$1"
  cat >"${env_file}" <<EOF
APP_ENV=development
INSTANCE_NAME=${INSTANCE_NAME}
API_PORT=${API_PORT}
POSTGRES_PORT=${POSTGRES_PORT}
REDIS_PORT=${REDIS_PORT}
MINIO_PORT=${MINIO_PORT}
MINIO_CONSOLE_PORT=${MINIO_CONSOLE_PORT}
POSTGRES_USER=zentra
POSTGRES_PASSWORD=zentra_secure_password
POSTGRES_DB=zentra
MINIO_ACCESS_KEY=zentra_minio
MINIO_SECRET_KEY=zentra_minio_secret
MINIO_BUCKET_ATTACHMENTS=attachments
MINIO_BUCKET_AVATARS=avatars
MINIO_BUCKET_COMMUNITY=community-assets
CDN_BASE_URL=http://localhost:${MINIO_PORT}
JWT_SECRET=$(openssl rand -hex 32)
JWT_ACCESS_TOKEN_EXPIRY=${JWT_ACCESS_TOKEN_EXPIRY}
JWT_REFRESH_TOKEN_EXPIRY=${JWT_REFRESH_TOKEN_EXPIRY}
ENCRYPTION_KEY=$(openssl rand -hex 32)
CORS_ALLOWED_ORIGINS=${CORS_ALLOWED_ORIGINS}
EOF
}

ACTION="${1:-}"
shift || true

if [[ -z "${ACTION}" ]]; then
  usage
  exit 1
fi

require_cmd docker
require_cmd openssl
require_cmd ss
require_cmd shuf

INSTANCE_NAME=""
API_PORT=""
POSTGRES_PORT=""
REDIS_PORT=""
MINIO_PORT=""
MINIO_CONSOLE_PORT=""
CORS_ALLOWED_ORIGINS="http://localhost:5173,http://127.0.0.1:5173,http://localhost:5174,http://127.0.0.1:5174,http://localhost:3000,http://127.0.0.1:3000"
JWT_ACCESS_TOKEN_EXPIRY="15m"
JWT_REFRESH_TOKEN_EXPIRY="2160h"
PURGE="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --name)
      INSTANCE_NAME="$2"
      shift 2
      ;;
    --api-port)
      API_PORT="$2"
      shift 2
      ;;
    --postgres-port)
      POSTGRES_PORT="$2"
      shift 2
      ;;
    --redis-port)
      REDIS_PORT="$2"
      shift 2
      ;;
    --minio-port)
      MINIO_PORT="$2"
      shift 2
      ;;
    --minio-console-port)
      MINIO_CONSOLE_PORT="$2"
      shift 2
      ;;
    --jwt-access-expiry)
      JWT_ACCESS_TOKEN_EXPIRY="$2"
      shift 2
      ;;
    --jwt-refresh-expiry)
      JWT_REFRESH_TOKEN_EXPIRY="$2"
      shift 2
      ;;
    --cors-origins)
      CORS_ALLOWED_ORIGINS="$2"
      shift 2
      ;;
    --purge)
      PURGE="true"
      shift 1
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ -z "${INSTANCE_NAME}" ]]; then
  echo "--name is required" >&2
  usage
  exit 1
fi

mkdir -p "${INSTANCES_DIR}"
ENV_FILE="${INSTANCES_DIR}/${INSTANCE_NAME}.env"
PROJECT_NAME="$(project_name_for "${INSTANCE_NAME}")"

case "${ACTION}" in
  up)
    API_PORT="${API_PORT:-$(random_port 18080 18999)}"
    POSTGRES_PORT="${POSTGRES_PORT:-$(random_port 15432 15999)}"
    REDIS_PORT="${REDIS_PORT:-$(random_port 16379 16999)}"
    MINIO_PORT="${MINIO_PORT:-$(random_port 19000 19999)}"
    MINIO_CONSOLE_PORT="${MINIO_CONSOLE_PORT:-$(random_port 19100 19999)}"

    ensure_free_port "${API_PORT}"
    ensure_free_port "${POSTGRES_PORT}"
    ensure_free_port "${REDIS_PORT}"
    ensure_free_port "${MINIO_PORT}"
    ensure_free_port "${MINIO_CONSOLE_PORT}"

    write_env_file "${ENV_FILE}"

    docker compose \
      --project-name "${PROJECT_NAME}" \
      --env-file "${ENV_FILE}" \
      -f "${COMPOSE_FILE}" \
      up -d --build

    echo ""
    echo "Instance '${INSTANCE_NAME}' started"
    echo "API:            http://localhost:${API_PORT}"
    echo "Health:         http://localhost:${API_PORT}/health"
    echo "MinIO API:      http://localhost:${MINIO_PORT}"
    echo "MinIO Console:  http://localhost:${MINIO_CONSOLE_PORT}"
    echo "Env file:       ${ENV_FILE}"
    ;;

  down)
    if [[ ! -f "${ENV_FILE}" ]]; then
      echo "No env file found for instance '${INSTANCE_NAME}' (${ENV_FILE})" >&2
      exit 1
    fi

    if [[ "${PURGE}" == "true" ]]; then
      docker compose \
        --project-name "${PROJECT_NAME}" \
        --env-file "${ENV_FILE}" \
        -f "${COMPOSE_FILE}" \
        down -v --remove-orphans
    else
      docker compose \
        --project-name "${PROJECT_NAME}" \
        --env-file "${ENV_FILE}" \
        -f "${COMPOSE_FILE}" \
        down --remove-orphans
    fi

    echo "Instance '${INSTANCE_NAME}' stopped"
    ;;

  logs)
    if [[ ! -f "${ENV_FILE}" ]]; then
      echo "No env file found for instance '${INSTANCE_NAME}' (${ENV_FILE})" >&2
      exit 1
    fi

    docker compose \
      --project-name "${PROJECT_NAME}" \
      --env-file "${ENV_FILE}" \
      -f "${COMPOSE_FILE}" \
      logs -f api
    ;;

  status)
    if [[ ! -f "${ENV_FILE}" ]]; then
      echo "No env file found for instance '${INSTANCE_NAME}' (${ENV_FILE})" >&2
      exit 1
    fi

    docker compose \
      --project-name "${PROJECT_NAME}" \
      --env-file "${ENV_FILE}" \
      -f "${COMPOSE_FILE}" \
      ps
    ;;

  *)
    echo "Unknown action: ${ACTION}" >&2
    usage
    exit 1
    ;;
esac
