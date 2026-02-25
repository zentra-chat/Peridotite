#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
DEPLOY_DIR="${BACKEND_DIR}/.deploy"
COMPOSE_FILE="${BACKEND_DIR}/docker-compose.instance.yml"

usage() {
  cat <<'EOF'
Usage:
  scripts/deploy-instance.sh --name <instance> [options]

Options:
  --api-port <port>                 API port (default: 8080)
  --postgres-port <port>            Postgres port (default: 5432)
  --redis-port <port>               Redis port (default: 6379)
  --minio-port <port>               MinIO API port (default: 9000)
  --minio-console-port <port>       MinIO console port (default: 9001)
  --domain <url>                    Public API base URL (e.g. https://api.example.com)
  --cors-origins <csv>              CORS origins list
  --discord-import-token <token>    Optional Discord import token
  --skip-docker-install             Skip docker installation checks

Example:
  scripts/deploy-instance.sh --name prod-us --domain https://api.chat.example

This script is intended to run on the deployment machine inside the backend directory.
EOF
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

install_docker_if_missing() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    return
  fi

  echo "Docker / Docker Compose not found. Attempting automatic install..."
  if command -v apt-get >/dev/null 2>&1; then
    sudo apt-get update
    sudo apt-get install -y ca-certificates curl gnupg lsb-release
    curl -fsSL https://get.docker.com | sh
    sudo usermod -aG docker "${USER}" || true
  else
    echo "Automatic Docker install is only scripted for apt-based systems." >&2
    echo "Install Docker manually, then rerun this script." >&2
    exit 1
  fi
}

INSTANCE_NAME=""
API_PORT="63566"
POSTGRES_PORT="5432"
REDIS_PORT="6379"
MINIO_PORT="9000"
MINIO_CONSOLE_PORT="9001"
DOMAIN=""
CORS_ALLOWED_ORIGINS="*"
DISCORD_IMPORT_TOKEN=""
SKIP_DOCKER_INSTALL="false"

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
    --domain)
      DOMAIN="$2"
      shift 2
      ;;
    --cors-origins)
      CORS_ALLOWED_ORIGINS="$2"
      shift 2
      ;;
    --discord-import-token)
      DISCORD_IMPORT_TOKEN="$2"
      shift 2
      ;;
    --skip-docker-install)
      SKIP_DOCKER_INSTALL="true"
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

if [[ "${SKIP_DOCKER_INSTALL}" != "true" ]]; then
  install_docker_if_missing
fi

require_cmd docker
require_cmd openssl

if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose plugin is required. Install it and rerun." >&2
  exit 1
fi

mkdir -p "${DEPLOY_DIR}"
ENV_FILE="${DEPLOY_DIR}/${INSTANCE_NAME}.env"
PROJECT_NAME="zentra_${INSTANCE_NAME}"

if [[ -z "${CORS_ALLOWED_ORIGINS}" ]]; then
  if [[ -n "${DOMAIN}" ]]; then
    CORS_ALLOWED_ORIGINS="${DOMAIN}"
  else
    CORS_ALLOWED_ORIGINS="*"
  fi
fi

if [[ -n "${DOMAIN}" ]]; then
  CDN_BASE_URL="${DOMAIN}"
else
  HOST_IP=$(hostname -I | awk '{print $1}')
  CDN_BASE_URL="http://${HOST_IP}:${MINIO_PORT}"
fi

cat >"${ENV_FILE}" <<EOF
APP_ENV=production
INSTANCE_NAME=${INSTANCE_NAME}
API_PORT=${API_PORT}
POSTGRES_PORT=${POSTGRES_PORT}
REDIS_PORT=${REDIS_PORT}
MINIO_PORT=${MINIO_PORT}
MINIO_CONSOLE_PORT=${MINIO_CONSOLE_PORT}
POSTGRES_USER=zentra
POSTGRES_PASSWORD=$(openssl rand -hex 16)
POSTGRES_DB=zentra
MINIO_ACCESS_KEY=zentra_minio
MINIO_SECRET_KEY=$(openssl rand -hex 16)
MINIO_BUCKET_ATTACHMENTS=attachments
MINIO_BUCKET_AVATARS=avatars
MINIO_BUCKET_COMMUNITY=community-assets
CDN_BASE_URL=${CDN_BASE_URL}
JWT_SECRET=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -hex 32)
CORS_ALLOWED_ORIGINS=${CORS_ALLOWED_ORIGINS}
DISCORD_IMPORT_TOKEN=${DISCORD_IMPORT_TOKEN}
EOF

echo "Deploying '${INSTANCE_NAME}'..."
docker compose \
  --project-name "${PROJECT_NAME}" \
  --env-file "${ENV_FILE}" \
  -f "${COMPOSE_FILE}" \
  up -d --build

echo ""
echo "Deployment complete"
echo "API URL:        http://localhost:${API_PORT}"
echo "Health URL:     http://localhost:${API_PORT}/health"
echo "MinIO API URL:  http://localhost:${MINIO_PORT}"
echo "MinIO Console:  http://localhost:${MINIO_CONSOLE_PORT}"
echo "Project Name:   ${PROJECT_NAME}"
echo "Env File:       ${ENV_FILE}"
echo ""
echo "Manage this instance with:"
echo "  docker compose --project-name ${PROJECT_NAME} --env-file ${ENV_FILE} -f ${COMPOSE_FILE} ps"
echo "  docker compose --project-name ${PROJECT_NAME} --env-file ${ENV_FILE} -f ${COMPOSE_FILE} logs -f api"
echo "  docker compose --project-name ${PROJECT_NAME} --env-file ${ENV_FILE} -f ${COMPOSE_FILE} down"
