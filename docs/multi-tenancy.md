# Multi-Tenancy and Auth

This document explains how authentication, users, tenants, roles, and tenant-scoped workloads work in Raito.

It is aimed at:

- **Integrators** who need to call the API correctly in multi-tenant setups.
- **Developers** working on or around the auth/tenant model.

---

## 1. Auth Overview

Raito supports four complementary auth layers:

1. **API keys** – primary mechanism for automation and service-to-service calls.
2. **Local auth** – email/password login for human users.
3. **OIDC auth** – delegated login via an external identity provider (IdP).
4. **Session cookies** – browser sessions for a future UI, built on JWTs.

### 1.1 API Keys

- API keys are sent via:

  ```http
  Authorization: Bearer raito_<key>
  ```

- The initial admin key is configured in `auth.initialAdminKey` and is used to bootstrap the system.
- System admins can create additional keys via:

  - `POST /admin/api-keys` – creates a non-admin key with a label and optional rate limit.

- Keys are stored in `api_keys` with the following relevant fields:

  - `is_admin` – whether the key maps to a system admin principal.
  - `user_id` – optional user association.
  - `tenant_id` – optional tenant association (tenant-scoped keys).

### 1.2 Local Auth (Email/Password)

- `POST /auth/login`

  - Body: `{ "email": "user@example.com", "password": "..." }`
  - On success:
    - Creates a `users` row and a personal tenant on first login.
    - Issues a session cookie (see below).
    - Returns `{ "success": true, "firstLogin": true|false }`.

- `POST /auth/logout`

  - Clears the session cookie.
  - Returns `{ "success": true }`.

Passwords are hashed (bcrypt) and never stored in plaintext. For development, `bootstrap` config can define local users with plaintext passwords that are hashed at startup.

### 1.3 OIDC Auth

OIDC is configured via `auth.oidc` in `config.yaml`:

- `enabled` – toggles OIDC support.
- `issuerURL`, `clientID`, `clientSecret`, `redirectURL` – IdP configuration.
- `allowedDomains` – optional list of allowed email domains.

Endpoints:

- `GET /auth/oidc/login`
  - Redirects the browser to the IdP, storing a `state` value in a cookie.
- `GET /auth/oidc/callback`
  - Validates `state`, exchanges the code for an ID token, verifies it, and extracts claims.
  - Upserts a `users` row for `(auth_provider=oidc, auth_subject=sub)`.
  - Enforces `allowedDomains` on the email claim.
  - Ensures a personal tenant exists and issues a session cookie.

### 1.4 Session Cookies

Browser sessions are JWTs signed with an HS256 secret configured under `auth.session`:

```yaml
auth:
  session:
    secret: "change_me_session_secret"
    cookieName: "raito_session"
    ttlMinutes: 1440
```

- The session JWT (cookie) contains:

  - `uid` – user ID.
  - `tid` – current tenant ID (default is the personal tenant).
  - `is_admin` – whether the user is a system admin.

- Cookies are issued on:

  - `POST /auth/login` (local auth).
  - `GET /auth/oidc/callback` (OIDC auth).

- Cookies are cleared on `POST /auth/logout`.
- For convenience, `GET /auth/session` returns the same payload as `/v1/me` for UI clients.

---

## 2. Principal and Tenant Context

Internally, Raito builds a `Principal` for each authenticated request:

```go
type Principal struct {
    UserID        *uuid.UUID
    IsSystemAdmin bool
    APIKeyID      *uuid.UUID
    APIKeyTenantID *string
    TenantID      *uuid.UUID
    TenantRole    string
}
```

### 2.1 How Principal Is Derived

- **API key auth** (`Authorization: Bearer raito_<key>`):
  - `authMiddleware` looks up the key and sets:
    - `APIKeyID` from the key ID.
    - `IsSystemAdmin` from `api_keys.is_admin`.
    - `TenantID` and `APIKeyTenantID` from `api_keys.tenant_id` (if present).
    - `UserID` from `api_keys.user_id` (if present).

- **Session cookie auth**:
  - `authMiddleware` parses the JWT session cookie.
  - Sets:
    - `UserID` from `uid` claim.
    - `TenantID` from `tid` claim.
    - `IsSystemAdmin` from `is_admin` claim.

The Principal is attached to the Fiber context as `c.Locals("principal")` and is used by authorization helpers and tenant-scoped logic.

