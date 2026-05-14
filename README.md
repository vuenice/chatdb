# ChatDB

ChatDB is a **single-binary** database viewer + lightweight API.

- **Backend**: plain Go (chi router), ships as one executable
- **Frontend**: Vue 3 SPA, embedded into the Go binary via `go:embed`
- **Targets**: PostgreSQL and MySQL/MariaDB (browse + run SQL)
- **Metadata**: local SQLite file (`chatdb.meta.sqlite`) for users + connection registry

## Features

- **Auth**: register/login with JWT (`/api/register`, `/api/login`, `/api/me`)
- **Connection registry**: save a DB connection (credentials **encrypted at rest**)
- **Browse catalog**:
  - databases
  - tables + columns + indexes
  - preview rows (paged)
- **Run SQL**: execute queries on read/write pools + **cancel** in-flight runs
- **Edit data**: update a row (best-effort; see “Notes” below)
- **DB operations (basic)**:
  - truncate database tables
  - delete database
  - rename database
  - import / export via **Workbench → Operations** links to dedicated pages:
    - **PostgreSQL**: `pg_dump` (plain `.sql` or custom-format `.dump`), import with `psql` or `pg_restore` (needs `postgresql-client` on the ChatDB host `PATH`)
    - **MySQL**: upload import still runs through the app executor; export remains a minimal table listing
- **Bulk table operations**: drop / truncate / analyze / optimize / repair / check across multiple tables

## Quickstart (single binary)

Prereqs: a recent Go + Node.js (for building the SPA).

On first start, ChatDB creates **`chatdb.config.json`** and **`chatdb.meta.sqlite`** under the OS user config directory (same folder):

| OS | Typical path |
|----|----------------|
| Windows | `%APPDATA%\chatdb\` |
| Linux | `$XDG_CONFIG_HOME/chatdb/` (default `~/.config/chatdb/`) |
| macOS | `~/Library/Application Support/chatdb/` |

The first-run config uses **`listen`: `127.0.0.1:6366`**, random **`jwt_secret`** and **`app_key`**, and an absolute **`metadata.path`** next to the JSON file. You do not need to copy or author a config file by hand. Edit the JSON there if you want to change the listen address or rotate secrets.

Build and run:

```bash
make build
./chatdb
```

Then open `http://127.0.0.1:6366`.

[`backend/chatdb.config.example.json`](backend/chatdb.config.example.json) shows the JSON field names only; the binary does not read that file at runtime.

## Development

Backend (API + embedded SPA when built):

```bash
make dev-backend
```

Frontend dev server (Vite on `:5173`, proxies `/api` to the backend):

```bash
make dev-frontend
```

### Windows (without Make, or prefer explicit paths)

From the repo root, use two terminals.

Backend:

```powershell
cd backend
go run .\cmd\chatdb
```

Frontend:

```powershell
cd frontend
npm install
npm run dev
```

### Sample database (optional)

```bash
docker compose up -d
```

Brings up sample Postgres on `:5434`; credentials and wiring are in [`docker-compose.yml`](docker-compose.yml) and [`scripts/init-sample-db.sql`](scripts/init-sample-db.sql). ChatDB’s own metadata lives in the SQLite file next to your auto-created `chatdb.config.json` (see Quickstart paths).

### Migrating from old `app_database` metadata

Earlier builds stored chatdb users and connection rows inside a Postgres/MySQL **app database**. The current config loader no longer accepts that shape (`DisallowUnknownFields`). To migrate:

1. Start the new build once so the metadata SQLite file is created under your user config directory (see Quickstart).
2. Copy `users` and `db_connections` from the old database into the new SQLite file (`sqlite3 chatdb.meta.sqlite`). Keep `app_key` unchanged so encrypted passwords stay decryptable, or re-add connections in the UI.
3. If your `chatdb.config.json` still contains a legacy `app_database` key, remove it (`DisallowUnknownFields` rejects unknown keys).

## API (current surface)

All endpoints are rooted at `/api`. Everything except health/register/login/connection-labels requires an `Authorization: Bearer <token>` header.

### Auth

- `POST /api/register`
- `POST /api/login`
- `GET /api/me`
- `GET /api/connection-labels`
- `GET /api/health`

**Register** creates:
- a ChatDB user (stored in the local metadata SQLite)
- a first DB connection record under the provided `connection_name`
- a JWT token to use for subsequent requests

Example:

```bash
curl -sS -X POST "http://127.0.0.1:6366/api/register" \
  -H 'content-type: application/json' \
  -d '{
    "connection_name":"local-dev",
    "driver":"postgres",
    "host":"127.0.0.1",
    "port":5432,
    "database":"postgres",
    "ssl_mode":"disable",
    "read_username":"postgres",
    "read_password":"postgres",
    "write_username":"postgres",
    "write_password":"postgres",
    "allowed_schemas":["public"]
  }'
```

### Connections

- `GET /api/connections`
- `POST /api/connections`
- `GET /api/connections/{id}`
- `PUT /api/connections/{id}`
- `DELETE /api/connections/{id}`

Notes:
- This build currently enforces **one connection per user** (creating a second returns HTTP 409).
- Passwords are never returned by the API; they are stored **encrypted** in the metadata SQLite.

### Catalog / browsing

- `GET /api/connections/{id}/databases`
- `GET /api/connections/{id}/tables?schema=...`
- `GET /api/connections/{id}/columns?schema=...&table=...`
- `GET /api/connections/{id}/indexes?schema=...&table=...`
- `GET /api/connections/{id}/rows?schema=...&table=...&limit=...&offset=...`
- `POST /api/connections/{id}/rows/update`

Catalog admin (driver-dependent):

- `GET /api/connections/{id}/catalog/roles`
- `GET /api/connections/{id}/catalog/login_users`
- `POST /api/connections/{id}/catalog/users`

### SQL execution

- `POST /api/connections/{id}/sql/execute`
- `POST /api/connections/{id}/sql/cancel`

`/sql/execute` payload:

```json
{
  "sql": "select 1",
  "pool": "read",
  "max_rows": 200,
  "database": "optional-db-name",
  "role": "optional-role-name"
}
```

### DB operations

- `POST /api/connections/{id}/truncate`
- `POST /api/connections/{id}/delete`
- `POST /api/connections/{id}/rename` (`{"new_name":"..."}`)
- `POST /api/connections/{id}/import` — multipart: `file`; for PostgreSQL include `format`: `psql` (plain `.sql`, runs `psql -f`) or `pgdump` (custom archive, runs `pg_restore`). Optional query `database=` for physical DB. MySQL: upload only (no `format`).
- `GET /api/connections/{id}/export` — for PostgreSQL, query `format=plain` (default; text SQL) or `format=archive` (`pg_dump -Fc`). Optional `database=` for physical DB override. MySQL returns a small placeholder listing.

### Bulk table operations

- `POST /api/connections/{id}/bulk?database=...`

Body:

```json
{
  "operation": "truncate",
  "tables": ["public.users", "public.orders"]
}
```

Supported `operation`: `drop`, `truncate`, `analyze`, `optimize`, `repair`, `check`

## What’s intentionally disabled (stubs)

The legacy UI expects these endpoints; this backend returns safe placeholder payloads so the frontend doesn’t crash:

- AI chat: `POST /api/connections/{id}/ai/chat` (disabled)
- Schema graph: `GET /api/connections/{id}/schema_graph` (empty)
- Monitoring: duplicate indexes / slow queries (unavailable)
- EXPLAIN: `POST /api/connections/{id}/sql/explain` (disabled)
- Saved/recent queries endpoints (return empty payloads in current router)

## Security notes

- `app_key` must be **exactly 32 bytes** (AES-256 key for encrypting stored DB passwords).
- Keep `chatdb.config.json` and `chatdb.meta.sqlite` in your app data directory **private** (first-run file mode is `0600` where supported).
- JWT signing uses `jwt_secret`; rotate it to invalidate existing sessions.

## Repo layout

- `backend/cmd/chatdb`: entrypoint (config + migrations + API + SPA)
- `backend/internal/api`: HTTP handlers + router
- `backend/internal/engine`: Postgres/MySQL engines (catalog + SQL)
- `backend/internal/store`: SQLite metadata store
- `backend/internal/migrate`: metadata schema bootstrap/upgrade
- `backend/web/dist`: built SPA (embedded in the binary)
- `frontend/`: Vue 3 app

## Notes / known limitations

- **One connection per user** is enforced in `POST /api/connections`.
- PostgreSQL **full dumps** require `pg_dump`, `psql`, and **`pg_restore`** (for archives) installed on the machine running the ChatDB binary (e.g. `postgresql-client`). Large imports/uploads may hit reverse-proxy or client timeouts if you terminate TLS in front of the app.
- Database “truncate” / bulk operations are dialect-sensitive; some SQL in these handlers may need refinement for strict Postgres/MySQL compatibility.

## License

Add your preferred license (MIT/Apache-2.0/etc.) and a `LICENSE` file at the repo root.
