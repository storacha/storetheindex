BIN := storetheindex
DOCKER?=$(shell which docker)

.PHONY: all build clean test storetheindex-prod storetheindex-debug docker-prod docker-dev

all: vet test build

build:
	CGO_ENABLED=1 go build

install:
	go install

lint:
	golangci-lint run

test:
	go test ./...

vet:
	go vet ./...

clean:
	go clean
	rm -f ./storetheindex

# Production binary - stripped symbols for smaller size
storetheindex-prod: FORCE
	@echo "Building storetheindex (production)..."
	CGO_ENABLED=1 go build -ldflags="-s -w" -o ./storetheindex

# Debug binary - no optimizations, no inlining, full symbols
storetheindex-debug: FORCE
	@echo "Building storetheindex (debug)..."
	CGO_ENABLED=1 go build -gcflags="all=-N -l" -o ./storetheindex

FORCE:

# Docker targets
docker-prod:
	$(DOCKER) build --target prod -t storetheindex:latest .

docker-dev:
	$(DOCKER) build --target dev -t storetheindex:dev .
