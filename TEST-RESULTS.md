# Zyvor Axiom Validation Results

## 2026-08-29 (latest) — Nice-to-have tier: PriorityClasses, per-workload cost, audit filtering/export, multi-tab terminal

Closes the last tier from the "what's missing" audit (a fifth item, a plugin/extension architecture, stays deliberately deferred — a large architectural investment with no concrete design basis, left for its own dedicated conversation rather than a batch-plan bullet):

```text
go test ./cmd/... ./internal/...                                        PASS
go vet ./cmd/... ./internal/...                                         PASS
go build ./cmd/zyvor-axiom                                              PASS (demo build)
go build -modfile=go.production.mod -tags kubernetes ./cmd/zyvor-axiom  PASS (kubernetes build)
npx craco build (frontend/)                                             PASS
```

**PriorityClasses** (read-only) — new fourth section on the Quotas page. Verified via Chrome (demo: all four seeded classes including the "default" badge) and curl (real cluster — real `system-node-critical`/`system-cluster-critical`/`kubevirt-cluster-critical` classes).

**Per-workload cost breakdown** — a "By workload" table on Ops → Cost, derived from the same per-pod CPU/memory-request loop `EstimateCost` already ran for the namespace breakdown, with ReplicaSet-owned pods resolved to their Deployment name. Verified via Chrome (demo) and curl against the real cluster: 72 real workloads, correctly showing clean names like `cdi-apiserver`/`cluster-network-addons-operator` rather than raw ReplicaSet hash suffixes.

**Audit log search + CSV export** — frontend-only (the whole audit dataset is always ≤200 entries in one response already). Verified via Chrome: typed a non-matching search term and got "No audit entries match", cleared it, clicked Export and confirmed a real `Blob`-backed download was triggered with the correct `axiom-audit-YYYY-MM-DD.csv` filename (intercepted `HTMLAnchorElement.prototype.click` to inspect the synthetic download without actually saving a file).

**Multi-tab terminal** — `PodDetail.jsx`'s Terminal tab now manages an array of sessions rendered as nested tabs, each mounting its own `PodTerminal` instance. Verified via Chrome that demo mode's one-shot-exec fallback still renders correctly (no regression — multi-tab only applies to the real interactive-terminal branch). Chrome could not reach the real cluster's terminal directly (the self-signed-cert interstitial blocks browser automation on that host, a known limitation noted in the 2026-08-28 entries below), so the feature's core technical assumption — that concurrent WebSocket terminal connections to the same pod are fully independent — was verified instead with two simultaneous raw Python `websocket-client` connections to a real KubeVirt `virt-launcher` pod on the live cluster, each sending a different `echo` command and each receiving only its own output with zero cross-talk.

### Live remote deployment (real k3s cluster, <host>)

One deploy for the batch (`./scripts/deploy-remote.sh <host> sus`, picking up the new `scheduling.k8s.io/priorityclasses` RBAC grant). PriorityClasses and per-workload cost re-verified against real cluster data via curl; the terminal's concurrency assumption verified via raw WebSocket as described above, then the scratch verification scripts and their throwaway Python virtualenv were deleted.

## 2026-08-29 — Second Kubernetes-console gap tier: command palette, admission webhooks, storage objects, node metrics/taints, Gateway API, Velero schedules, pod diagnosis, RBAC write, GitOps sync, Helm install

A second "what else is missing" audit (fresh, grep-verified against the repo as it stood after the Tier-1 pass) found 11 more concrete gaps versus OpenShift Console/Lens/Rancher/k9s. Scoped to "must-have + high-value" (explicitly excluding a nice-to-have tier: PriorityClasses, audit-log filtering, per-workload cost breakdown, multi-tab terminal, plugin architecture). Implemented and verified one item at a time, easiest/lowest-risk first, committing and deploying after each:

```text
go test ./cmd/... ./internal/...                                        PASS
go vet ./cmd/... ./internal/...                                         PASS
go build ./cmd/zyvor-axiom                                              PASS (demo build)
go build -modfile=go.production.mod -tags kubernetes ./cmd/zyvor-axiom  PASS (kubernetes build)
npx craco build (frontend/)                                             PASS
```

