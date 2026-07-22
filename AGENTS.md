# Farplane

Farplane is a control plane and web client for AI agent computers. The Go
API lives in `farplane-backend/` (Gin, Postgres). The SPA lives in
`farplane-web/` (TanStack Router, Rsbuild). This project is not live yet:
you do not need to keep backwards compatibility, and you can drop and
recreate the local databases at any time.

## Rules

- Write documentation and plans in ASD-STE100 Simplified Technical
  English (STE).
- Timestamps: use `TIMESTAMPTZ(6)` for all timestamp columns (UTC,
  microsecond precision). Do not use second-precision timestamps. In Go,
  store as `time.Time` in UTC and keep microsecond resolution.
- Naming: table and column names must be explicit. Never abbreviate (for
  example `organizations`, not `orgs`; `organization_id`, not `org_id`).
  Prefer full words in indexes and constraints too.

## How to set up

1. Install [mise](https://mise.jdx.dev/), then from the repo root run
   `mise install` (pins Go and Bun from `mise.toml`).
2. Make sure local Postgres is running on `127.0.0.1:5432` (user
   `postgres` / password `postgres`).
3. Create databases and apply migrations:

```bash
make db-create
make migrate-up
make migrate-up-test
```

See `.env.example` for optional overrides (`DATABASE_URL`, `APP_ENV`,
OAuth, and so on).

## How to run

Run the API and the SPA in two terminals:

```bash
make backend   # http://localhost:8080
make web       # http://localhost:3000
```

Open `http://localhost:3000`. The SPA talks to the API at
`http://localhost:8080`.
