.PHONY: build run test clean docker-build migrate

APP_NAME := rdapd
BUILD_DIR := build
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -ldflags="-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

VERSION ?= 1.0.0

build:
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BUILD_DIR)/$(APP_NAME) ./cmd/rdapd

run: build
	./$(BUILD_DIR)/$(APP_NAME) -config config.yaml

test:
	go test -v -race -count=1 ./...

test-cover:
	go test -v -race -coverprofile=coverage.out -count=1 ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

clean:
	rm -rf $(BUILD_DIR) coverage.out coverage.html

docker-build:
	docker build -t rdap-server:$(VERSION) -t rdap-server:latest .

docker-run:
	docker-compose up -d

docker-stop:
	docker-compose down

migrate-up:
	go run ./cmd/migrate -direction up

migrate-down:
	go run ./cmd/migrate -direction down

tidy:
	go mod tidy
	go mod verify

fmt:
	go fmt ./...
