# Zyvor Axiom Validation Results

## 2026-08-28 (later) — customer-site features: ConfigMaps/Quotas/Ingresses/Jobs, notifications, RBAC, multi-cluster

Six features added in response to "what else needs to be done for a real customer deployment":

```text
go test ./cmd/... ./internal/...                                        PASS
go vet ./cmd/... ./internal/...                                         PASS
go build ./cmd/zyvor-axiom                                              PASS (demo build)
go build -modfile=go.production.mod -tags kubernetes ./cmd/zyvor-axiom  PASS (kubernetes build)
npx craco build (frontend/)                                             PASS
```

**ConfigMaps / ResourceQuotas / LimitRanges / Ingresses** — full CRUD for ConfigMaps, read-only views for the other three. Verified via curl against demo seed data, then via Chrome: created and deleted a real ConfigMap through the actual UI dialog, confirmed the create/delete toasts and table updates.

**Jobs & CronJobs** — separate page from Workloads. Verified via Chrome: clicked "run now" on a demo CronJob (a new Job appeared, `lastSchedule` updated to "now"), toggled the suspend `Switch` off then on (status badge flipped Running↔Paused), deleted the manually-triggered Job — all through the real UI, all confirmed via toast + table state.

**Slack/webhook alert notifications** — Settings → Notifications card (Slack + generic webhook URLs, forward-warnings toggle, two independent Test buttons) plus a 60s background dispatcher. Verified: PUT'd a webhook URL, clicked "Test webhook" in the real UI, and a local Python HTTP listener received the actual POST body (`{"message":"Axiom test notification...","source":"zyvor-axiom"}`). Added 6 unit tests (`internal/platform/notify_test.go`) covering payload delivery and the dispatcher's enabled/disabled gating via a fake provider + `httptest` servers, since the fixed 60s interval isn't practical to exercise against real demo data timing.

**Local Users / RBAC** — bcrypt-hashed local accounts (admin/viewer), enforced centrally in the auth middleware (any `viewer` mutating request → 403, before it reaches a handler). Verified via Chrome: created a viewer user through the real Settings → Users dialog, logged in as that user in a separate tab ("Welcome back, chrome-test-user"), confirmed full read access to Overview, then confirmed a real `403 {"error":"read-only user..."}` calling `POST /api/v1/namespaces` with that session's own credentials from inside the browser (not a separate curl). Added unit tests for `Guard`'s multi-user/read-only enforcement and `UserHub` CRUD.

**Real multi-cluster switching** — `Provider` is always a `SwitchableProvider` now (generated delegator for all 88 interface methods, `switchable_provider_gen.go`), so no handler code changed. Verified via Chrome: opened the cluster-switcher dropdown (3 real demo clusters), clicked a different one, watched the Overview hero/breadcrumb/sidebar genuinely update to the new cluster's data after reload — then switched back and confirmed the state reverted cleanly.

### Live remote deployment (real k3s cluster, 80.79.5.173)

Redeployed with `./scripts/deploy-remote.sh 80.79.5.173 sus`. First deploy attempt surfaced a real RBAC gap: the ClusterRole never granted `resourcequotas`, `limitranges`, `ingresses` (networking.k8s.io), or `cronjobs` (batch), so those four endpoints returned `403 Forbidden` in production despite working fine in demo mode (the demo provider has no RBAC to enforce). Fixed `deploy/k8s/rbac.yaml`, redeployed, and reverified — all four now return real cluster data:

- Ingresses: `guestkit-enterprise` (traefik), `packetwolf` (nginx, TLS) — pre-existing unrelated workloads on the shared lab host
- ResourceQuotas: real KubeVirt VM instance quotas in `default` and `zeus-os-e2e`
- CronJobs: `aether-cilium-connectivity`, `zyvor-qa-smoke`, `redis-backup` — real scheduled jobs already running on this cluster
- `GET /api/v1/clusters` correctly reports exactly **one** cluster (`in-cluster`) on this deployment — no fabricated multi-cluster entries in production, since this host only has the one ServiceAccount-based connection
- Created a real viewer account against this cluster, confirmed the bcrypt hash (not plaintext) landed in the `zyvor-axiom-users` Secret, confirmed the 403-on-write enforcement, then deleted it and confirmed the Secret's JSON blob went back to `{}`

Could not get a live Chrome screenshot of the remote host specifically for this round — the browser automation extension cannot interact with the self-signed-certificate warning interstitial (`chrome-error://` page), a hard Chrome platform restriction hit repeatedly in earlier sessions too. All remote verification for this round is via direct `curl`/`ssh kubectl` against the real cluster's real API responses and real Secret contents, which covers the same functional ground as a screenshot would.

## 2026-08-28 — React frontend rewrite + live remote validation

The embedded UI was replaced entirely: the hand-written `web/app.js` vanilla-JS SPA is gone, replaced by `frontend/` (React 19 + CRACO + Tailwind + shadcn/ui), ported directly from the kube-cockpit reference and wired to Axiom's real API.

```text
go test ./cmd/... ./internal/...                                    PASS
go vet ./cmd/... ./internal/...                                     PASS
go build ./cmd/zyvor-axiom                                          PASS (demo build)
go build -modfile=go.production.mod -tags kubernetes ./cmd/zyvor-axiom  PASS (kubernetes build)
npx craco build (frontend/)                                         PASS
```

Verified in a real Chrome browser (not just curl) against `make demo` locally, covering every page reachable from the sidebar, plus:

- Login (local Basic-auth form + SSO buttons), workload launch wizard (Form/Intent/Validate-YAML), node cordon/drain, VM start/stop/migrate, Helm rollback/uninstall, agent-run approve/deny, backup create/restore, secret create, namespace create/delete, pod exec (one-shot, demo mode), Atlas page, PacketWolf panel (not-configured state).

### Live remote deployment (real k3s cluster + real Atlas)

Deployed to a real remote host with `./scripts/deploy-remote.sh <host> <user> --with-atlas` (Axiom + a separate Atlas storage-control-plane service, auto-wired together). Found and fixed a real bug in the process: an unrelated pre-existing Service was already holding the default NodePort, which `kubectl apply` silently rejected — the script now checks the apply's exit status and the live NodePort, failing loudly with a clear "pick a different `AXIOM_NODE_PORT`" message instead of limping on.

Verified end-to-end **in an actual Chrome window** against the live cluster (not a demo):

- Real interactive Pod terminal: typed `echo HELLO_FROM_REAL_CHROME_TERMINAL` on the keyboard into the xterm.js UI → WebSocket → real bash PTY in a real KubeVirt pod → output rendered back correctly
- Atlas page showing real Ceph capacity (204TB raw / 90TB used) from actual backends/pools/volumes
- Workloads page listing all 65 real Deployments on the cluster
- Pod detail page showing real owner references, container images, resource requests from an actual pod's live manifest
- Getting Started onboarding checklist correctly detecting real unconfigured-SSO state

Also verified directly over a raw WebSocket client (bypassing the browser, to isolate protocol correctness): the same terminal endpoint with `?access_token=` query-param auth, confirming the `__resize__:WxH` control message and PTY exec both work independent of any frontend code.

## 2026-08-27 — prior validation (superseded)

```text
go test ./...          PASS
go vet ./...           PASS
go build               PASS
node --check web/app.js PASS   (web/ no longer exists — see above)
source identity scan   PASS
HTTP smoke test        PASS
```

At the time of this entry, the kubernetes-tagged build could not be validated in the sandbox (no network access to `proxy.golang.org` to download `client-go`/`gorilla/websocket`). This is resolved as of 2026-08-28 above — the kubernetes build has since been compiled and deployed successfully.