**Command palette real search** — expanded from a hardcoded 8-of-26 route list to the full sidebar (via a new shared `frontend/src/constants/navItems.js` so the two can't drift again) plus live pod/workload/namespace search using `cmdk`'s built-in fuzzy filter. Verified in Chrome: typed "checkout", got real live pods `checkout-01..06` in the `commerce` namespace, clicked through to the pod detail page.

**Admission webhook visibility, StorageClasses/PersistentVolumes** — two new read-only Ops/Storage sections. Verified via Chrome (demo) and curl (real cluster — real KubeVirt CDI admission webhooks, real Ceph/local-path StorageClasses, real PVs).

**Per-node metrics + taint management** — new `/nodes/:name` detail page (didn't exist before; `GET /api/v1/nodes/{name}` was already there but nothing consumed it). Verified the full taint add/verify/remove write path against the real cluster (<host>), cleaning up the test taint immediately.

**Gateway API visibility** — added to the Ingresses page. Verified via curl against the real cluster: correctly returns `[]` since no Gateway API CRDs are installed there, rather than an error.

**Velero schedule creation** — "New schedule" dialog on Backups. Verified end-to-end against the real cluster (which has a genuine Velero install): created a real Schedule via the API, confirmed it, then removed it with `kubectl delete` since this pass intentionally has no delete-schedule endpoint.

**Pod scheduling diagnosis** — the single highest-value item in this batch: an automatic "why is this pod not running" banner on Pod detail, aggregating the `PodScheduled` condition, warning Events, and container waiting reasons. Verified in Chrome for both a healthy pod (no banner) and a real CrashLoopBackOff demo pod (banner with the correct diagnosis text), and via curl against the real cluster.

**RBAC write support, deliberately scoped to namespaced Role/RoleBinding** — `ClusterRole`/`ClusterRoleBinding` stay read-only on purpose (see ARCHITECTURE.md). Verified the full create→list→delete round trip via the real Chrome UI against demo mode (demo's RBAC lists moved from a static list to a mutable map so this actually persists), and via curl against the real cluster — created a real Role/RoleBinding, confirmed both in the live list, deleted both.

**GitOps sync trigger** — per-row Sync button for Argo Application (patches `operation.sync`) and Flux Kustomization/HelmRelease (bumps the `reconcile.fluxcd.io/requestedAt` annotation). Verified in Chrome against demo mode. The real cluster has neither Argo nor Flux installed, so `ListGitOps` correctly returns `[]` there and a sync attempt against a made-up target correctly surfaces the real Kubernetes "resource not found" error rather than pretending to succeed.

**Helm chart install** — the largest lift, and the one that surfaced real bugs via live testing. Installed and fully uninstalled a minimal `podinfo` chart in a throwaway namespace on the real cluster:
- First attempt failed: `helm install --wait` needs RBAC read on `apps/replicasets` to poll rollout status, which Axiom's ServiceAccount didn't have. Removed `--wait` from the install path (kept on uninstall/rollback, which already worked with it) since install is already framed to the caller as "started," not a synchronous result.
- Second attempt still failed with the identical RBAC error — turned out `helm install` needs to read ReplicaSets to apply a Deployment's owned resources at all, not just to wait on them. Added a read-only `apps/replicasets` RBAC grant (natural complement to the existing `deployments` write grant) and confirmed success on a fresh retry: real Deployment/Service/Pod came up healthy, `GET /api/v1/helm/releases` correctly listed the new release, then `POST .../uninstall` (the pre-existing, already-proven endpoint) removed it cleanly. Namespace and Jobs deleted afterward; confirmed zero residue on the shared cluster.

### Live remote deployment (real k3s cluster, <host>)

Ten deploys across this batch (one per item, plus a couple of `--quick` redeploys for bugfixes found mid-verification: the RBAC-forbidden-vs-empty CRD-browser UX gap from the previous pass's docs update, and the two Helm install RBAC fixes above). Every item re-verified against real cluster data or a real write-round-trip after each deploy, as summarized per item above.

## 2026-08-28 — Tier-1 Kubernetes-console gap closure: HPA, generic apply, PDB, NetworkPolicy CRUD, bulk actions, per-pod metrics, CRD browser

Seven features closing the remaining gaps against best-in-class Kubernetes consoles (OpenShift Console, Lens, Rancher), scoped from a fresh "what's missing" audit and implemented one at a time, backend-then-frontend, following the session's established Provider-interface + demo/kubernetes dual-implementation pattern:

```text
go test ./cmd/... ./internal/...                                        PASS
go vet ./cmd/... ./internal/...                                         PASS
go build ./cmd/zyvor-axiom                                              PASS (demo build)
go build -modfile=go.production.mod -tags kubernetes ./cmd/zyvor-axiom  PASS (kubernetes build)
npx craco build (frontend/)                                             PASS
```

**HorizontalPodAutoscaler** (list/create/delete) — new Autoscaling page. Verified via Chrome against the local demo server: opened the Create dialog, picked a real workload from the target-workload dropdown, created a real HPA, watched it appear in the list with a success toast, deleted it, confirmed the list returned to its original state. Verified against the real <host> cluster via curl (empty list — no HPAs exist on that cluster yet, which is itself the correct honest response).

**PodDisruptionBudgets** (read-only) — added as a third `Section` on the existing Quotas page. Verified via Chrome (demo) and curl (real cluster — six real PDBs across `kubevirt`, `packetwolf`, `rook-ceph`, `zeus-os`, `zyvor-relay`).

**NetworkPolicy CRUD** — list/delete on the Networking page plus a "Create via YAML" flow (reuses the generic apply endpoint rather than a bespoke rule-builder form). Verified via Chrome (demo: pre-filled template, apply correctly rejected with the demo-mode message) and curl (real cluster — ten real NetworkPolicies across `cluster-network-addons`, `ironwolf-system`, `packetwolf`, `zeus-os*`, `zyvor-qa-sandbox`).

**Generic server-side apply** — `POST /api/v1/apply` reuses `ValidateManifest`'s parse/discover path with a real `Patch(types.ApplyPatchType)` instead of a dry-run `Create`. Wired into three places: the Workloads launch wizard's YAML tab (Validate → Apply), the NetworkPolicy create-via-YAML flow, and new editable-YAML tabs on both Pod detail and Workload detail (replacing the old read-only `<pre>` with a `<textarea>` + Save button). Verified via Chrome against the demo server (all three surfaces correctly show the "not available in demo mode" message) and for real against the live cluster: applied a real `Namespace` manifest via curl, confirmed it was created, then deleted it immediately since it was only a verification artifact with no lasting purpose.

**Per-pod/container CPU & memory graphs** — new Metrics tab on Pod detail, reusing the `Sparkline` component (extracted from Overview.jsx into `frontend/src/components/Sparkline.jsx` for this purpose) against a new `GET /api/v1/pods/{ns}/{name}/metrics`, backed by the same raw `metrics.k8s.io` `AbsPath` trick as node metrics plus a small in-memory per-pod ring buffer. Verified via Chrome (demo: synthetic but realistic-looking CPU/memory curves) and curl (real cluster: real live CPU/memory numbers from a real Axiom pod, `"source":"metrics-server"`).

**Generic CRD browser** — new Custom Resources page: search/select any installed CRD, browse its instances via `AutoTable` (extracted from Atlas.jsx into `frontend/src/components/AutoTable.jsx`), open one for its raw manifest. Verified via Chrome (demo: PrometheusRule instances, detail dialog) and curl (real cluster: real CRDs from cert-manager, KubeVirt, and dozens of others). **Found a real UX gap during real-cluster verification**: the ClusterRole's RBAC for CRD instances is an intentional per-group allowlist (not a wildcard), so browsing a CRD outside that allowlist (e.g. cert-manager's `Challenge`) returned a real `403 Forbidden` from the Kubernetes API — but the frontend was swallowing that into the same "no instances found" empty state as a genuinely-empty CRD, which is actively misleading. Fixed by threading the actual error message through to the UI (`CustomResources.jsx`'s `instancesError` state) so a permission gap reads as "Permission denied: ..." instead of silently looking like there's just nothing there.

**Bulk multi-select actions** — no backend changes; a checkbox column plus an "N selected" toolbar on Pods (bulk evict) and Workloads (bulk restart/delete), firing `Promise.allSettled` over the existing single-resource endpoints. Verified via Chrome (demo): selected two pods, toolbar appeared with count; selected a workload, toolbar showed Restart/Delete buttons.

### Live remote deployment (real k3s cluster, <host>)

Full deploy with `./scripts/deploy-remote.sh <host> sus` (RBAC ClusterRole picked up the new `autoscaling`/`policy` verbs), then a `--quick` redeploy for the CRD-error-surfacing fix. All seven features re-verified against real cluster data after each deploy, as summarized above.

## 2026-08-28 (earlier) — audit persistence, CI, user edit, viewer banner, real Keycloak SSO, namespace RBAC — plus a regression found and fixed

Continuation of the "what else is missing" punch list, implemented one item at a time:

```text
go test ./cmd/... ./internal/...                                        PASS
go vet ./cmd/... ./internal/...                                         PASS
go build ./cmd/zyvor-axiom                                              PASS (demo build)
go build -modfile=go.production.mod -tags kubernetes ./cmd/zyvor-axiom  PASS (kubernetes build)
npx craco build (frontend/)                                             PASS
```

**Audit log persistence** — `internal/platform/audit.go` gained an optional `AuditStore`. Verified against the real <host> cluster: created a user (generating an audit entry), restarted the pod, confirmed the entry survived — persisted in a `zyvor-axiom-audit` ConfigMap.

**CI pipeline** — `.github/workflows/ci.yml` mirrors the exact sequence run manually all session (frontend build → demo vet/test/build → kubernetes vet/build → manifest YAML sanity check). Ran the full sequence locally end-to-end, including deliberately breaking `internal/ui/dist/` to confirm the embed-failure mode the step ordering exists to avoid. The workflow itself is correct but the first push-triggered run failed to start — a GitHub billing/spending-limit issue on the `zyvorai` org, unrelated to the workflow.

**Edit-role / reset-password for local users** — `PATCH /api/v1/users/{username}`. Verified via curl (create viewer → PATCH role to admin → PATCH password → authenticate with the new password as admin → write succeeds) and via the real Chrome UI (opened the new pencil-icon Edit dialog, changed a role from viewer to admin, watched the badge update with the correct toast).

**Viewer role banner** — a persistent strip under the TopBar for any Viewer session, closing the "no visual indication of read-only" gap. Verified in Chrome side-by-side: an admin tab shows nothing, a fresh local-viewer tab shows the banner consistently across pages.

**Real Keycloak SSO wiring (via Chrome, live infrastructure)** — configured a real Keycloak instance (<host>:30180) end-to-end: fixed its `sslRequired` policy blocking plain-HTTP admin login (required an admin action the user performed themselves — passwords are never entered by the assistant), created the `zyvor-axiom` OIDC client (public, PKCE, correct redirect URI for the live <host> deployment), created `platform-admins`/`platform-viewers` groups, added a Group Membership protocol mapper emitting a bare-name (not full-path) `groups` claim into the ID token, and wired Axiom's Settings → Integrations + `AXIOM_OIDC_ADMIN_GROUP`/`AXIOM_OIDC_VIEWER_GROUP`.

**Regression found and fixed along the way**: verifying OIDC persistence surfaced that a `PUT /api/v1/integrations` "succeeded" but didn't survive a pod restart — the multi-cluster `SwitchableProvider` change (earlier this session) had left `OpenIntegrationStore` and the interactive-terminal WebSocket handler still type-asserting `Provider` directly to `*KubernetesProvider`, which now always fails since `main.go` always passes a `*SwitchableProvider`. This silently downgraded **all** Integrations settings (Atlas/OIDC/Postgres/Probe/notifications) to in-memory-only, and completely broke the interactive Pod terminal (500 on every request) in production. Fixed with one shared `platform.CurrentKubernetesProvider` helper used everywhere this pattern was needed, instead of leaving near-duplicate type switches to individually drift. Verified for real: PUT the OIDC config, restart the pod, confirm it's still there; for the terminal, connected a raw Python `websocket-client` to the real endpoint against a live `virt-launcher` pod, sent `echo HELLO_FROM_REGRESSION_TEST`, and got real bash PTY output back.

**Full end-to-end SSO proof**: created a Keycloak user in `platform-viewers`, signed into Axiom for real (Keycloak login page → redirect → Axiom session), landed on real cluster data (76/80 real pods), saw the Viewer banner render (proving the group→role mapping worked), and confirmed a real write attempt from that browser session's own cookie got a real `403 read-only user` — not a curl simulation, the actual authenticated browser session.

**Namespace-scoped viewer accounts** — a `viewer` account can now be restricted to specific namespaces (`namespaces: []string` on `User`, enforced in the auth middleware via a fixed allowlist of namespaced-resource path prefixes, checked against `?namespace=` or the path segment). Added 8 table-driven Guard tests plus UserHub lifecycle tests. Verified against the real <host> cluster: created a viewer scoped to `aether-system`+`packetwolf`, confirmed `?namespace=aether-system` returned real pod data (200), `?namespace=axiom-system` was blocked (403), and omitting `?namespace=` entirely was also blocked (403, since that means "all namespaces" and would otherwise leak). Also verified the Add User dialog's new namespace-restriction field through the real Chrome UI, including the namespace badges rendering correctly in the Users table.

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

### Live remote deployment (real k3s cluster, <host>)

Redeployed with `./scripts/deploy-remote.sh <host> sus`. First deploy attempt surfaced a real RBAC gap: the ClusterRole never granted `resourcequotas`, `limitranges`, `ingresses` (networking.k8s.io), or `cronjobs` (batch), so those four endpoints returned `403 Forbidden` in production despite working fine in demo mode (the demo provider has no RBAC to enforce). Fixed `deploy/k8s/rbac.yaml`, redeployed, and reverified — all four now return real cluster data:

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
