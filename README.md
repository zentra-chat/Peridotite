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
   docker compose up -d postgres redis minio
   ```

4. **Run migrations**
   ```bash
   docker compose run --rm migrate up
   ```

5. **Start the server**
   ```bash
   docker compose up -d --build api
   ```

The API will be available at `http://localhost:8080`

For full API details, see [API.md](API.md).

## Docker Compose workflow

Use standard Docker Compose commands for local development and most deployments:

```bash
# start full stack
docker compose up -d --build

# inspect state
docker compose ps

# follow API logs
docker compose logs -f api

# stop stack
docker compose down
```

To remove containers and volumes:

```bash
docker compose down -v
```

## Running migrations

Use the dedicated migration container:

```bash
# apply all pending migrations
docker compose run --rm migrate up

# rollback one migration
docker compose run --rm migrate down 1

# rollback three migrations
docker compose run --rm migrate down 3
```

Run migrations after PostgreSQL is up and before launching API changes that depend on new schema.

### Development

```bash
# Install dependencies
go mod download

# Run with hot reload (using air)
air
```