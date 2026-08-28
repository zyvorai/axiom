# Zyvor Axiom API Reference

All endpoints except `GET /api/v1/health` require authentication (HTTP Basic, OIDC session, or API key when configured). JSON mutation bodies are limited to 1 MiB and reject unknown fields unless noted.

## Session and health

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/health` | Unauthenticated liveness/readiness |
| GET | `/api/v1/session` | Authenticated user, role, mode, cluster |
| GET | `/api/v1/overview` | Cluster health summary |
| GET | `/api/v1/regions` | Region / multi-cluster metadata |
| GET | `/api/v1/metrics/utilization` | CPU/memory utilization aggregates |
| GET | `/api/v1/realtime/capabilities` | SSE / streaming feature flags |

## Authentication

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/auth/providers` | OIDC, SAML, LDAP, lab operator login flags |
| GET | `/api/v1/auth/oidc/login` | Start OIDC authorize redirect |
| GET | `/api/v1/auth/oidc/callback` | OIDC callback (IdP redirect target) |
| GET | `/api/v1/auth/saml/login` | Redirect to SAML IdP (`AXIOM_SAML_LOGIN_URL`) |
| POST | `/api/v1/auth/logout` | Clear session cookie |

`GET /api/v1/auth/providers` returns `lab.operator_login: true` and `lab.hint: "demo / demo"` when no SSO is configured.

## Workloads

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/workloads?namespace=...` | List Deployments / StatefulSets |
| POST | `/api/v1/workloads` | Launch Deployment (+ optional Service) |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/scale` | Set desired replicas |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/start` | Restore saved replicas |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/stop` | Scale to zero (save replicas) |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/restart` | Rollout via template annotation |
| DELETE | `/api/v1/workloads/{namespace}/{kind}/{name}` | Delete controller + Axiom-owned Service |
| GET | `/api/v1/workloads/{namespace}/{kind}/{name}/still?action=` | Preview Still actions (hush/sleep/seal/end) |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/seal` | Apply deny-all NetworkPolicy isolation |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/unseal` | Remove Axiom isolation policy |
| GET | `/api/v1/workloads/{namespace}/{kind}/{name}/rollout` | Rollout status and revision history |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/rollout/{action}` | Rollout action (pause/resume/restart) |
| GET | `/api/v1/workloads/{namespace}/{kind}/{name}/network/guard` | PacketWolf / isolation summary |
| GET | `/api/v1/resources/{namespace}/{kind}/{name}` | Raw resource detail (YAML-oriented). `kind` ∈ `deployment\|statefulset\|daemonset\|pod` (case-insensitive) |

### Launch request

```json
{
  "namespace": "apps",
  "name": "hello-axiom",
  "image": "nginx:1.27",
  "replicas": 3,
  "port": 80,
  "expose": true
}
```

### Scale request

```json
{"replicas": 6}
```

## Pods

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/pods?namespace=...` | List Pods |
| GET | `/api/v1/pods/{namespace}/{name}/logs?container=&tail=200&follow=false&previous=false` | Read or stream logs (chunked `text/plain`, not SSE) |
| POST | `/api/v1/pods/{namespace}/{name}/exec` | Run a single command in a container, return captured output |
| GET | `/api/v1/pods/{namespace}/{name}/terminal?container=&shell=` | **kubernetes build only.** Upgrades to a WebSocket carrying a real interactive PTY (`TTY:true`). Client sends raw keystrokes (text or binary frames) plus a `__resize__:WIDTHxHEIGHT` control message; server streams raw output as binary frames. Gate on `GET /api/v1/realtime/capabilities`'s `interactiveExec` before use. |
| DELETE | `/api/v1/pods/{namespace}/{name}` | Terminate Pod |
| POST | `/api/v1/pods/{namespace}/{name}/evict` | Policy-aware eviction |
| GET | `/api/v1/pods/{namespace}/{name}/window/{port}/` | HTTP window proxy into Pod (reverse-proxies into the pod's container port) |

Exec accepts a shell string or argv array:

```json
{"container":"app","command":"whoami"}
```

```json
{"container":"app","command":["/bin/sh","-lc","env | sort"]}
```

`/logs`, `/exec`, `/terminal` and `/window/{port}/` are all reached through a plain browser navigation or WebSocket, neither of which can carry a custom `Authorization` header — for local Basic-auth sessions, append `?access_token=<base64 user:pass>` (see [Authentication](#authentication) below). OIDC/SAML cookie sessions need nothing extra.

## Nodes

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/nodes` | List nodes |
| GET | `/api/v1/nodes/{name}` | Node detail |
| POST | `/api/v1/nodes/{name}/cordon` | Mark unschedulable |
| POST | `/api/v1/nodes/{name}/uncordon` | Mark schedulable |
| POST | `/api/v1/nodes/{name}/drain` | Cordon and evict eligible Pods |

