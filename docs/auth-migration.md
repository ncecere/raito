# Auth & Multi-Tenancy Rollout / Migration Guide

This document is aimed at operators who already run Raito in production (usually API-key-only) and want to roll out the new user auth, sessions, and multi-tenancy features safely.

It assumes you are comfortable applying database migrations and editing `config.yaml`.

---

## 1. What Changes With This Upgrade?

New versions of Raito add:

- `users`, `tenants`, `tenant_members` tables, and `api_keys.user_id` / `jobs.tenant_id` columns.
- Local auth (email+password), OIDC auth, and JWT-based session cookies.
- Tenant discovery/selection and tenant-scoped job listing.
- Tenant-scoped API keys and per-tenant usage endpoints.

Important characteristics for existing deployments:

- Existing **API keys continue to work** as before.
- Existing **jobs** will have `tenant_id = NULL` and remain visible to any authenticated caller (subject to endpoint-specific rules).
- New features are mostly **opt-in via config** – you can enable them gradually.

---

## 2. Step 0 – Upgrade and Run Migrations

1. **Upgrade the Raito binary / image** to the desired version.
2. **Apply DB migrations** the same way you already do (e.g., via the built-in migrate command or your existing migration tooling).
   - Ensure only **one** process runs migrations at a time to avoid duplicate-type or conflicting DDL errors.
3. Verify the service starts and `/healthz` continues to report `status: "ok"`.

At this point:

- You still rely on API keys (via `Authorization: Bearer raito_<key>`).
- New tables and columns exist but do not change core behavior until you enable user auth and sessions.

---

## 3. Step 1 – Stay in API-Key-Only Mode

If you want to upgrade first and introduce user auth later, keep Raito in API-key-only mode.

In `config.yaml`:

```yaml
auth:
  enabled: true                  # keep this as-is
  initialAdminKey: "change_me_admin_key"  # existing admin key

  local:
    enabled: false               # disable local auth for now

  oidc:
    enabled: false               # disable OIDC for now

  session:
    secret: ""                  # empty or omitted => sessions disabled
    # cookieName: "raito_session"  # optional
    # ttlMinutes: 1440
```

Behavior in this mode:

- Requests are authenticated **only** via API keys.
- Session cookies are ignored (no secret configured).
- New endpoints that expect a user/tenant context (e.g., `/auth/session`, `/v1/tenants`, `/v1/jobs` for UI use) will generally return `401` or `400` and can be ignored until you turn these features on.

You can run in this mode indefinitely while validating the upgrade.

---

## 4. Step 2 – Introduce Users and Sessions (Local Auth)

Once you are ready to let human users log in and see their own tenants/jobs, enable local auth and sessions.

### 4.1 Enable Session Cookies

First, configure `auth.session`:

```yaml
auth:
  enabled: true
  initialAdminKey: "change_me_admin_key"  # keep existing admin key

  session:
    secret: "change_me_session_secret"    # REQUIRED: HS256 signing key
    cookieName: "raito_session"           # optional
    ttlMinutes: 1440                       # 24h session lifetime
```

Notes:

- If `secret` is non-empty, Raito will issue and validate JWT-based session cookies.
- The initial admin API key **continues to work**; it is your safety net if login is misconfigured.

### 4.2 Enable Local Auth for Human Users

Then enable local auth:

```yaml
auth:
  enabled: true

  local:
    enabled: true
```

For development/testing you can also use the `bootstrap` block (see `deploy/config/config.example.yaml`) to seed a local admin user:

```yaml
bootstrap:
  allowPlaintextPasswords: true
  users:
    - email: "dev-admin@example.com"
      name: "Dev Admin"
      isSystemAdmin: true
      provider: "local"
      password: "dev-only-password"
```

With this configuration:

- `POST /auth/login` accepts email/password and issues a session cookie.
- On first login for a new email, Raito creates a `users` row, a **personal tenant**, and a `tenant_members` row with role `tenant_admin`.
- `GET /auth/session` and `/v1/me` expose the current user and their personal tenant to UI clients.

Existing API-key-based clients are unaffected.

---

## 5. Step 3 – (Optional) Introduce OIDC Auth

