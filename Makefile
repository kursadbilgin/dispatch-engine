.PHONY: build test lint run clean docker-up docker-down

# Build
build:
	go build -o bin/api ./cmd/api

# Run
run:
	go run ./cmd/api

# Test
test:
	go test -race -coverprofile=coverage.out ./...

# Lint
lint:
	golangci-lint run ./...

# Docker
docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

# Clean
clean:
	rm -rf bin/ coverage.out
