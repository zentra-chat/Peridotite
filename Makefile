.PHONY: all build run test clean docker-up docker-down migrate help instance-up instance-down instance-logs deploy-instance

# Variables
BINARY_NAME=gateway
BUILD_DIR=bin
CMD_DIR=cmd/gateway

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOFMT=gofmt

# Build tags
BUILD_TAGS=
LDFLAGS=-ldflags "-s -w"

all: clean build

## build: Build the application binary
build:
	@echo "Building..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)/main.go
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

## run: Run the application
run:
	@echo "Running..."
	$(GORUN) $(CMD_DIR)/main.go

## test: Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -cover ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

## lint: Run linter
lint:
	@echo "Linting..."
	@golangci-lint run ./...

## docker-up: Start Docker containers
docker-up:
	@echo "Starting Docker containers..."
	docker-compose up -d

## docker-down: Stop Docker containers
docker-down:
	@echo "Stopping Docker containers..."
	docker-compose down

## docker-logs: View Docker logs
docker-logs:
	docker-compose logs -f

## instance-up: Start a named isolated instance stack (INSTANCE=name)
instance-up:
	@if [ -z "$(INSTANCE)" ]; then echo "Usage: make instance-up INSTANCE=test2"; exit 1; fi
	@chmod +x scripts/instance-local.sh
	@./scripts/instance-local.sh up --name $(INSTANCE)

## instance-down: Stop a named isolated instance stack (INSTANCE=name)
instance-down:
	@if [ -z "$(INSTANCE)" ]; then echo "Usage: make instance-down INSTANCE=test2"; exit 1; fi
	@chmod +x scripts/instance-local.sh
	@./scripts/instance-local.sh down --name $(INSTANCE)

## instance-logs: Stream API logs from a named isolated instance stack (INSTANCE=name)
instance-logs:
	@if [ -z "$(INSTANCE)" ]; then echo "Usage: make instance-logs INSTANCE=test2"; exit 1; fi
	@chmod +x scripts/instance-local.sh
	@./scripts/instance-local.sh logs --name $(INSTANCE)

## deploy-instance: Deploy a named instance on current machine (INSTANCE=name)
deploy-instance:
	@if [ -z "$(INSTANCE)" ]; then echo "Usage: make deploy-instance INSTANCE=prod-us"; exit 1; fi
	@chmod +x scripts/deploy-instance.sh
	@./scripts/deploy-instance.sh --name $(INSTANCE)

## migrate-up: Run database migrations
migrate-up:
	@echo "Running migrations..."
	@chmod +x scripts/migrate.sh
	@. ./.env && ./scripts/migrate.sh

## migrate-down: Rollback database migrations
migrate-down:
	@echo "Rolling back migrations..."
	@. ./.env && psql $$DATABASE_URL < migrations/000001_initial_schema.down.sql

## setup: Full development setup
setup:
	@chmod +x scripts/setup.sh
	@./scripts/setup.sh

## help: Show this help message
help:
	@echo "Peridotite - Zentra Backend"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'
