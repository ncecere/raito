# Web UI (Dashboard)

Raito includes a browser-based dashboard (built with Vite + React + shadcn/ui). In production deployments, the UI is embedded into the `raito-api` Go binary and served by the same Fiber server that serves the HTTP API.

This doc covers:
- How the embedded UI is built and served
- Local development workflow (`bun run dev`)
- Build/release notes (Go build tags + Docker)

---

## 1. How the UI is served

When the binary is compiled with the `embedwebui` build tag, the built frontend assets under `frontend/dist/` are embedded via `go:embed` and served by the API process.

- UI entrypoint: `GET /` serves `index.html`
- Static assets: `/assets/*` served from the embedded `dist/assets/*`
- SPA routing: unknown paths without a file extension fall back to `index.html`
- API paths are not hijacked:
  - `/v1/*`, `/admin/*`, `/auth/*`, `/healthz`, `/metrics` are handled by the API router as usual

If the binary is **not** compiled with `-tags embedwebui`, the server does not serve any UI routes.

---

## 2. Local development (fast UI iteration)

The recommended workflow during UI development is to run:

1) The Go API server on `:8080`
2) The Vite dev server on `:5173`

### 2.1 Start the API

From `raito/`:

```bash
go run ./cmd/raito-api -config deploy/config/config.yaml -role api
```

### 2.2 Start the frontend dev server

From `raito/frontend/`:

```bash
bun install
bun run dev
```

The Vite dev server proxies `/auth`, `/v1`, and `/admin` to `http://localhost:8080` (see `frontend/vite.config.ts`), so the UI “just works” without CORS issues.

---

## 3. Production builds (embedded UI)

### 3.1 Build locally (non-Docker)

From `raito/frontend/`:

```bash
bun install --frozen-lockfile
bun run build
```

Then from `raito/`:

```bash
go build -tags embedwebui ./cmd/raito-api
```

This produces a `raito-api` binary that serves both:
- the HTTP API (e.g. `/v1/scrape`)
- the Web UI at `/`

### 3.2 Build via Docker

The `raito/Dockerfile` builds the frontend and embeds it automatically:
- builds the UI with Bun (`bun run build`)
- compiles Go with `-tags embedwebui`

So `docker compose up` (from `raito/deploy`) will serve the UI from the same `raito-api` container.

---

## 4. Authentication in the UI

The dashboard uses browser session cookies (issued by the API):
- Local login: `POST /auth/login` (email/password)
- OIDC login: `GET /auth/oidc/login` and `/auth/oidc/callback`
- The login page calls `GET /auth/providers` to determine which login methods are enabled.

Most API endpoints are still accessible via API keys (`Authorization: Bearer ...`). See:
- `docs/auth.md`
- `docs/multi-tenancy.md`

---

## 5. Admin pages

When you log in as a system admin, the UI exposes admin pages (users/tenants/keys/jobs/usage/audit/system settings).

System settings writes updates back to the server config file and may require a restart for some changes to take effect.