### 2.2 Admin-Only Middleware

- `adminOnlyMiddleware` checks `Principal.IsSystemAdmin` and is applied to the `/admin/*` route group.
- All admin endpoints assume that a Principal is present and represents a system admin.

---

## 3. Tenants and Roles

### 3.1 Tenants

Tenants are stored in the `tenants` table:

- `id` – UUID.
- `slug` – human-readable identifier.
- `name` – display name.
- `type` – `personal` or `org`.
- `owner_user_id` – for personal tenants, the owning user.

Types:

- **personal** – created automatically for each new user on first login (local or OIDC).
- **org** – organizational or application tenants, created explicitly by system admins.

System admins manage tenants via `/admin/tenants`:

- `POST /admin/tenants` – create an org tenant with `{ slug, name, type? }`.
- `GET /admin/tenants` – list tenants (with pagination).
- `GET /admin/tenants/:id` – get tenant details.
- `PATCH /admin/tenants/:id` – update `name` and/or `slug`.

### 3.2 Tenant Membership and Roles

Membership is stored in the `tenant_members` table:

- `tenant_id`, `user_id`, `role`.
- `role` is one of:
  - `tenant_admin`
  - `tenant_member`

Helpers:

- `RequireTenantAdmin(c, p, tenantID)` – ensures the Principal is a system admin or a tenant admin for `tenantID`.
- `RequireTenantMemberOrAdmin(c, p, tenantID)` – ensures the Principal is a system admin, tenant admin, or tenant member.

#### Admin Membership Management

System admins manage membership via `/admin/tenants`:

- `POST /admin/tenants/:id/members` – add or update a member role.
- `PATCH /admin/tenants/:id/members/:userID` – change member role.
- `DELETE /admin/tenants/:id/members/:userID` – remove a member.

#### Tenant-Admin Membership Management (`/v1`)

Tenant admins can manage their tenants via `/v1/tenants`:

- `POST /v1/tenants/:id/members` – add or update a member.
- `PATCH /v1/tenants/:id/members/:userID` – change a member’s role.
- `DELETE /v1/tenants/:id/members/:userID` – remove a member.

These endpoints require the caller to be a system admin or tenant admin for the target tenant.

### 3.3 Discovering and Selecting Tenants

For end-users, tenant selection is handled via `/v1/tenants`:

- `GET /v1/tenants`
  - Returns all tenants the current user belongs to, including their role:

    ```json
    {
      "success": true,
      "tenants": [
        {"id": "...", "slug": "user-alice", "name": "Alice", "type": "personal", "role": "tenant_admin"},
        {"id": "...", "slug": "acme", "name": "Acme Corp", "type": "org", "role": "tenant_admin"}
      ]
    }
    ```

- `POST /v1/tenants/:id/select`
  - Requires the caller to be a member/admin of the tenant.
  - Re-issues the session cookie with `tid` set to the selected tenant.
  - Future requests authenticated via the session cookie will use this tenant as the default context.

---

## 4. Tenant-Scoped Workloads and Jobs

Raito uses a job model for crawl, extract, and batch scrape workloads. Jobs are stored in the `jobs` table and now carry a `tenant_id` column.

### 4.1 Associating Jobs with Tenants

All job creation paths attempt to associate a job with the current tenant:

- Async endpoints:
  - `POST /v1/crawl`
  - `POST /v1/extract` (async)
  - `POST /v1/batch/scrape`

  These use the Principal’s `TenantID` when creating jobs.

- Job-queue-backed sync endpoints:
  - `POST /v1/scrape` (when using the job executor).
  - `POST /v1/map` (when using the job executor).

  These pass a `tenant_id` through the executor’s context when available.

If no tenant context is available, jobs may have `tenant_id = NULL` (legacy or system-wide jobs).

### 4.2 Job Status and Tenant Enforcement

Status endpoints enforce tenant-scoped access for non-admin callers:

- `GET /v1/crawl/:id`
- `GET /v1/extract/:id`
- `GET /v1/batch/scrape/:id`

Rules:

- If `job.tenant_id` is `NULL`:
  - Any authenticated caller can see the job.
- If `job.tenant_id` is set:
  - System admins can always see the job.
  - Non-admins must have `Principal.TenantID == job.tenant_id`.
  - Otherwise the endpoint returns `404 NOT_FOUND` to avoid leaking job existence.

### 4.3 Tenant-Scoped Job Listing

