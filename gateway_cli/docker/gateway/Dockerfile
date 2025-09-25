FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o gateway cmd/gateway/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/gateway .
COPY --from=builder /app/.env.example .env

EXPOSE 8080
CMD ["./gateway"]

# Makefile
.PHONY: build run test clean docker-build docker-run dev

# Build the gateway
build:
	go build -o bin/gateway cmd/gateway/main.go

# Run the gateway locally
run: build
	./bin/gateway

# Run with hot reload (requires air: go install github.com/cosmtrek/air@latest)
dev:
	air

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# Docker commands
docker-build:
	docker build -t smart-home-gateway -f Dockerfile.gateway .

docker-run:
	docker-compose up -d

docker-down:
	docker-compose down

# Development setup
setup:
	cp .env.example .env
	go mod tidy
	docker-compose up -d redis

# Run with mock services
mock:
	mkdir -p mock-services/auth mock-services/device-registry
	echo '{"status":"healthy","service":"auth"}' > mock-services/auth/health
	echo '{"status":"healthy","service":"device-registry"}' > mock-services/device-registry/health
	echo '{"message":"Auth service mock"}' > mock-services/auth/index.html
	echo '{"message":"Device registry mock"}' > mock-services/device-registry/index.html
	docker-compose up