If you want SSO via an external IdP, configure `auth.oidc`.

```yaml
auth:
  oidc:
    enabled: true
    issuerURL: "https://accounts.example.com"
    clientID: "raito-client-id"
    clientSecret: "raito-client-secret"
    redirectURL: "https://raito.example.com/auth/oidc/callback"
    allowedDomains:
      - "example.com"
```

Behavior:

- `GET /auth/oidc/login` redirects the browser to your IdP.
- `GET /auth/oidc/callback` validates the response, upserts a user, creates a personal tenant (if needed), and issues a session cookie.
- `allowedDomains` can restrict logins to specific email domains.

You can run local auth and OIDC side-by-side or choose one; both ultimately produce the same kind of session cookie and user/tenant records.

---

## 6. Step 4 – Introduce Org Tenants and Tenant-Scoped API Keys

Once users and sessions are working, you can migrate from global API keys to tenant-scoped keys and org tenants.

### 6.1 Create Org Tenants

Using a system admin (via API key or session), create organizational tenants:

- `POST /admin/tenants` with `{ "slug": "acme", "name": "Acme Corp", "type": "org" }`.
- Optionally use the `bootstrap.tenants` block in `config.yaml` to seed tenants and admins at startup.

### 6.2 Create Tenant-Scoped API Keys

For each tenant:

1. Ensure at least one user is a **tenant admin** (via `/admin/tenants/:id/members` or `bootstrap` config).
2. Create tenant-scoped keys:

   ```bash
   curl -X POST \
     -H "Authorization: Bearer raito_<admin_or_tenant_admin_key>" \
     -H "Content-Type: application/json" \
     -d '{"label":"acme-ci","rateLimitPerMinute":120}' \
     http://<host>/v1/tenants/<tenant_id>/api-keys
   ```

3. Update client configs (CI pipelines, services) to use the new tenant-scoped keys.

When a tenant-scoped key is used:

- `authMiddleware` sets `Principal.TenantID` from `api_keys.tenant_id`.
- New jobs created by that key are associated with the tenant via `jobs.tenant_id`.
- Job listing and status endpoints enforce tenant scoping for non-admins.

### 6.3 Decommission Old Global Keys

After clients are updated:

- Use `GET /v1/tenants/:id/api-keys` (and `/admin/api-keys` if present) to audit keys.
- Revoke no-longer-needed keys via `DELETE /v1/tenants/:id/api-keys/:keyID` or the admin APIs.

Existing jobs created with old keys will typically have `tenant_id = NULL` and remain visible to admins; new jobs will be tenant-scoped.

---

## 7. Step 5 – Monitoring and Safe Rollback

### 7.1 What to Monitor

After enabling sessions/local/OIDC and tenant scoping, monitor:

- **HTTP status codes** via `/metrics` or your reverse proxy:
  - Spikes in `401` or `403` may indicate misconfigured auth or missing tenant context.
- **Logs** around login and auth:
  - Failed logins, OIDC errors, and `UNAUTHENTICATED` / `FORBIDDEN` responses.
- **Usage endpoints**:
  - `/v1/tenants/:id/usage` can help you understand which tenants are active.

### 7.2 Rolling Back to API-Key-Only

If you need to revert quickly:

1. Set `auth.local.enabled: false` and `auth.oidc.enabled: false`.
2. Set `auth.session.secret` to an empty string (or remove the block) to disable session cookies.
3. Keep `auth.enabled: true` and `auth.initialAdminKey` unchanged so API keys remain valid.

You do **not** need to roll back DB migrations; the additional tables/columns can remain unused until you re-enable auth features.

---

## 8. Summary

- You can upgrade to the new version and stay in **API-key-only mode** until ready.
- Enabling **sessions + local/OIDC auth** introduces user accounts and personal tenants without breaking existing clients.
- Org tenants and **tenant-scoped API keys** let you gradually segment workloads by tenant.
- Metrics and logs should be monitored for auth-related errors during and after the rollout.

For deeper details on auth flows and tenant behavior, see:

- `docs/auth.md` – local/OIDC auth and session cookies.
- `docs/multi-tenancy.md` – tenants, roles, tenant-scoped keys, and usage.