## Network and namespaces

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/namespaces` | List namespaces |
| POST | `/api/v1/namespaces` | Create namespace |
| DELETE | `/api/v1/namespaces/{name}` | Delete namespace |
| GET | `/api/v1/services?namespace=...` | List Services |
| GET | `/api/v1/network?namespace=...` | Service→Pod topology graph |
| GET | `/api/v1/packetwolf/status` | PacketWolf connectivity |
| GET | `/api/v1/packetwolf/proxy/{path...}` | Authenticated proxy to PacketWolf API |

Set `AXIOM_PACKETWOLF_URL` (and optional `AXIOM_PACKETWOLF_API_KEY`) to enable PacketWolf.

## Virtual machines (KubeVirt)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/vms?namespace=...` | List VirtualMachines |
| POST | `/api/v1/vms/{namespace}/{name}/start` | Start VM |
| POST | `/api/v1/vms/{namespace}/{name}/stop` | Stop VM |
| POST | `/api/v1/vms/{namespace}/{name}/migrate` | Live migration |
| GET | `/api/v1/vms/{namespace}/{name}/console` | virtctl / VNC hints |
| GET | `/api/v1/vm-snapshots` | List snapshots |
| POST | `/api/v1/vm-snapshots` | Create snapshot |
| GET | `/api/v1/vm-clones` | List clones |
| POST | `/api/v1/vm-clones` | Create clone |
| GET | `/api/v1/vm-exports` | List exports |
| POST | `/api/v1/vm-exports` | Create export |
| GET | `/api/v1/vm-restores` | List restores |
| POST | `/api/v1/vm-restores` | Restore from snapshot |

## Storage

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/volumes?namespace=...` | PVCs and storage classes |
| GET | `/api/v1/atlas` | Atlas summary (policies, pools, volumes) |
| GET | `/api/v1/atlas/{path...}` | Atlas API proxy |
| POST | `/api/v1/atlas/volumes` | Request Atlas volume (+ optional PVC) |

## Secrets and backups

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/secrets?namespace=...` | List Secrets (keys only) |
| POST | `/api/v1/secrets` | Create Secret |
| GET | `/api/v1/secrets/{namespace}/{name}` | Secret detail |
| DELETE | `/api/v1/secrets/{namespace}/{name}` | Delete Secret |
| GET | `/api/v1/external-secrets?namespace=...` | ExternalSecret CRs |
| GET | `/api/v1/backups?namespace=...` | Volume snapshots / backups |
| POST | `/api/v1/backups` | Create backup |
| POST | `/api/v1/backups/restore` | Restore PVC from snapshot |
| GET | `/api/v1/velero/backups?namespace=...` | Velero backups |
| GET | `/api/v1/velero/schedules?namespace=...` | Velero schedules |

## Helm, GitOps, ops

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/helm/releases` | Helm releases |
| GET | `/api/v1/helm/releases/{namespace}/{name}/history` | Release revision history |
| POST | `/api/v1/helm/releases/{namespace}/{name}/rollback` | Rollback (`{"revision": N}`) |
| POST | `/api/v1/helm/releases/{namespace}/{name}/uninstall` | Uninstall release |
| GET | `/api/v1/gitops` | GitOps applications (Flux/Argo when present) |
| GET | `/api/v1/alerts?namespace=...` | Alertmanager / Prometheus alerts |
| GET | `/api/v1/audit` | Axiom audit log |
| GET | `/api/v1/rbac?namespace=...` | Roles and bindings |
| GET | `/api/v1/cost` | Cost / utilization estimates |
| POST | `/api/v1/validate` | Dry-run manifest validation |

## Agent runs and platform intents

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/agent-runs?namespace=...` | List agent runs |
| POST | `/api/v1/agent-runs` | Create diagnostic agent run |
| GET | `/api/v1/agent-runs/{namespace}/{name}` | Agent run detail |
| POST | `/api/v1/agent-runs/{namespace}/{name}/approve` | Approve run |
| POST | `/api/v1/agent-runs/{namespace}/{name}/deny` | Deny run |
| GET | `/api/v1/platform-intents?namespace=...` | Platform intent CRs |

## Haven identity

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/haven/identity?namespace=...` | Haven identity plane summary |
| GET | `/api/v1/haven/planes` | Identity planes |
| GET | `/api/v1/haven/admin` | Haven admin status |

## Integrations (Settings)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/integrations` | Current integration config (secrets redacted) |
| PUT | `/api/v1/integrations` | Save Atlas, Postgres, OIDC, probe settings |
| GET | `/api/v1/integrations/examples` | Example integration payloads |
| POST | `/api/v1/integrations/atlas/test` | Test Atlas gateway |
| POST | `/api/v1/integrations/oidc/test` | Test OIDC discovery |
| POST | `/api/v1/integrations/postgres/test` | Test Postgres connectivity |
| POST | `/api/v1/integrations/probe/test` | HTTP / in-cluster probe |

## Intent placement

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/intent/parse` | Parse natural-language launch intent |
| POST | `/api/v1/intent/place` | Score and place workload from intent |

## Events

`GET /api/v1/events?namespace=...` returns cluster or namespace events sorted by time.

## Error shape

```json
{"error":"human-readable message"}
```

Accepted async mutations (Helm uninstall/rollback, some VM ops) return `202` with a `jobName` when the provider tracks background work.
