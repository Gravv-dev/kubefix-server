# syntax=docker/dockerfile:1
# Build stage: compile the Go backend binary.
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Download dependencies first (layer caching).
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a statically linked binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/server main.go

# Run stage: minimal image.
FROM alpine:3.20

WORKDIR /
COPY --from=builder /bin/server /bin/server

# The app reads all config from environment variables (see .env.example).
EXPOSE 8080

USER nobody:nobody
ENTRYPOINT ["/bin/server"]
