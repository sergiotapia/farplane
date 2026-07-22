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
OAuth, GitHub App, and so on).

### GitHub App (repo connect)

Farplane is self-hosted: each install creates **its own** GitHub App.
GitHub App names are global, so the manifest names the App
`Farplane AI ({Farplane organization name})`.

GitHub **requires publicly reachable HTTPS URLs** for the App webhook
and callback. `localhost`, `127.0.0.1`, and plain `http://` will fail
Farplane’s manifest checks (and GitHub rejects localhost hooks).

1. Copy `.env.example` to `.env` (the API loads `.env` from the repo
   root or `farplane-backend/`).
2. Set public URLs before creating the App:
   - `APP_API_BASE_URL` — public base URL of this API (webhooks and
     callbacks). Required for the manifest.
   - `APP_BASE_URL` — public base URL of the SPA (App homepage /
     redirects after OAuth-style flows).
3. For local development, use a tunnel such as
   [ngrok](https://ngrok.com/) on the API port, then set for example:

```bash
# Terminal A: expose the API
ngrok http 8080

# .env (use the https URL ngrok prints)
APP_API_BASE_URL=https://YOUR_SUBDOMAIN.ngrok-free.app
APP_BASE_URL=http://localhost:3000
```

   Restart `make backend` after changing `.env`. If the ngrok URL
   changes, update `APP_API_BASE_URL` and create a new App (or edit
   the existing App’s webhook URL in GitHub).
4. Sign in as owner/admin → **GitHub** → **Create Farplane AI GitHub
   App**. That uses GitHub’s App Manifest flow. Credentials are stored
   encrypted in Postgres (keyed from `SESSION_SECRET`).
5. Then click **Connect GitHub** to install the App on repos or an org.

Optional: set `GITHUB_APP_*` in `.env` to override DB credentials.
Members without GitHub never need an account; only people who can
install the App click Connect.

## How to run

Run the API and the SPA in two terminals:

```bash
make backend   # http://localhost:8080
make web       # http://localhost:3000
```

Open `http://localhost:3000`. The SPA talks to the API at
`http://localhost:8080`.
