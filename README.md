# Zyvor Axiom

Zyvor Axiom is a Go-based private-cloud control plane for **k3s and Kubernetes**. It presents applications first, while still giving operators direct access to Pods, logs, exec, events, nodes, scaling, restart, stop/start, eviction and drain workflows.

This project intentionally contains **no Metal3 workload support**.

## UX

The embedded web UI is dependency-free and ships inside the Go binary. Its design combines Zyvor's editorial open-infrastructure language with a cinematic Apple-style product surface.

- Dark / System / Light appearance modes
- Appearance preference persisted in `localStorage`
- Oversized private-cloud hero and Axiom visual core
- Glass command bar and compact expandable navigation
- Workload-first Cloud Canvas
- Workload, Pod and Node detail drawers
- Launch workload sheet
- Pod logs and command execution console
- `Cmd/Ctrl + K` command palette
- Responsive layout for desktop, tablet and mobile
- No CDN, external fonts or browser package dependencies

## Backend

The core project uses only the Go standard library so the demo build is reproducible in restricted environments.

Production Kubernetes support is kept behind the `kubernetes` build tag and uses:

- `client-go`
- Kubernetes Deployments and StatefulSets
- Kubernetes Pod log API
- Kubernetes exec/SPDY API
- Eviction API for Pod disruption
- Kubernetes Nodes API for cordon/drain
- Kubernetes Events API

### API surface

See [`docs/API.md`](docs/API.md) for the full reference (~80 routes). Core groups:

- **Cluster:** health, session, overview, regions, utilization, events
- **Workloads:** CRUD, scale, stop/start, restart, seal/unseal, rollout, Still previews
- **Pods:** list, logs, exec, evict, window proxy
- **Nodes:** list, cordon, uncordon, drain
- **Network:** namespaces, services, topology, PacketWolf status/proxy
- **VMs:** KubeVirt lifecycle, snapshots, clones, exports, restores
- **Ops:** Helm, GitOps, secrets, backups, Velero, alerts, audit, RBAC, cost
- **Platform:** Atlas, Haven identity, agent runs, integrations, intent placement
- **Auth:** providers, OIDC/SAML login, logout

## Run the tested local demo

```bash
go test ./...
go vet ./...
go build -o bin/zyvor-axiom ./cmd/zyvor-axiom

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

## Synchronize web assets

The Go binary embeds `internal/ui/dist`.

```bash
./scripts/sync-ui.sh
```

Then rebuild the Go binary.

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


## Detailed documentation

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — runtime and provider design
- [`docs/API.md`](docs/API.md) — complete HTTP API reference
- [`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md) — k3s/Kubernetes deployment and hardening
- [`docs/DESIGN-SYSTEM.md`](docs/DESIGN-SYSTEM.md) — Axiom 17 dark/light UX system
- [`docs/RENAMING.md`](docs/RENAMING.md) — full Aether → Axiom identity map

## Project layout

```text
cmd/zyvor-axiom/          server entrypoint
internal/api/              HTTP API and realtime hooks
internal/auth/             Basic-auth reference middleware
internal/config/           environment configuration
internal/platform/         provider model, demo + Kubernetes implementations
internal/ui/               embedded web assets
web/                       editable UX source
scripts/                   development helpers
deploy/k8s/                Kubernetes deployment and RBAC
```
