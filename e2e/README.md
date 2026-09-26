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

## Visual regression (opt-in)

`tests/visual.spec.js` compares screenshots of the key screens, in both themes, with committed
baselines. It is **not** part of `npm test` or CI: baselines are recorded on macOS and font
rendering differs on Linux runners. Run it against a fresh demo server:

```bash
npm run test:visual           # compare
npm run test:visual:update    # refresh baselines after an intentional UI change (review the diffs first)
```

See `docs/DESIGN-SYSTEM.md` → *Visual regression*.

## Functional specs (state-changing flows)

`tests/flows-*.spec.js` submit the create / edit / delete / action flows through the real UI and
verify the result three ways: the mutating request came back 2xx, the UI shows the change, and an
independent API read (Basic auth, not the page's session) agrees. Any mutating call that returns
>= 400 or any console error fails the test unless the test declared it expected. Every test uses
uniquely named resources and cleans up.

| Spec | Covers |
| --- | --- |
| `flows-core` | namespaces |
| `flows-config` | ConfigMaps, Secrets (edit values), HPA, NetworkPolicy / Apply dialog, Atlas volume, read-only pages |
| `flows-workloads` | Run wizard (form / intent / validate), scale, rollout, delete, pods, jobs / cronjobs, nodes |
| `flows-ops` | VMs, backups / Velero, Atlas, Helm, GitOps, agent runs, RBAC |
| `flows-admin` | Users (incl. real sign-in of created users), Settings integrations + Test buttons, clusters |
| `flows-integrations` | PacketWolf (mock server), plugin page, OIDC / SAML / LDAP against in-process mock IdPs |

```sh
make ui && go build -o bin/zyvor-axiom ./cmd/zyvor-axiom
AXIOM_MODE=demo AXIOM_USERNAME=demo AXIOM_PASSWORD=demo AXIOM_ADDR=:5091 ./bin/zyvor-axiom &
cd e2e && AXIOM_E2E_BASE_URL=http://localhost:5091 npx playwright test
BROWSERS=all ...   # also Firefox and WebKit (npx playwright install firefox webkit)
```

`flows-integrations` ignores `AXIOM_E2E_BASE_URL`: it starts its own servers (from a copy of
`bin/zyvor-axiom`) with the environment each flow needs. Known product bugs are declared with
`fixme(...)` and run only with `AXIOM_E2E_RUN_FIXME=1`.

**What a pass proves.** The demo backend is in-memory, so these specs prove the UI-to-API contract
and UI behaviour, not the Kubernetes implementation. That is covered by the fake-clientset tests in
`internal/platform/*_kubernetes_test.go` (`go test -modfile=go.production.mod -tags kubernetes ./internal/...`).
Real IdPs (Keycloak, Okta, AD), real cluster behaviour and real Slack / Atlas are not exercised.

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
