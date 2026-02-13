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
- Installs Docker automatically on apt-based machines (unless `--skip-docker-install` is passed)
- Generates secure secrets and per-instance env config
- Builds and runs an isolated stack using `docker-compose.instance.yml`

If you don’t have a domain yet, you can still run:

```bash
scripts/deploy-instance.sh --name staging
```

### Development

```bash
# Install dependencies
go mod download

# Run with hot reload (using air)
air
```