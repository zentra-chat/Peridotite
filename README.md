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

### Development

```bash
# Install dependencies
go mod download

# Run with hot reload (using air)
air
```

## Database Schema

Tables:
- `users` - User accounts and profiles
- `communities` - Community/server information
- `channels` - Text/voice channels
- `messages` - Partitioned by month for performance
- `roles` - Permission-based role system
- `community_members` - Member-community relationships