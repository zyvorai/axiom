# Zyvor Axiom

Zyvor Axiom is a Go-based private-cloud control plane for **k3s and Kubernetes**. It presents applications first, while still giving operators direct access to Pods, logs, an interactive terminal, events, nodes, scaling, restart, stop/start, eviction and drain workflows.

This project intentionally contains **no Metal3 workload support**.

## UX

The web UI is a React 19 + Tailwind + shadcn/ui single-page app ("Liquid Glass" design system), built with CRACO and embedded inside the Go binary. Source lives in `frontend/`; `scripts/sync-ui.sh` builds it and copies the output into `internal/ui/dist/` (a generated, gitignored build artifact — never edit `internal/ui/dist/` directly).

Pages:

- **Getting Started** — live setup checklist (SSO, Atlas, PacketWolf, Postgres, probe) driven by real `/api/v1/integrations` + `/api/v1/auth/providers` status
- **Overview** — cluster hero, metric rings, utilization sparklines, alerts, activity
- **Nodes** — capacity, kubelet status, cordon/uncordon/drain
- **Workloads** — Deployments/StatefulSets/DaemonSets/CronJobs, with a full action menu (start/stop/restart/scale/seal/unseal/delete), a rollout dialog (history + pause/resume/restart/undo), and a "Run workload" wizard (Form / natural-language Intent / YAML validation)
- **Workload detail** (`/workloads/:ns/:kind/:name`) — labels/annotations, raw manifest YAML, owned Pods, related events
- **Pods** — list with per-row exec and evict
- **Pod detail** (`/pods/:ns/:name`) — full container specs (resources, env, mounts, probes), volumes, tolerations, owner, conditions, raw YAML, an **interactive PTY terminal** (real WebSocket shell — falls back to one-shot exec in demo mode), and per-port "Open" buttons that proxy straight into the pod's container port from the browser
- **Platform Intents** — workloads placed via the Intent launch flow
- **Networking · Cilium** — service graph, plus a live PacketWolf (eBPF/Cilium flow) panel when `AXIOM_PACKETWOLF_URL` is configured
- **Services**, **Namespaces** (with create/delete)
- **Virtual Machines** — KubeVirt lifecycle (start/stop/migrate), real console info (`virtctl vnc` command — Axiom has no embedded browser VNC client), and Snapshots/Clones/Exports/Restores tabs
- **Storage** — PVCs grouped by StorageClass
- **Atlas** — live storage control-plane view (backends/pools/volumes/buckets/policies) when Atlas is connected, with a "Create volume" flow
- **Pod Logs** — chunked-HTTP live tail with container selection, deep-linkable from Pod detail
- **Events**, **Secrets** (+ External Secrets, create/delete), **Backups** (VolumeSnapshots + Velero, create/restore), **Alerts**
- **Ops hub** — tabbed Helm / GitOps / Agents / Identity / Audit / Access (RBAC) / Cost
- **Settings** — OIDC, Atlas, Postgres, Probe integration cards with save + test-connection

Real auth throughout: local Basic-auth sign-in form plus SSO buttons that redirect to OIDC/SAML when configured (`GET /api/v1/auth/providers`), with a lab-mode "fill demo/demo" hint when no SSO is set up.

## Backend

The core project uses only the Go standard library so the demo build is reproducible in restricted environments.

Production Kubernetes support is kept behind the `kubernetes` build tag and uses:

- `client-go`
- Kubernetes Deployments, StatefulSets, DaemonSets
- Kubernetes Pod log API
- Kubernetes exec/SPDY API — both one-shot (`ExecCommand`) and a real interactive PTY over WebSocket (`ExecWS`)
- Eviction API for Pod disruption
- Kubernetes Nodes API for cordon/drain
- Kubernetes Events API

### API surface

See [`docs/API.md`](docs/API.md) for the full reference (~90 routes). Core groups:

- **Cluster:** health, session, overview, regions, utilization, events
- **Workloads:** CRUD, scale, stop/start, restart, seal/unseal, rollout, Still previews
- **Pods:** list, logs, one-shot exec, interactive terminal (WebSocket), evict, window proxy
- **Nodes:** list, cordon, uncordon, drain
- **Network:** namespaces, services, topology, PacketWolf status/proxy
- **VMs:** KubeVirt lifecycle, snapshots, clones, exports, restores
- **Ops:** Helm, GitOps, secrets, backups, Velero, alerts, audit, RBAC, cost
- **Platform:** Atlas, Haven identity, agent runs, integrations, intent placement
- **Auth:** providers, OIDC/SAML login, logout

## Run the tested local demo

```bash
go test ./cmd/... ./internal/...
go vet ./cmd/... ./internal/...
make build   # builds the frontend, then the Go binary

AXIOM_MODE=demo \
AXIOM_USERNAME=demo \
AXIOM_PASSWORD=demo \
AXIOM_ADDR=:5090 \
./bin/zyvor-axiom
```

