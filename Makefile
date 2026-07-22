.PHONY: lint test test-integration test-e2e test-race build docker clean run coverage swagger docker-up docker-down pipeline-demo mcp-server mcp-client verify-all test-coverage

# Lint
lint:
	golangci-lint run --timeout=5m

# Unit tests
test:
	go test -v -race -coverprofile=coverage.out ./...

# Integration tests (需要 Docker)
test-integration:
	go test -v -tags=integration -timeout=10m ./...

# E2E tests
test-e2e:
	go test -tags=e2e -v -count=1 ./test/e2e/...

# Race detection
test-race:
	go test -race ./...

# Build
build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/server ./cmd/server
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/migrate-data ./cmd/migrate-data

# Docker build
docker:
	docker build -t network-digital-twin:latest .

# Run locally
run:
	go run ./cmd/server

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html

# Coverage report
coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Swagger doc generation
swagger:
	swag init -g cmd/server/main.go -o docs/swagger

# Docker compose
docker-up:
	docker-compose -f deploy/docker-compose.yml up -d

docker-down:
	docker-compose -f deploy/docker-compose.yml down

# Pipeline demo
pipeline-demo:
	go run cmd/pipeline-demo/main.go

# MCP server/client
mcp-server:
	go run cmd/server/main.go

mcp-client:
	go run cmd/mcp-client/main.go

# Test coverage (simple)
test-coverage:
	go test -cover ./...

# Full verification
verify-all: build lint test test-race test-e2e
	@echo "=== All checks passed ==="
