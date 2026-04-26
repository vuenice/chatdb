# ChatDB

Single-binary database viewer with a Vue 3 SPA frontend and a small plain-Go API. Saved connections target **PostgreSQL** or **MySQL/MariaDB**, and chatdb stores its own users + connection registry in a local **SQLite** file alongside the binary.

The legacy Goravel implementation is preserved under [`old_goravel_backend/`](old_goravel_backend/) for reference.

## Layout

| Path | Role |
|------|------|
| [`backend/cmd/chatdb`](backend/cmd/chatdb) | Entry point. Loads config, bootstraps metadata, serves API + embedded SPA. |
| [`backend/internal/config`](backend/internal/config) | JSON config loader (`-config` flag, defaults, validation). |
| [`backend/internal/migrate`](backend/internal/migrate) | Idempotent SQLite DDL for `users` + `db_connections`. |
| [`backend/internal/store`](backend/internal/store) | Typed queries against the SQLite metadata DB. |
| [`backend/internal/engine`](backend/internal/engine) | `Engine` interface with Postgres + MySQL implementations. |
| [`backend/internal/api`](backend/internal/api) | HTTP handlers (chi router). |
| [`backend/internal/auth`](backend/internal/auth) | bcrypt + JWT helpers + middleware. |
| [`backend/web/dist`](backend/web/dist) | Built SPA, populated by `make frontend`, embedded at compile time. |
| [`frontend/`](frontend/) | Vue 3 + Vite source. |

## Configuration

Copy and edit [`backend/chatdb.config.example.json`](backend/chatdb.config.example.json) to e.g. `backend/chatdb.config.json` (the binary defaults to that path; override with `-config /path/to/file.json`):

```json
{
  "listen": "127.0.0.1:3000",
  "jwt_secret": "long-random-string",
  "app_key": "32-byte-key-change-me-aaaaaaaaaa",
  "metadata": {
    "path": "chatdb.meta.sqlite"
  }
}
```

- `app_key` must be **exactly 32 bytes** (used as the AES-256-GCM key for stored connection passwords).
- `metadata.path` is the SQLite file that holds chatdb's own `users` and `db_connections` tables. It is created on first startup (with `journal_mode=WAL` and foreign keys enabled). Relative paths resolve against the binary's working directory; back this file up the same way you back up `chatdb.config.json`.
- The saved **target** databases (what users browse) still live in their own Postgres or MySQL/MariaDB servers and are added at runtime via the UI or `POST /api/connections` — chatdb supports any number of them.

### Migrating from the old Postgres/MySQL metadata layout

Earlier builds stored chatdb's own tables inside an `app_database` you configured (Postgres schema or MySQL database). That `app_database` block is **no longer accepted** by the config loader. To migrate an existing install:

1. Start the new build once with a fresh `metadata.path` so the SQLite schema is created.
2. Copy `users` and `db_connections` rows from the old metadata location into the new SQLite file (`sqlite3 chatdb.meta.sqlite`). Keep `app_key` unchanged so the encrypted `read_password` / `write_password` columns stay decryptable; otherwise re-add connections through the UI.
3. Remove the old `app_database` block from `chatdb.config.json`.

## Build a single binary

Requires Go 1.22+ and Node 18+.

```bash
make build
```

Produces `./chatdb` (CGO disabled, suitable for any modern Linux x86_64). The Vue app is embedded via `go:embed`, so the binary serves both the SPA and the JSON API on `listen`. Ship the binary plus your `chatdb.config.json` and the SQLite metadata file (file permissions `0600` recommended for both).

## Development

In two terminals:

```bash
make dev-backend    # go run ./cmd/chatdb on :3000
make dev-frontend   # vite on :5173, proxies /api to :3000
```

on windows
```sh
cd backend
go run .\cmd\chatdb -config .\chatdb.config.json
```
```sh
cd frontend
npm run dev
```

## Sample databases (optional)

```bash
docker compose up -d
```

Brings up a sample customer Postgres on `:5434` you can register as a target via the UI (credentials in [`docker-compose.yml`](docker-compose.yml) and [`scripts/init-sample-db.sql`](scripts/init-sample-db.sql)). chatdb's own metadata stays in the local SQLite file regardless.

## API surface (v1 viewer)

- `POST /api/register`, `POST /api/login`, `GET /api/me`
- `GET|POST|PUT|DELETE /api/connections` (+ `/{id}`)
- `GET /api/connections/{id}/databases|tables|columns|rows`
- `POST /api/connections/{id}/sql/execute|cancel`
- `GET /api/health`

The endpoints from the legacy app for **AI chat**, **saved/recent queries**, **monitoring**, **schema graph**, and **EXPLAIN** are stubbed (return empty/disabled payloads) so the existing UI keeps working without errors. Reintroduce them as needed.

## Notes

- `connections.driver` may be `"postgres"` or `"mysql"`. Defaults to `postgres` for compatibility with the existing UI.
- Identifier quoting differs per dialect; the row-preview endpoint validates schema/table names with a strict regex before quoting.
- Stored connection passwords are AES-256-GCM encrypted with `app_key`; only the encrypted form ever leaves the binary.
