# Peridotite - Zentra Backend

Peridotite is the backend server for Zentra, an encrypted community chat application.

## Tech Stack

- **Go 1.23** - Backend language
- **PostgreSQL 16** - Primary database with partitioned tables
- **Redis 7** - Session storage, caching, pub/sub
- **MinIO** - S3-compatible object storage
- **Chi Router** - HTTP routing
- **gorilla/websocket** - WebSocket connections

## Getting Started

### Prerequisites

- Go 1.23+
- Docker & Docker Compose
- Make (optional)

### Quick Start

1. **Clone the repository**
   ```bash
   git clone https://github.com/zentra-chat/peridotite.git
   cd peridotite
   ```

2. **Set up environment**
   ```bash
   cp .env.example .env
   ```

3. **Start infrastructure**
   ```bash
   docker-compose up -d
   ```

4. **Run migrations**
   ```bash
   # Apply migrations
   make migrate-up
   ```

5. **Start the server**
   ```bash
   go run cmd/gateway/main.go
   ```

The API will be available at `http://localhost:8080`

For full API details, see [API.md](API.md).

## Run a Second Instance (Local Testing)

To test cross-instance identity behavior, launch a second isolated stack (DB/Redis/MinIO/API) on different ports:

```bash
chmod +x scripts/instance-local.sh
scripts/instance-local.sh up --name test2
```

This prints the second instance API URL (for example `http://localhost:18081`).

## Quick Deploy Script (Any Host)

For fast setup on a VM/server with minimal manual config:

```bash
chmod +x scripts/deploy-instance.sh
scripts/deploy-instance.sh --name prod-us --domain https://api.example.com
```

What it does:
- Generates secure secrets and per-instance env config
- Builds and runs an isolated stack using `docker-compose.instance.yml`

If you don’t have a domain yet, you can still run:

```bash
scripts/deploy-instance.sh --name staging
```

### Instance Management Commands

The same script can manage an existing deployed instance:

```bash
# rebuild and relaunch only API
scripts/deploy-instance.sh rebuild-api --name prod-us

# relaunch API without rebuilding image
scripts/deploy-instance.sh relaunch-api --name prod-us

# wipe Postgres data and start fresh
scripts/deploy-instance.sh wipe-db --name prod-us

# pull latest backend changes and restart full stack
scripts/deploy-instance.sh update-restart --name prod-us

# stop the current instance stack
scripts/deploy-instance.sh down --name prod-us
```

Notes:
- `deploy` remains the default action if none is specified.
- Existing `.deploy/<instance>.env` is reused by default so secrets stay stable.
- Use `--force-regenerate-env` with `deploy` only when you intentionally want new secrets/config.

### Development

```bash
# Install dependencies
go mod download

# Run with hot reload (using air)
air
```