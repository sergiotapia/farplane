This is a fresh project, not live, no need to preserve backwards compatibility.

We can always drop the dev db, recreate it from scratch.

Use ASD-STE100 Simplified Technical English (STE for short) for documentation
and plans.

## Layout

| Path | Role |
|---|---|
| `farplane-backend/` | Go control plane (Gin) |
| `farplane-web/` | TanStack Router SPA (Rsbuild) |
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

### 1. Control plane (`farplane-backend`)

Default listen address: `:8080` (`PORT` or `ADDR` can override).

```bash
cd farplane-backend
go run ./cmd/farplane
```

Or from the repo root:

```bash
mise run backend
```

Smoke checks:

- `GET http://localhost:8080/health` → `{"status":"ok"}`
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

1. Start the Go API on port **8080**.
2. Start the SPA on port **3000**.
3. Open `http://localhost:3000` in the browser.
4. The SPA will call the Go API at `http://localhost:8080` (wire this in later stages).
