# Auth Flows (Local, OIDC, and Sessions)

This document focuses on user authentication flows in Raito: how local auth, OIDC auth, and session cookies work together, and how they relate to API keys.

For multi-tenancy, tenant roles, and tenant-scoped API keys/usage, see `docs/multi-tenancy.md`.

---

## 1. Auth Modes Overview

Raito supports two major ways to authenticate requests:

1. **API keys** – for automation and service-to-service calls.
2. **User auth + session cookies** – for human users and browser-based UIs.

### 1.1 API Keys (Recap)

- API keys are sent via:

  ```http
  Authorization: Bearer raito_<key>
  ```

- The initial admin key is configured via `auth.initialAdminKey` in `config.yaml`.
- Admins can create additional keys using:

  - `POST /admin/api-keys` – creates a non-admin key with a label and optional per-minute rate limit.

- Keys live in the `api_keys` table with the following important fields:

  - `is_admin` – whether the key maps to a system admin.
  - `user_id` – optional user association.
  - `tenant_id` – optional tenant association for tenant-scoped keys.

API keys are the primary mechanism for CLI tools, CI pipelines, and other automation.

---

## 2. Local Auth (Email + Password)

Local auth lets you log in human users with email/password credentials.

### 2.1 Configuration

Local auth is enabled via the `auth.local` block in `config.yaml`:

```yaml
auth:
  enabled: true
  local:
    enabled: true
```

You can bootstrap an initial local admin user in development using the `bootstrap` config (see `docs/config.md` and `docs/multi-tenancy.md`).

### 2.2 Endpoints

#### `POST /auth/login`

- Request body:

  ```json
  {
    "email": "user@example.com",
    "password": "your-password"
  }
  ```

- Behavior:

  - Verifies the password against a bcrypt hash stored in the `users` table.
  - On **first successful login** for a new email:
    - Creates a `users` row.
    - Creates a personal tenant (type `personal`, `owner_user_id = user.id`).
    - Adds a `tenant_members` row with role `tenant_admin`.
  - Issues a session cookie (see Section 4) attached to the response.

- Response:

  ```json
  {
    "success": true,
    "firstLogin": true
  }
  ```

  `firstLogin` is `true` only when the user and their personal tenant were created.

#### `POST /auth/logout`

- Clears the session cookie and returns:

  ```json
  { "success": true }
  ```

This endpoint does not revoke API keys; it only affects browser sessions.

---

## 3. OIDC Auth (External IdP)

OIDC auth allows users to log in via an external identity provider (IdP), such as Auth0, Okta, or a self-hosted OIDC provider.

### 3.1 Configuration

In `config.yaml`:

```yaml
auth:
  oidc:
    enabled: true
    issuerURL: "https://accounts.example.com"
    clientID: "raito-client-id"
    clientSecret: "raito-client-secret"
    redirectURL: "http://localhost:8080/auth/oidc/callback"
    allowedDomains:
      - "example.com"
```

- `issuerURL` – the OIDC issuer (well-known discovery endpoint).
- `clientID`, `clientSecret` – client credentials registered with the IdP.
- `redirectURL` – must match what’s configured in the IdP.
- `allowedDomains` – optional; restricts logins to specific email domains.

### 3.2 Endpoint Flow

#### 1) `GET /auth/oidc/login`

- Redirects the user to the IdP’s authorization endpoint.
- Generates a random `state` value, stores it in a cookie, and includes it in the redirect URL.

Usage from a browser (simplified):

```bash
# In a browser or web app, navigate to:
http://localhost:8080/auth/oidc/login
```

#### 2) IdP redirects to `/auth/oidc/callback`

- Raito receives `code` and `state` query parameters.
- Validates that:
  - `state` matches the cookie.
  - The code can be exchanged for tokens.
- Verifies the ID token and extracts claims (including `email` and `sub`).

User handling:

- Attempts to find an existing user by `(auth_provider=oidc, auth_subject=sub)`.
- If not found:
  - Optionally checks `email` domain against `allowedDomains`.
  - Creates a new `users` row.
  - Creates a personal tenant and `tenant_members` row as in local auth.

Session handling:

- Issues a session cookie for the user (see Section 4).

For browser-based flows, the callback redirects back to `/` (the dashboard). For non-browser clients, the handler returns JSON:

```json
{
  "success": true,
  "firstLogin": true
}
```

`firstLogin` is `true` when the OIDC user and their personal tenant are created for the first time.

### 3.3 UI provider discovery

The dashboard login page calls `GET /auth/providers` to discover which auth providers are enabled (local and/or OIDC) so it can render the correct sign-in options without hardcoding configuration.

---

## 4. Session Cookies (JWT-Based)

### 4.1 Configuration

Session cookies are configured via `auth.session`:

```yaml
auth:
  session:
    secret: "change_me_session_secret"   # HS256 signing key
    cookieName: "raito_session"          # optional; default is "raito_session"
    ttlMinutes: 1440                      # session lifetime in minutes (24h)
```

- If `secret` is empty, session cookies are disabled (API-key-only mode).

### 4.2 Cookie Contents

Raito uses an HS256 JWT stored in a cookie to represent the session:

- Claims:

  - `uid` – user ID.
  - `tid` – current tenant ID (defaults to personal tenant).
  - `is_admin` – whether the user is a system admin.
  - Standard JWT fields: `iat`, `exp`.

The cookie is:

- `HTTPOnly` – not accessible from JavaScript.
- `Secure` – should be sent only over HTTPS (in production).
- `SameSite=Lax`.

### 4.3 Issuance and Parsing

- **Issued on**:
  - `POST /auth/login` (local auth).
  - `GET /auth/oidc/callback` (OIDC auth).

- **Cleared on**:
  - `POST /auth/logout`.

- **Used by**:
  - `authMiddleware` as an alternative to API keys.
  - `GET /auth/session` to expose the current user and personal tenant to UI clients.

Example: checking the current session from a browser or HTTP client:

```bash
curl -b cookies.txt http://localhost:8080/auth/session
# or with a browser, simply navigate to /auth/session
```

Response shape mirrors `/v1/me`:

```json
{
  "success": true,
  "user": { "id": "...", "email": "user@example.com", "isSystemAdmin": false },
  "personalTenant": { "id": "...", "slug": "user-...", "name": "User" }
}
```

---

## 5. Putting It Together

### 5.1 Choosing Between API Keys and Sessions

- Use **API keys** for:
  - CLI tools.
  - CI/CD pipelines.
  - Server-to-server integrations.

- Use **local or OIDC auth + sessions** for:
  - Browser-based UIs.
  - Human users who need to select tenants and inspect their own jobs.

### 5.2 Typical Human Login Flow

1. Operator configures `auth.local` or `auth.oidc` in `config.yaml`.
2. User opens the UI and logs in:
   - Via `POST /auth/login` with email/password, or
   - Via browser redirect flow starting at `GET /auth/oidc/login`.
3. Raito creates the user and a personal tenant (on first login) and issues a session cookie.
4. The UI uses:
   - `GET /auth/session` or `/v1/me` to learn who is logged in.
   - `GET /v1/tenants` and `POST /v1/tenants/:id/select` to drive a tenant switcher.

### 5.3 Where to Look Next

- For tenant concepts (personal vs org, roles, tenant-scoped keys, and usage), read `docs/multi-tenancy.md`.
- For endpoint shapes and example curls, see:
  - `docs/usage.md`
  - `docs/scrape.md`, `docs/crawl.md`, `docs/batch-scrape.md`, `docs/search.md`, `docs/extract.md`
