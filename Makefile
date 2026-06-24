.PHONY: build test test-integration test-race lint docker-up docker-down pipeline-demo

build:
	go build ./...

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

test-race:
	go test -race ./...

lint:
	golangci-lint run

docker-up:
	docker-compose -f deploy/docker-compose.yml up -d

docker-down:
	docker-compose -f deploy/docker-compose.yml down

pipeline-demo:
	go run cmd/pipeline-demo/main.go
