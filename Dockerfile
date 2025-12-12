# syntax=docker/dockerfile:1

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

# Build the API binary
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /raito-api ./cmd/raito-api

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates \
    && adduser -D -g '' raito

WORKDIR /app

# Copy binary, config, and migrations
COPY --from=builder /raito-api /app/raito-api
COPY config /app/config
COPY db /app/db

USER raito

EXPOSE 8080

# By default uses config/config.yaml inside the image; override with -config if needed.
ENTRYPOINT ["/app/raito-api"]
