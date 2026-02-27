#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKEND_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
DEPLOY_DIR="${BACKEND_DIR}/.deploy"
COMPOSE_FILE="${BACKEND_DIR}/docker-compose.instance.yml"

ACTION="deploy"

usage() {
  cat <<'EOF'
Usage:
  scripts/deploy-instance.sh [action] --name <instance> [options]

Actions:
  deploy           Create/update env and deploy full stack (default)
  rebuild-api      Rebuild and restart only the API service
  relaunch-api     Restart API container without rebuilding
  wipe-db          Delete Postgres data volume, then start stack again
  update-restart   git pull (ff-only), then rebuild and restart full stack
  down             Stop current instance stack

If no action is provided, "deploy" is used.

Options:
  --api-port <port>                 API port (default: 63566)
  --postgres-port <port>            Postgres port (default: 5432)
  --redis-port <port>               Redis port (default: 6379)
  --minio-port <port>               MinIO API port (default: 9000)
  --minio-console-port <port>       MinIO console port (default: 9001)
  --domain <url>                    Public API base URL (e.g. https://api.example.com)
  --cors-origins <csv>              CORS origins list
  --discord-import-token <token>    Optional Discord import token
  --force-regenerate-env            Recreate env file (new secrets)

Example:
  scripts/deploy-instance.sh deploy --name prod-us --domain https://api.chat.example
  scripts/deploy-instance.sh rebuild-api --name prod-us
  scripts/deploy-instance.sh wipe-db --name prod-us

This script is intended to run on the deployment machine inside the backend directory.
EOF
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

project_name_for() {
  local instance_name="$1"
  echo "zentra_${instance_name}"
}

compose_cmd() {
  docker compose \
    --project-name "${PROJECT_NAME}" \
    --env-file "${ENV_FILE}" \
    -f "${COMPOSE_FILE}" \
    "$@"
}

load_env_file() {
  if [[ -f "${ENV_FILE}" ]]; then
    set -a
    # shellcheck disable=SC1090
    source "${ENV_FILE}"
    set +a
  fi
}

write_env_file() {
  cat >"${ENV_FILE}" <<EOF
APP_ENV=production
INSTANCE_NAME=${INSTANCE_NAME}
API_PORT=${API_PORT}
POSTGRES_PORT=${POSTGRES_PORT}
REDIS_PORT=${REDIS_PORT}
MINIO_PORT=${MINIO_PORT}
MINIO_CONSOLE_PORT=${MINIO_CONSOLE_PORT}
POSTGRES_USER=${POSTGRES_USER}
POSTGRES_PASSWORD=${POSTGRES_PASSWORD}
POSTGRES_DB=${POSTGRES_DB}
MINIO_ACCESS_KEY=${MINIO_ACCESS_KEY}
MINIO_SECRET_KEY=${MINIO_SECRET_KEY}
MINIO_BUCKET_ATTACHMENTS=${MINIO_BUCKET_ATTACHMENTS}
MINIO_BUCKET_AVATARS=${MINIO_BUCKET_AVATARS}
MINIO_BUCKET_COMMUNITY=${MINIO_BUCKET_COMMUNITY}
CDN_BASE_URL=${CDN_BASE_URL}
JWT_SECRET=${JWT_SECRET}
ENCRYPTION_KEY=${ENCRYPTION_KEY}
CORS_ALLOWED_ORIGINS=${CORS_ALLOWED_ORIGINS}
DISCORD_IMPORT_TOKEN=${DISCORD_IMPORT_TOKEN}
EOF
}

ensure_env_for_management() {
  if [[ ! -f "${ENV_FILE}" ]]; then
    echo "No env file found for instance '${INSTANCE_NAME}' (${ENV_FILE})" >&2
    echo "Run deploy first: scripts/deploy-instance.sh deploy --name ${INSTANCE_NAME}" >&2
    exit 1
  fi
  load_env_file
}

run_deploy() {
  mkdir -p "${DEPLOY_DIR}"

  if [[ -f "${ENV_FILE}" && "${FORCE_REGENERATE_ENV}" != "true" ]]; then
    echo "Using existing env file: ${ENV_FILE}"
    load_env_file

    API_PORT="${API_PORT:-63566}"
    POSTGRES_PORT="${POSTGRES_PORT:-5432}"
    REDIS_PORT="${REDIS_PORT:-6379}"
    MINIO_PORT="${MINIO_PORT:-9000}"
    MINIO_CONSOLE_PORT="${MINIO_CONSOLE_PORT:-9001}"
    CORS_ALLOWED_ORIGINS="${CORS_ALLOWED_ORIGINS:-*}"
    DISCORD_IMPORT_TOKEN="${DISCORD_IMPORT_TOKEN:-}"
    POSTGRES_USER="${POSTGRES_USER:-zentra}"
    POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-$(openssl rand -hex 16)}"
    POSTGRES_DB="${POSTGRES_DB:-zentra}"
    MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-zentra_minio}"
    MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-$(openssl rand -hex 16)}"
    MINIO_BUCKET_ATTACHMENTS="${MINIO_BUCKET_ATTACHMENTS:-attachments}"
    MINIO_BUCKET_AVATARS="${MINIO_BUCKET_AVATARS:-avatars}"
    MINIO_BUCKET_COMMUNITY="${MINIO_BUCKET_COMMUNITY:-community-assets}"
    JWT_SECRET="${JWT_SECRET:-$(openssl rand -hex 32)}"
    ENCRYPTION_KEY="${ENCRYPTION_KEY:-$(openssl rand -hex 32)}"
    if [[ -n "${DOMAIN}" ]]; then
      CDN_BASE_URL="${DOMAIN}"
    else
      CDN_BASE_URL="${CDN_BASE_URL:-}"
      if [[ -z "${CDN_BASE_URL}" ]]; then
        HOST_IP=$(hostname -I | awk '{print $1}')
        CDN_BASE_URL="http://${HOST_IP}:${MINIO_PORT}"
      fi
    fi
  else
    API_PORT="${API_PORT:-63566}"
    POSTGRES_PORT="${POSTGRES_PORT:-5432}"
    REDIS_PORT="${REDIS_PORT:-6379}"
    MINIO_PORT="${MINIO_PORT:-9000}"
    MINIO_CONSOLE_PORT="${MINIO_CONSOLE_PORT:-9001}"

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

    POSTGRES_USER="zentra"
    POSTGRES_PASSWORD="$(openssl rand -hex 16)"
    POSTGRES_DB="zentra"
    MINIO_ACCESS_KEY="zentra_minio"
    MINIO_SECRET_KEY="$(openssl rand -hex 16)"
    MINIO_BUCKET_ATTACHMENTS="attachments"
    MINIO_BUCKET_AVATARS="avatars"
    MINIO_BUCKET_COMMUNITY="community-assets"
    JWT_SECRET="$(openssl rand -hex 32)"
    ENCRYPTION_KEY="$(openssl rand -hex 32)"
  fi

  write_env_file

  echo "Deploying '${INSTANCE_NAME}'..."
  compose_cmd up -d --build

  echo ""
  echo "Deployment complete"
  echo "API URL:        http://localhost:${API_PORT}"
  echo "Health URL:     http://localhost:${API_PORT}/health"
  echo "MinIO API URL:  http://localhost:${MINIO_PORT}"
  echo "MinIO Console:  http://localhost:${MINIO_CONSOLE_PORT}"
  echo "Project Name:   ${PROJECT_NAME}"
  echo "Env File:       ${ENV_FILE}"
}