Open `http://localhost:5090` — the login form appears automatically in lab mode. Sign in with **`demo` / `demo`**.

Remote k3s lab deploys use the same credentials unless you override `AXIOM_USERNAME` / `AXIOM_PASSWORD`. See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Production Kubernetes build

The production module manifest is separate so the dependency-free demo build can be compiled without downloading Kubernetes modules.

```bash
go mod download -modfile=go.production.mod
./scripts/sync-ui.sh   # builds frontend/ and refreshes internal/ui/dist/
go build -modfile=go.production.mod -tags kubernetes -o bin/zyvor-axiom-kubernetes ./cmd/zyvor-axiom
```

Run using in-cluster credentials:

```bash
AXIOM_MODE=kubernetes \
AXIOM_USERNAME=demo \
AXIOM_PASSWORD='replace-me' \
AXIOM_ADDR=:5090 \
./bin/zyvor-axiom-kubernetes
```

Or outside the cluster:

```bash
KUBECONFIG=$HOME/.kube/config AXIOM_MODE=kubernetes ./bin/zyvor-axiom-kubernetes
```

Only the `kubernetes`-tagged build registers the interactive terminal WebSocket route and Pod `resources` support; the demo build's Pod detail page falls back to one-shot exec automatically (via `GET /api/v1/realtime/capabilities`).

## Build the frontend

The Go binary embeds `internal/ui/dist`, which is a **generated build artifact** (gitignored) — never commit changes to it directly.

```bash
./scripts/sync-ui.sh
```

This runs `npm install && npx craco build` inside `frontend/` and copies the output into `internal/ui/dist/`, then rebuild the Go binary. Requires Node 20+ and npm locally (not needed if you only build via Docker — the multi-stage `Dockerfile` has its own `node:20-slim` build stage).

For local frontend development against a running backend:

```bash
make demo &                 # backend on :5090
cd frontend && npm start    # dev server on :3000, proxies /api to :5090 (see package.json "proxy")
```

## Workload semantics

Axiom treats workload lifecycle semantically rather than pretending a Pod is a VM:

- **Stop Deployment/StatefulSet**: save desired replicas in `axiom.zyvor.dev/last-replicas`, then scale to zero.
- **Start**: restore the saved replica count.
- **Restart**: patch the Pod template using `axiom.zyvor.dev/restarted-at`.
- **Scale**: update the native workload replica count.
- **Evict Pod**: use Kubernetes Eviction API in production.
- **Drain Node**: skip DaemonSet and `kube-system` Pods and evict ordinary workloads.

## Security

Lab deploys use HTTP Basic auth (`AXIOM_USERNAME` / `AXIOM_PASSWORD`, default `demo` / `demo`) over HTTPS with a self-signed certificate. Production deployments should:

- Replace the lab cert with a CA-issued TLS certificate
- Enable OIDC/SAML via Settings → Integrations or environment variables
- Place Axiom behind an identity-aware proxy when using LDAP (`AXIOM_LDAP_ENABLED`)
- Grant the ServiceAccount only the RBAC verbs in `deploy/k8s/rbac.yaml` and narrow namespaces further where possible

A browser can't attach an `Authorization` header to a plain link or WebSocket, which the pod-window proxy and interactive terminal both need for local Basic-auth sessions. Axiom accepts a `?access_token=<base64 user:pass>` query parameter for exactly this case (`internal/auth/basic.go`); OIDC/SAML sessions ride their cookie and need nothing extra.

## Storage: Atlas

Axiom's Atlas page (and `/api/v1/atlas*`) talks to [Atlas](../atlas), Zyvor's separate storage control-plane service, when `AXIOM_ATLAS_URL` (and optionally `AXIOM_ATLAS_TOKEN`) is configured — via Settings → Integrations, or automatically when deploying with `./scripts/deploy-remote.sh <host> <user> --with-atlas` (see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)). Atlas is entirely optional; without it the Atlas page shows an honest "not connected" state.

## Detailed documentation

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — runtime and provider design
- [`docs/API.md`](docs/API.md) — complete HTTP API reference
- [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) — k3s/Kubernetes deployment and hardening
- [`docs/DESIGN-SYSTEM.md`](docs/DESIGN-SYSTEM.md) — Axiom "Liquid Glass" UX system
- [`docs/RENAMING.md`](docs/RENAMING.md) — full Aether → Axiom identity map

## Project layout

```text
cmd/zyvor-axiom/          server entrypoint
internal/api/              HTTP API and realtime hooks
internal/auth/             Basic-auth reference middleware
internal/config/           environment configuration
internal/platform/         provider model, demo + Kubernetes implementations
internal/ui/               embed.go + generated dist/ (gitignored build output)
frontend/                  React UX source (CRACO/Tailwind/shadcn) — edit here
scripts/                   development and deploy helpers
deploy/k8s/                Kubernetes deployment and RBAC
```
