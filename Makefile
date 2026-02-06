.PHONY: build clean run test install docker-build docker-run docker-compose-up docker-compose-down docker-compose-logs

BINARY_NAME=goclaw
BUILD_DIR=bin
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
DOCKER_IMAGE=goclaw
DOCKER_TAG=$(VERSION)

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="-X 'main.Version=$(VERSION)'" -o $(BUILD_DIR)/$(BINARY_NAME) .

clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	go clean

run:
	@echo "Running $(BINARY_NAME)..."
	go run .

test:
	@echo "Running tests..."
	go test -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

install:
	@echo "Installing $(BINARY_NAME)..."
	go install

deps:
	@echo "Downloading dependencies..."
	go mod download

mod-tidy:
	@echo "Tidying go.mod..."
	go mod tidy

fmt:
	@echo "Formatting code..."
	go fmt ./...

lint:
	@echo "Linting code..."
	golangci-lint run ./...

all: clean fmt lint test build

# Docker targets
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest

docker-run:
	@echo "Running Docker container..."
	docker run --rm -it \
		-p 8080:8080 \
		-v $(PWD)/config.json:/home/goclaw/.goclaw/config.json:ro \
		$(DOCKER_IMAGE):latest

docker-compose-up:
	@echo "Starting services with docker-compose..."
	docker-compose up -d

docker-compose-down:
	@echo "Stopping services..."
	docker-compose down

docker-compose-logs:
	@echo "Showing logs..."
	docker-compose logs -f

docker-compose-ps:
	@echo "Showing running services..."
	docker-compose ps

docker-shell:
	@echo "Opening shell in container..."
	docker-compose exec goclaw sh

# Development helpers
dev: docker-compose-up docker-compose-logs

lint-fix:
	@echo "Auto-fixing lint issues..."
	golangci-lint run --fix ./...

# Setup
setup:
	@echo "Setting up development environment..."
	@mkdir -p .goclaw/workspace .goclaw/sessions
	@cp .env.example .env 2>/dev/null || echo "Please copy .env.example to .env and configure"
	@echo "Setup complete. Edit .env with your configuration."

help:
	@echo "Available targets:"
	@echo "  build           - Build the binary"
	@echo "  clean           - Clean build artifacts"
	@echo "  run             - Run the application"
	@echo "  test            - Run tests"
	@echo "  test-coverage   - Run tests with coverage report"
	@echo "  install         - Install the binary to GOPATH/bin"
	@echo "  deps            - Download dependencies"
	@echo "  mod-tidy        - Tidy go.mod"
	@echo "  fmt             - Format code"
	@echo "  lint            - Run linter"
	@echo "  lint-fix        - Auto-fix lint issues"
	@echo "  all             - Run clean, fmt, lint, test, and build"
	@echo ""
	@echo "Docker targets:"
	@echo "  docker-build        - Build Docker image"
	@echo "  docker-run          - Run Docker container"
	@echo "  docker-compose-up   - Start services with docker-compose"
	@echo "  docker-compose-down - Stop services"
	@echo "  docker-compose-logs - Show logs from services"
	@echo "  docker-compose-ps   - Show running services"
	@echo "  docker-shell        - Open shell in container"
	@echo "  dev                - Start development environment"
	@echo ""
	@echo "Setup:"
	@echo "  setup            - Setup development environment"

