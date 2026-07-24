# Farplane web quality gates

Local tooling for the SPA. Root `make check` / `make gauntlet` compose
these into the agent Definition of Done. Thresholds are strict; prefer
fixing code over disabling rules.

## Fast gates (no servers)

Suggested composition order for root `make check`:

```text
format-web → lint-web → typecheck-web → test-web
  → knip-web → deps-web → audit-web → mutate-web → test-web-e2e
```

```bash
make lint-web          # biome check --error-on-warnings (preset: all)
make format-web        # biome format --write
make typecheck-web     # tsc --noEmit
make test-web          # vitest; coverage floors 100/100/100/100 on src/lib
make knip-web          # dead code / unused exports / unused deps
make deps-web          # dependency-cruiser (cycles, orphans, layers)
make audit-web         # bun audit --audit-level=moderate
make mutate-web        # StrykerJS; break ≥ 95 on src/lib helpers
```

## E2E (Playwright + playwright-bdd)

```bash
make test-web-e2e
```

Strict settings: `forbidOnly`, `retries: 0`, short action/expect timeouts.

Prerequisites:

1. API running: `make backend` (http://localhost:8080)
2. Chromium installed once: `cd farplane-web && bunx playwright install chromium`
3. SPA starts automatically via Playwright `webServer` (or reuse
   `make web` on port 3000)
4. Authenticated journeys need a real user:

```bash
export E2E_EMAIL='you@example.com'
export E2E_PASSWORD='your-password'
make test-web-e2e
```

Without `E2E_EMAIL` / `E2E_PASSWORD`, authenticated Gherkin scenarios skip.
Unauthenticated sign-in page scenarios still run when the API is up (wrong
password) or without the API (page smoke + client validation).

Org switch is not in the UI yet. `organization.feature` asserts the active
organization name in the sidebar after sign-in.

## Package scripts

| Script | Purpose |
|---|---|
| `lint` / `lint:fix` | Biome check / autofix (`--error-on-warnings`) |
| `format` | Biome format write |
| `typecheck` | `tsc --noEmit` |
| `test` / `test:watch` | Vitest |
| `test:coverage` | Vitest with coverage thresholds |
| `test:e2e` | bddgen + Playwright |
| `knip` | Knip (unused files, exports, dependencies) |
| `deps` | dependency-cruiser |
| `audit` | `bun audit` (moderate+) |
| `mutate` | StrykerJS (break ≥ 95) |