For non-admin users, job listing and detail are tenant-scoped:

- `GET /v1/jobs`
  - Non-admins:
    - Requires a tenant context (from session or tenant-bound API key).
    - Lists jobs for the current tenant only.
  - System admins:
    - Can optionally filter by `?tenantId=<uuid>`.
  - Supports filtering by `type`, `status`, `sync`, `limit`, and `offset`.

- `GET /v1/jobs/:id`
  - Uses the same tenant enforcement as the status endpoints:
    - System admins see all jobs.
    - Non-admins only see jobs for their current tenant.

Example (listing jobs for current tenant):

```bash
curl -H "Authorization: Bearer raito_<tenant_key>" \
  http://localhost:8080/v1/jobs
```

---

## 5. Tenant API Keys

Tenant-scoped API keys allow automation to operate within a single tenant without needing a user/session.

### 5.1 Creating Tenant API Keys

- `POST /v1/tenants/:id/api-keys`

  - Allowed for system admins and tenant admins of the target tenant.
  - Body:

    ```json
    {
      "label": "crawler-key",
      "rateLimitPerMinute": 60
    }
    ```

  - Returns the raw key once:

    ```json
    {
      "success": true,
      "key": "raito_..."
    }
    ```

  - The underlying `api_keys` row has `tenant_id` set to the tenant, so `authMiddleware` will set `Principal.TenantID` appropriately for these keys.

### 5.2 Listing and Revoking Tenant Keys

- `GET /v1/tenants/:id/api-keys`

  - Allowed for system admins and tenant admins of the tenant.
  - Returns metadata (ID, label, isAdmin, createdAt) for active keys.

- `DELETE /v1/tenants/:id/api-keys/:keyID`

  - Allowed for system admins and tenant admins of the tenant.
  - Ensures the key belongs to the tenant before marking it revoked.

Example:

```bash
# Create a tenant-scoped key
curl -X POST \
  -H "Authorization: Bearer raito_<admin_or_tenant_admin_key>" \
  -H "Content-Type: application/json" \
  -d '{"label": "acme-ci", "rateLimitPerMinute": 120}' \
  http://localhost:8080/v1/tenants/<tenant_id>/api-keys
```

When this key is used, `authMiddleware` will infer the tenant from `api_keys.tenant_id`, so no explicit tenant ID needs to be passed in each request.

---

## 6. Tenant Usage

`GET /v1/tenants/:id/usage` provides a simple usage summary for a tenant:

- Response shape:

  ```json
  {
    "success": true,
    "jobs": 42,
    "documents": 123,
    "jobsByType": {
      "crawl": 10,
      "extract": 32
    },
    "documentsByType": {
      "crawl": 40,
      "batch_scrape": 83
    }
  }
  ```

- Filters:

  - `since=<RFC3339>` – only count jobs/documents created at or after this timestamp.
  - `window=24h|7d|30d` – convenience for common time windows (ignored if `since` is provided).

- Access control:

  - System admins can query any tenant.
  - Non-admins must be members/admins of the tenant.

Example:

```bash
curl -H "Authorization: Bearer raito_<tenant_key>" \
  "http://localhost:8080/v1/tenants/<tenant_id>/usage?window=7d"
```

This is useful for dashboards and for monitoring how heavily a given tenant is using crawl/extract/batch workloads.

---

## 7. Putting It Together

- For **automation**:
  - Prefer tenant-scoped API keys (`POST /v1/tenants/:id/api-keys`).
  - Tenant context is inferred from the key; you dont need to pass tenant IDs on every call.

- For **UI / human users**:
  - Use local or OIDC auth to create users and their personal tenants.
  - Use `/v1/tenants` and `/v1/tenants/:id/select` to drive a tenant picker.
  - Use `/v1/jobs`, `/v1/jobs/:id`, and `/v1/tenants/:id/usage` to present tenant-scoped activity and metrics.

- For **admins/operators**:
  - Use `/admin/tenants` and `/admin/tenants/:id/members` for system-wide tenant and membership management.
  - Use `/admin/api-keys` for global key creation, and `/v1/tenants/:id/api-keys` for tenant-scoped keys.

When adding new endpoints or resources, prefer deriving tenant context from Principal and enforcing access via `RequireTenantAdmin`/`RequireTenantMemberOrAdmin` rather than passing tenant IDs directly from clients wherever possible.