# syntax=docker/dockerfile:1

# Build the frontend (Vite) so it can be embedded into the Go binary.
FROM --platform=$BUILDPLATFORM oven/bun:1-alpine AS webui-builder

WORKDIR /app/frontend

# Cache frontend deps
COPY frontend/package.json frontend/bun.lock ./
RUN bun install --frozen-lockfile

# Build frontend
COPY frontend ./
RUN bun run build

# Build stage
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /app

# Install certificates for HTTPS downloads and builds
RUN apk add --no-cache ca-certificates

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source
COPY . .

# Copy the built frontend into place for go:embed patterns.
COPY --from=webui-builder /app/frontend/dist ./frontend/dist

# Build the API binary
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -tags embedwebui -trimpath -ldflags="-s -w" -o /raito-api ./cmd/raito-api

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates chromium curl \
    && adduser -D -g '' raito

WORKDIR /app

# Copy binary, config, and migrations
COPY --from=builder /raito-api /app/raito-api
COPY deploy/config /app/config
COPY db /app/db

USER raito

EXPOSE 8080

# By default uses config/config.yaml inside the image; override with -config if needed.
ENTRYPOINT ["/app/raito-api"]
