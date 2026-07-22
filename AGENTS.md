This is a fresh project, not live, no need to preserve backwards compatibility.

We can always drop the dev db, recreate it from scratch.

Use ASD-STE100 Simplified Technical English (STE for short) for documentation
and plans.

## Layout

| Path | Role |
|---|---|
| `farplane-backend/` | Go control plane (Gin) |
| `farplane-web/` | TanStack Router SPA (Rsbuild) |
| `Makefile` | Common tasks (DB, migrate, test, run) |
| `plan.md` | Product and toolchain plan |
| `mise.toml` | Pinned Go and Bun tools |

## Tools

Install [mise](https://mise.jdx.dev/), then from the repo root:

```bash
mise install
```

This installs the Go and Bun versions in `mise.toml`.

## How to run both (local development)

Run the API and the SPA in two terminals.

### 0. Postgres (local)

Use the local Postgres on `127.0.0.1:5432` with user `postgres` / password `postgres`.

| Database | Use |
|---|---|
| `farplane_dev` | Local control plane |
| `farplane_test` | Automated tests |

Create the databases (if needed) and apply migrations:

```bash
make db-create
make migrate-up
make migrate-up-test
```

Default connection strings:

- Dev: `postgres://postgres:postgres@127.0.0.1:5432/farplane_dev?sslmode=disable`
- Test: `postgres://postgres:postgres@127.0.0.1:5432/farplane_test?sslmode=disable`

Override with `DATABASE_URL` / `TEST_DATABASE_URL`. Set `APP_ENV=local` (default) to allow the local DSN; other `APP_ENV` values require `DATABASE_URL`. See `.env.example`.

Useful Make targets:

- `make db-create` / `make db-drop` — create or drop the Farplane databases
- `make db-psql` / `make db-psql-test` — open `psql` on dev or test
- `make migrate-up` / `make migrate-down` / `make migrate-status` — schema migrations (dev)
- `make migrate-up-test` / `make migrate-reset-test` — schema migrations (test)
- `make migrate-create NAME=add_users` — add a new SQL migration file
- `make test` / `make test-backend` — run tests
- `make backend` / `make web` — run the API or the SPA

Stack notes:

- Driver: [pgx/v5](https://github.com/jackc/pgx) with `pgxpool`
- Migrations: [pressly/goose](https://github.com/pressly/goose) (SQL files under `farplane-backend/internal/db/migrations`)

### 1. Control plane (`farplane-backend`)

Default listen address: `:8080` (`PORT` or `ADDR` can override).
Requires a reachable Postgres (`DATABASE_URL`).

```bash
cd farplane-backend
go run ./cmd/farplane
```

Or from the repo root:

```bash
make backend
# or: mise run backend
```

Smoke checks:

- `GET http://localhost:8080/health` → `{"status":"ok"}` (liveness)
- `GET http://localhost:8080/ready` → `{"status":"ok","database":"up"}` (readiness)
- `GET http://localhost:8080/api/v1/hello` → `{"message":"farplane"}`

CORS allows the SPA origin `http://localhost:3000`.

### 2. Client UI (`farplane-web`)

Default dev server: `http://localhost:3000`.

```bash
cd farplane-web
bun install
bun run dev
```

Or from the repo root:

```bash
mise run install-web
mise run web
```

Useful scripts:

- `bun run dev` — Rsbuild dev server
- `bun run build` — production static build into `farplane-web/dist`
- `bun run preview` — preview the production build

### Expected local setup

1. Ensure Postgres databases exist (`make db-create`) and apply migrations (`make migrate-up`).
2. Start the Go API on port **8080** (`make backend`).
3. Start the SPA on port **3000** (`make web`).
4. Open `http://localhost:3000` in the browser.
5. The SPA will call the Go API at `http://localhost:8080` (wire this in later stages).
