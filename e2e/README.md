# Axiom E2E smoke tests

Playwright tests that replace manual Chrome-based verification with
something that runs unattended. Deliberately narrow: login, core
navigation (sidebar + command palette), and the health/metrics
endpoints — not a full regression suite for every page.

## Run locally

1. Start a demo-mode Axiom server (a fresh one — see the rate-limiter
   note below):

   ```bash
   cd .. && make demo &   # or: AXIOM_MODE=demo AXIOM_ADDR=:5090 ./bin/zyvor-axiom
   ```

2. Install and run:

   ```bash
   npm install
   npx playwright install --with-deps chromium
   npm test
   ```

Override the target and credentials with `AXIOM_E2E_BASE_URL`,
`AXIOM_E2E_USERNAME`, `AXIOM_E2E_PASSWORD` if not testing against a
default local demo instance.

## Why `workers: 1`

Axiom's login rate limiter (`internal/auth/ratelimit.go`) counts every
unauthenticated request — including the app's own pre-login session
probe — as a failed attempt per client IP, after a small grace count.
Several browser contexts hitting the same server concurrently can trip
it, so `playwright.config.js` runs tests serially. If you do hit a
`429 too many failed login attempts`, restart the server (the limiter's
state is in-memory and resets with it) rather than waiting out the
backoff.

## CI

See the `e2e` job in `.github/workflows/ci.yml` — it builds the demo
binary, starts it, waits for `/api/v1/health`, then runs this suite
against it.
