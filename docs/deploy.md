# Deploying Raito

This document explains how to run the Raito API using Docker Compose and how configuration is wired together.

Raito exposes a HTTP API (by default on `:8080`) for scraping, crawling, search, and extraction. The recommended way to run it in production-like environments is via the `deploy/` setup in this repository.

Raito also includes a Web UI (dashboard). When using the Docker image or a binary compiled with the `embedwebui` build tag, the UI is served from the same process at:

- `http://localhost:8080/` – dashboard UI

See `docs/web-ui.md` for details.

## Prerequisites

- Docker and Docker Compose
- Network access to the sites you want to scrape
- Access to a Postgres-compatible database and Redis (provided by the compose file)

---

## Directory layout

Inside the `raito/` repo:

- `deploy/docker-compose.yaml` – all-in-one stack (Postgres, Redis, SearxNG, API, workers)
- `deploy/config/config.yaml` – main configuration used by the containers
- `deploy/config/config.example.yaml` – example config you can copy and modify
- `deploy/searxng/settings.yml` – minimal SearxNG config enabling JSON output

At runtime, the API binary reads YAML config via the `-config` flag (default `config/config.yaml`, relative to the working directory). In the Docker setup we mount `deploy/config` into `/app/config` and run the binary with `-config /app/config/config.yaml`.

---

## Quick start with Docker Compose

From the `raito/` directory:

```bash
cd deploy
# Optional: create your own config based on the example
# cp config/config.example.yaml config/config.yaml

# Start the full stack (Postgres, Redis, SearxNG, API, workers)
docker compose up -d
```

This will start:

- `postgres` – Postgres 16 with `raito` database and user
- `redis` – Redis 7 used for rate limiting
- `searxng` – SearxNG instance used by `/v1/search`
- `api` – Raito API process (`-role api`)
- `worker1`, `worker2` – background worker processes (`-role worker`)

Once the containers are healthy, the API will be reachable at:

- `http://localhost:8080` – dashboard UI (and API base URL)
- `http://localhost:8080/healthz` – health check
- `http://localhost:8080/metrics` – Prometheus-style metrics

To follow logs:

```bash
cd deploy
docker compose logs -f api worker1 worker2
```

To stop everything:

```bash
cd deploy
docker compose down
```

---

## Configuration: deploy/config/config.yaml

The file `deploy/config/config.yaml` is the canonical config used by `deploy/docker-compose.yaml`. It is copied into the image at build time and also mounted into `/app/config` at runtime.

Key sections:

- `server` – host and port the HTTP server binds to (inside the container this is usually `0.0.0.0:8080`).
- `database.dsn` – Postgres DSN; the default uses the `postgres` service name from the compose file:

  ```yaml
  database:
    dsn: "postgres://raito:raito@postgres:5432/raito?sslmode=disable"
  ```

- `redis.url` – Redis connection URL; the default uses the `redis` service:

  ```yaml
  redis:
    url: "redis://redis:6379"
  ```

- `auth` – enables API key auth and configures an initial admin key:

  ```yaml
  auth:
    enabled: true
    initialAdminKey: "change_me_admin_key"
  ```

  On startup, Raito ensures an admin API key with this value exists.

- `search` – controls the optional `/v1/search` endpoint and provider; in the default compose setup this is SearxNG running at `http://searxng:8080`.

- `llm` – configures LLM providers for features like `summary`, `json`, and `branding` formats on `/v1/scrape` and `/v1/extract`. You typically set the API keys via environment variables, for example:

  ```yaml
  llm:
    defaultProvider: "openai"
    openai:
      apiKey: "${OPENAI_API_KEY}"
      baseURL: "https://api.openai.com/v1"
      model: "gpt-4.1-mini"
  ```

In production you should:

- Change `auth.initialAdminKey` to a strong value.
- Point `database.dsn` and `redis.url` at your production services (or adjust the compose file).
- Configure an appropriate LLM provider and models.

---

## Using the published Docker image

You can also run Raito using the published image without the provided compose stack.

```bash
# Example: run Raito with a host-mounted config directory
mkdir -p ./config
# create ./config/config.yaml based on deploy/config/config.example.yaml

docker run --rm -p 8080:8080 \
  -v "$(pwd)/config:/app/config" \
  ghcr.io/ncecere/raito-api:latest \
  /app/raito-api -config /app/config/config.yaml
```

In this mode you are responsible for providing Postgres, Redis, and (optionally) SearxNG and LLM credentials.

---

## Running with Go locally

For local development without containers for the API:

1. Start Postgres and Redis using the same compose file:

   ```bash
   cd raito/deploy
   docker compose up -d postgres redis
   ```

2. Ensure `deploy/config/config.yaml` points at `postgres` and `redis` (as in the default).

3. From the `raito/` directory, run the API server using that config:

   ```bash
   cd raito
   go run ./cmd/raito-api -config deploy/config/config.yaml
   ```

4. Create an admin or user API key via `/admin/api-keys` (see `docs/usage.md`).

This gives you a local development setup that matches the Docker Compose environment while still running the Go binary directly on your machine.
