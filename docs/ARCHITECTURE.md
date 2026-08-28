# Zyvor Axiom Architecture

```text
Browser (vanilla JS SPA, embedded in Go binary)
  |  HTTPS + JSON
  |  SSE state stream · log follow · pod exec WebSocket
  v
Zyvor Axiom API (Go net/http)
  |
  +-- Auth middleware (Basic · OIDC · SAML bridge · API key)
  +-- Workload lifecycle API (scale · stop/start · seal/unseal · rollout)
  +-- Pod API (logs · exec · evict · window proxy)
  +-- Node API (cordon · drain)
  +-- Network + PacketWolf proxy
  +-- KubeVirt VM lifecycle
  +-- Atlas / Haven / integrations hub
  +-- Helm · GitOps · Velero · agent runs
  +-- Embedded UX (web/ → internal/ui/dist/)
  |
Provider interface (internal/platform)
  |
  +-- DemoProvider (stdlib, deterministic local lab)
  |
  +-- KubernetesProvider (-tags kubernetes, client-go)
        |
        +-- Deployments / StatefulSets / Services
        +-- Pods / Logs / Exec / Eviction
        +-- Nodes / Events / Namespaces
        +-- KubeVirt CRDs (when installed)
        +-- Helm 3 releases (when CLI available)
        +-- External Secrets · Velero · Flux/Argo (best-effort list)
        v
      k3s / Kubernetes API server
```

## Provider boundary

`internal/platform/provider.go` is the contract between the API and runtime. The UX never knows whether it is connected to the deterministic lab or a real cluster — the same JSON shapes are returned in both modes.

This makes local product development possible without faking HTTP endpoints in the frontend.

## Realtime layer

`GET /api/v1/realtime/capabilities` advertises whether SSE and WebSocket exec are available for the current build profile. The UI connects an event stream after login to refresh overview metrics without full page reloads.

## Integrations hub

Settings persist integration config in-memory (demo) or via the cluster config hub (kubernetes). Supported integrations:

- **Atlas** — storage gateway proxy and volume intent API
- **Postgres** — platform database connectivity probe
- **OIDC** — issuer/login URL for SSO buttons
- **HTTP probe** — curl-from-pod or external URL test
- **PacketWolf** — `AXIOM_PACKETWOLF_URL` env; status + authenticated API proxy

## Haven identity

When Haven Keycloak is installed in the cluster, `/api/v1/haven/*` surfaces identity plane health for the Settings → Identity panel.

## Kubernetes lifecycle semantics

Axiom does not expose a misleading VM-like "power" abstraction for Kubernetes Pods.

- Workload **Stop** scales Deployments/StatefulSets to zero.
- The previous desired replica count is persisted in `axiom.zyvor.dev/last-replicas` and restored by **Start**.
- **Restart** modifies the Pod template annotation `axiom.zyvor.dev/restarted-at`.
- **Seal / Unseal** apply and remove a deny-all NetworkPolicy for immediate isolation; PacketWolf enriches flow visibility when configured.
- Pod **Evict** uses the policy eviction subresource.
- Node **Drain** cordons the node, skips DaemonSets/mirror Pods/system namespace Pods, then requests evictions.

## Build profiles

### Demo

The default build has zero third-party Go dependencies.

```bash
go build ./cmd/zyvor-axiom
```

### Kubernetes

The production provider is selected at compile time.

```bash
go build -modfile=go.production.mod -tags kubernetes ./cmd/zyvor-axiom
```

The provider first tries in-cluster configuration, then `KUBECONFIG`, then `$HOME/.kube/config`.

## UI source layout

| Path | Role |
|------|------|
| `web/app.js` | SPA logic (~4k lines) |
| `web/styles.css` | Liquid glass design tokens |
| `web/lucide-icons.js` | Lucide SVG helpers (zeus-os aligned) |
| `internal/ui/dist/` | Embedded copy (run `./scripts/sync-ui.sh` before build) |