run_rebuild_api() {
  ensure_env_for_management
  echo "Rebuilding and relaunching API for '${INSTANCE_NAME}'..."
  compose_cmd up -d --build api
  echo "API rebuild/relaunch complete"
}

run_relaunch_api() {
  ensure_env_for_management
  echo "Relaunching API for '${INSTANCE_NAME}'..."
  compose_cmd restart api
  echo "API relaunched"
}

run_wipe_db() {
  ensure_env_for_management
  echo "Stopping stack before DB wipe..."
  compose_cmd down --remove-orphans

  local volume_name
  volume_name="${PROJECT_NAME}_postgres_data"
  echo "Removing DB volume ${volume_name}..."
  docker volume rm "${volume_name}" >/dev/null 2>&1 || true

  echo "Recreating stack with fresh database..."
  compose_cmd up -d --build
  echo "DB wipe complete; stack is running"
}

run_update_restart() {
  ensure_env_for_management
  require_cmd git

  echo "Updating source from git (ff-only)..."
  git -C "${BACKEND_DIR}" pull --ff-only

  echo "Rebuilding and restarting stack..."
  compose_cmd up -d --build
  echo "Update + restart complete"
}

run_down() {
  ensure_env_for_management
  echo "Taking down instance '${INSTANCE_NAME}'..."
  compose_cmd down --remove-orphans
  echo "Instance '${INSTANCE_NAME}' is down"
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
FORCE_REGENERATE_ENV="false"

if [[ $# -gt 0 && "${1}" != --* ]]; then
  ACTION="$1"
  shift
fi

while [[ $# -gt 0 ]]; do
  case "$1" in
    --help|-h)
      usage
      exit 0
      ;;
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
    --force-regenerate-env)
      FORCE_REGENERATE_ENV="true"
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

require_cmd docker
require_cmd openssl

if ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose plugin is required. Install it and rerun." >&2
  exit 1
fi

mkdir -p "${DEPLOY_DIR}"
ENV_FILE="${DEPLOY_DIR}/${INSTANCE_NAME}.env"
PROJECT_NAME="$(project_name_for "${INSTANCE_NAME}")"

case "${ACTION}" in
  deploy)
    run_deploy
    ;;
  rebuild-api)
    run_rebuild_api
    ;;
  relaunch-api)
    run_relaunch_api
    ;;
  wipe-db)
    run_wipe_db
    ;;
  update-restart)
    run_update_restart
    ;;
  down)
    run_down
    ;;
  *)
    echo "Unknown action: ${ACTION}" >&2
    usage
    exit 1
    ;;
esac

echo ""
echo "Manage this instance with:"
echo "  docker compose --project-name ${PROJECT_NAME} --env-file ${ENV_FILE} -f ${COMPOSE_FILE} ps"
echo "  docker compose --project-name ${PROJECT_NAME} --env-file ${ENV_FILE} -f ${COMPOSE_FILE} logs -f api"
echo "  scripts/deploy-instance.sh rebuild-api --name ${INSTANCE_NAME}"
echo "  scripts/deploy-instance.sh relaunch-api --name ${INSTANCE_NAME}"
echo "  scripts/deploy-instance.sh wipe-db --name ${INSTANCE_NAME}"
echo "  scripts/deploy-instance.sh update-restart --name ${INSTANCE_NAME}"
echo "  scripts/deploy-instance.sh down --name ${INSTANCE_NAME}"
