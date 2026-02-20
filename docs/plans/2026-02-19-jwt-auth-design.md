# JWT Auth with Refresh Tokens

Date: 2026-02-19

## Summary

Replace HTTP Basic Auth with account-based JWT authentication. Access tokens are short-lived (1 hour); refresh tokens are long-lived (7 days, configurable) and stored in the DB to support explicit revocation.

## Auth Flow

- `POST /api/auth/login` -- accepts `{username, password}`, verifies against `accounts` table (bcrypt), returns `{access_token, refresh_token}`
- `POST /api/auth/refresh` -- accepts `{refresh_token}`, validates against DB, returns new `{access_token, refresh_token}` (rotation), invalidates old refresh token
- `POST /api/auth/logout` -- accepts `{refresh_token}`, deletes it from DB
- All other API endpoints require `Authorization: Bearer <access_token>`
- `/cert` remains unauthenticated

## Tokens

- **Access token**: HS256 JWT, 1 hour TTL, signed with a secret auto-generated at first run and stored in `~/.agent-workspace/config.json`
- **Refresh token**: opaque random string (32 bytes, hex-encoded), 7 days TTL (configurable), stored in the DB

## Database Changes

New `accounts` table:
```sql
CREATE TABLE accounts (
    id         TEXT PRIMARY KEY,
    username   TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at INTEGER NOT NULL
)
```

New `refresh_tokens` table:
```sql
CREATE TABLE refresh_tokens (
    token      TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
)
```

## CLI Commands

- `agent-workspace adduser <username>` -- prompts for password, bcrypts it, inserts into `accounts`
- `agent-workspace passwd <username>` -- change password, invalidates all refresh tokens for that account

## Frontend

- Login page at `/login`; unauthenticated requests redirect there
- Access token stored in memory (not localStorage)
- Refresh token stored in `localStorage`
- On 401, attempt silent refresh; if refresh fails, redirect to login
- SSE connection passes access token as query param (`?token=<access_token>`) since `EventSource` cannot set headers

## Config Changes

`AuthConfig` in `config.json`:
- Remove `Username`, `Password`
- Add `JWTSecret` (string, auto-generated on first run if empty)
- Add `RefreshTokenTTL` (duration string, default `"168h"`)

Auth is active when at least one account exists in the DB. If no accounts exist, the server is unauthenticated (same as today with no auth config).

## Packages

- `internal/webserver/auth.go` -- middleware, token issuing, JWT helpers
- `internal/db/` -- account and refresh token CRUD
- `internal/webserver/static/login.html` -- login page
- CLI adduser/passwd commands in `main.go`
