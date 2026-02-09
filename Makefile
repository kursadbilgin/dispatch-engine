.PHONY: build-api build-worker build test lint run-api run-worker clean

# Build
build-api:
	go build -o bin/api ./cmd/api

build-worker:
	go build -o bin/worker ./cmd/worker

build: build-api build-worker

# Run
run-api:
	go run ./cmd/api

run-worker:
	go run ./cmd/worker

# Test
test:
	go test -race -coverprofile=coverage.out ./...

# Lint
lint:
	golangci-lint run ./...

# Clean
clean:
	rm -rf bin/ coverage.out