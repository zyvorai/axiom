# Zyvor Axiom API Reference

All endpoints except `GET /api/v1/health` require authentication (HTTP Basic — including an LDAP simple-bind fallback when `AXIOM_LDAP_URL` is set — an OIDC or SAML session cookie, or an API key when configured). JSON mutation bodies are limited to 1 MiB and reject unknown fields unless noted. Authenticated callers with a `viewer` role (see [Users](#users)) can call every `GET` route but get `403` on any mutating one.

## Errors and validation

Errors are JSON, `{"error": "message"}`. Status classes: `400` invalid input, `401` / `403` not authenticated / not allowed, `404` not found, `409` already exists or nothing to do (e.g. rollback / undo with no previous revision), `429` blocked by a PodDisruptionBudget (eviction, drain), `5xx` a real server or upstream failure. Both the demo and Kubernetes builds classify the same way (see [ARCHITECTURE.md](ARCHITECTURE.md#http-status-mapping)). Notable validation rules:

- Workload `name` must be an RFC 1123 label (lowercase letters, digits, `-`; max 63); replicas must be an integer >= 0.
- `POST /hpas`: `maxReplicas` below `minReplicas` is a `400` (an omitted max defaults to min).
- `POST /secrets` needs at least one `data` / `keys` entry.
- `POST /users`: `username` must not contain `:` or control characters (HTTP Basic cannot carry them); password at least 8 characters.
- `POST /apply` and `POST /validate` take exactly one object — a multi-document (`---`) manifest is reported invalid.
- Sizes and quantities (`size`, `cpu`, `memory`, quota `hard`) that do not parse are `400`.

## Session and health

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/health` | Unauthenticated liveness/readiness |
| GET | `/api/v1/session` | Authenticated user, role, mode, cluster |
| GET | `/api/v1/overview` | Cluster health summary |
| GET | `/api/v1/regions` | Legacy cosmetic region breakdown of the active cluster (kept for the existing hero/topbar copy) |
| GET | `/api/v1/metrics/utilization` | CPU/memory utilization aggregates |
| GET | `/api/v1/realtime/capabilities` | SSE / streaming feature flags |

`GET /api/v1/session` returns `role` as a human label (`"Platform Admin"` or `"Viewer"`) resolved from whichever account authenticated the request — the env operator account and API-key auth are always admin; local accounts (see [Users](#users)) and OIDC/SAML sessions carry their own role (SSO sessions default to admin today).

## Clusters

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/clusters` | List registered clusters, with the caller's own currently-active one flagged |
| POST | `/api/v1/clusters` | Register a fleet cluster from a kubeconfig: `{"name","kubeconfig"}` (`kubeconfig` is the raw YAML content) |
| DELETE | `/api/v1/clusters/{name}` | Unregister a fleet cluster |
| POST | `/api/v1/clusters/{name}/activate` | Switch the active cluster for the calling browser tab/script only |

Register/unregister (and Settings → Clusters, which drives them) are restricted to `admin` — see [Users](#users)'s operator-role note. The backend `Provider` is always a `SwitchableProvider` delegating to whichever cluster is active *for the caller*, not one global switch — see `docs/ARCHITECTURE.md`'s Multi-cluster section. The frontend sends an `X-Axiom-Session` header (one random ID per browser tab) so switching clusters in one tab never affects another tab or another user; a request sent with no such header (e.g. a scripted `curl`) is scoped per authenticated username instead. A normal in-cluster (ServiceAccount) deployment registers exactly one cluster — this is not a stub, it's the honest single-cluster case. A kubeconfig with multiple contexts registers one provider per context. Demo mode registers three independent demo clusters.

## Authentication

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/auth/providers` | OIDC, SAML, LDAP, lab operator login flags |
| GET | `/api/v1/auth/oidc/login` | Start OIDC authorize redirect |
| GET | `/api/v1/auth/oidc/callback` | OIDC callback (IdP redirect target) |
| GET | `/api/v1/auth/saml/login` | Redirect to SAML IdP (`AXIOM_SAML_LOGIN_URL`) |
| POST | `/api/v1/auth/saml/acs` | SAML Assertion Consumer Service (HTTP-POST binding only) — verifies the response's XML-DSig signature against `AXIOM_SAML_IDP_CERT`, checks audience and validity window, then mints the same session cookie an OIDC login does. Lab-only `AXIOM_SAML_ALLOW_UNSIGNED=true` skips verification. |
| POST | `/api/v1/auth/logout` | Clear session cookie |

**OIDC verification.** The callback verifies the ID token's signature against the provider's JWKS (fetched from the discovery document's `jwks_uri`, 10 s timeout, cached 15 min, re-fetched at most every 30 s on an unknown `kid`), accepts only RS256/384/512, PS256/384/512 and ES256/384/512 (never `none` or HMAC), then checks `iss`, `aud` (and `azp` for multi-audience tokens), `exp`, `sub` and the `nonce` bound to the sign-in by the `axiom_oidc_nonce` cookie. Any failure rejects the login with `401`.

**Login throttling.** Failed credentials are throttled twice: per client IP (grace of 3, then exponential backoff to 5 min) and per attempted account name (grace of 5, same backoff, keyed on the lower-cased username whether or not the account exists). A successful login clears the per-IP counter and only the account counter of the account whose own credentials were verified, so interleaving a valid login from another account no longer defeats throttling. A throttled request gets `429` with `Retry-After`; lockouts are audited as `auth.rate_limited` / `auth.account_rate_limited`. Lockout is per account, so someone who knows a username can temporarily lock it out - it clears by itself within 5 minutes.

`GET /api/v1/auth/providers` returns `lab.operator_login: true` and `lab.hint: "demo / demo"` when no SSO is configured, and an `ldap` block (`enabled`/`domain`/`url`) when `AXIOM_LDAP_*` is set — LDAP has no separate login form; the same username/password fields on the local sign-in form simple-bind against `AXIOM_LDAP_URL` first, falling through to local accounts on failure.

### Stream tickets

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/ws-ticket` | Mint a single-use ticket for one streaming GET path: `{"path":"/api/v1/pods/{ns}/{name}/terminal"}` → `{"ticket","path","expiresIn":30,"expiresAt"}` |

Browser WebSocket/EventSource constructors cannot set headers, and credentials in the URL (`?access_token=`, `?axiom_session=`, `?api_key=`) are captured by reverse-proxy access logs. Instead, call `POST /ws-ticket` (any authenticated role, including read-only viewers and namespace-restricted accounts) and open the stream with `?ticket=<ticket>`. A ticket is random, bound to the caller's identity and to exactly that path, valid for 30 seconds and destroyed the first time it is presented (even against the wrong path). The usual role and namespace checks are re-applied when it is redeemed (a viewer's ticket for a terminal is still `403`). Eligible paths: `/api/v1/events/stream`, `/api/v1/pods/{ns}/{name}/terminal`, `/api/v1/pods/{ns}/{name}/logs`, `/api/v1/vms/{ns}/{name}/vnc`, `/api/v1/vms/{ns}/{name}/console`; anything else is `400`. A bad, reused, expired or mismatched ticket is `401` and is never rescued by other credentials on the same request. Responses are `Cache-Control: no-store`.

The URL-credential forms still work but are **deprecated**: a request that authenticates with them gets `Deprecation: true` and an `X-Axiom-Deprecated` header, and the server logs a one-time warning.

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

## Jobs and CronJobs

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/jobs?namespace=...` | List Jobs (status, completions, duration, owning CronJob if any) |
| DELETE | `/api/v1/jobs/{namespace}/{name}` | Delete a Job |
| GET | `/api/v1/cronjobs?namespace=...` | List CronJobs (schedule, suspend state, last-scheduled time) |
| POST | `/api/v1/cronjobs/{namespace}/{name}/trigger` | Run now — builds a Job from the CronJob's JobTemplate, same as `kubectl create job --from=cronjob` |
| POST | `/api/v1/cronjobs/{namespace}/{name}/suspend` | `{"suspend": true\|false}` — suspend or resume the schedule |
| DELETE | `/api/v1/cronjobs/{namespace}/{name}` | Delete a CronJob |

## ConfigMaps, quotas, and ingresses

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/configmaps?namespace=...` | List ConfigMaps (namespace, name, keys) |
| POST | `/api/v1/configmaps` | Create a ConfigMap (`{"namespace","name","data":{...}}`) |
| GET | `/api/v1/configmaps/{namespace}/{name}` | ConfigMap detail (full data) |
| PUT | `/api/v1/configmaps/{namespace}/{name}` | Replace a ConfigMap's data: `{"data":{...}}` |
| DELETE | `/api/v1/configmaps/{namespace}/{name}` | Delete a ConfigMap |
| GET | `/api/v1/resourcequotas?namespace=...` | ResourceQuotas (hard vs. used, per namespace) |
| POST | `/api/v1/resourcequotas` | Create a ResourceQuota: `{"namespace","name","hard":{...}}` |
| DELETE | `/api/v1/resourcequotas/{namespace}/{name}` | Delete a ResourceQuota |
| GET | `/api/v1/limitranges?namespace=...` | LimitRanges (Container/Pod/PVC default/min/max) |
| GET | `/api/v1/ingresses?namespace=...` | Ingresses (class, hosts, TLS, load-balancer address) — the OpenShift-Route equivalent |
| POST | `/api/v1/ingresses` | Create an Ingress: `{"namespace","name","host","service","port","class","tlsSecret"}` (`class`/`tlsSecret` optional) |
| DELETE | `/api/v1/ingresses/{namespace}/{name}` | Delete an Ingress |

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
| GET | `/api/v1/pods/{namespace}/{name}/metrics` | Live CPU/memory for the pod and each container, plus a short in-memory rolling time series (not historical Prometheus data) |

Exec accepts a shell string or argv array:

```json
{"container":"app","command":"whoami"}
```

```json
{"container":"app","command":["/bin/sh","-lc","env | sort"]}
```

`/logs`, `/exec`, `/terminal` and `/window/{port}/` are all reached through a plain browser navigation or WebSocket, neither of which can carry a custom `Authorization` header. For `/terminal` and other streams use a single-use [stream ticket](#stream-tickets) (`?ticket=`); for the `/window/{port}/` HTTP proxy, which loads many sub-resources, local Basic-auth sessions still append the deprecated `?access_token=<base64 user:pass>` (see [Authentication](#authentication) below). OIDC/SAML cookie sessions need nothing extra.

## Autoscaling, disruption budgets, network policies

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/hpas?namespace=...` | List HorizontalPodAutoscalers |
| POST | `/api/v1/hpas` | Create an HPA: `{"namespace","name","targetKind","targetName","minReplicas","maxReplicas","targetCpuPercent"}` |
| DELETE | `/api/v1/hpas/{namespace}/{name}` | Delete an HPA |
| GET | `/api/v1/poddisruptionbudgets?namespace=...` | List PodDisruptionBudgets (read-only) |
| GET | `/api/v1/networkpolicies?namespace=...` | List NetworkPolicies |
| DELETE | `/api/v1/networkpolicies/{namespace}/{name}` | Delete a NetworkPolicy |

### NetworkPolicy analysis

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/networking/policies/{namespace}/analysis` | Per-pod effective NetworkPolicy position in one namespace |

Returns `policies`, `pods[]` (`pod`, `labels`, `policies`, `ingressIsolated`, `egressIsolated`, and the allowed `ingress[]` / `egress[]` peers with `kind` = `any`\|`pods`\|`namespace`\|`cidr`, the matched `pods`/`namespaces`, `cidr`/`except` and `ports`), `uncovered` (pods selected by no policy: fully open both ways), `defaultDenyIngress`/`defaultDenyEgress`, `warnings` and `notes`. Computed from the policies' selectors and ports with Kubernetes semantics (union of all selecting policies; `policyTypes` default; empty `from`/`to`/`ports` = everything). Pod names are resolved only inside the analysed namespace; cross-namespace peers list the matching namespace names. The namespace is required (`all` is `400`) and is checked against namespace-restricted accounts like any `{namespace}` path. It does not know whether the CNI enforces policies.

Creating a NetworkPolicy has no dedicated form endpoint — use `POST /api/v1/apply` with a NetworkPolicy manifest (see [Generic apply](#generic-apply) below).

## Generic apply

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/apply` | Server-side apply of any manifest: `{"manifest":"<yaml or json>"}` |
| POST | `/api/v1/apply/diff` | Preview of `apply`: live vs proposed per object, nothing is changed (operator/admin; denied to namespace-restricted accounts like `apply`) |

**kubernetes build only.** Parses the manifest, resolves its GVK to a GVR via discovery, and applies it with `types.ApplyPatchType` / `FieldManager: "zyvor-axiom"` — a real `kubectl apply`, not a dry run (see [`POST /api/v1/validate`](#helm-gitops-ops) for dry-run-only validation). In the demo build this always returns `{"valid":false,"message":"apply is not available in demo mode — requires a real cluster (build with -tags kubernetes)"}`.

Response shape:

```json
{"valid":true,"kind":"Deployment","apiVersion":"apps/v1","namespace":"apps","name":"hello-axiom","message":"applied"}
```

```json
{"valid":false,"errors":["human-readable parse or apply error"]}
```

### Change preview (`POST /api/v1/apply/diff`)

Same body as `apply`. On a real cluster it issues the same forced server-side apply with `dryRun=All`, so defaulting, validation and admission run and the response shows the object as it *would* be stored next to the live object (server-managed fields such as `managedFields`, `status`, `resourceVersion` removed; Secret `data`/`stringData` values replaced by `<redacted>`). Read-only, so it is not audited. The UI (Apply dialog, Pod/Workload YAML tabs) enables **Apply** only after a successful preview of the exact text being applied.

```json
{"valid":true,"objects":[{"kind":"Deployment","apiVersion":"apps/v1","namespace":"apps","name":"hello","exists":true,"changed":true,
  "live":"...yaml...","proposed":"...yaml...","foreignManagers":["argocd-application-controller"]}],
 "warnings":["Deployment hello is also managed by argocd-application-controller. Axiom applies with force, so conflicting fields move to Axiom and the next sync by that tool may revert or fight this change."]}
```

`foreignManagers` lists `managedFields` managers other than Axiom and core cluster components (Argo CD, Flux, Helm, kubectl, ...). Invalid input or an API-server rejection comes back as `{"valid":false,"errors":[...]}`. In the demo build the preview compares against the demo object and `live`/`proposed` are JSON.

## Resource graph

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/graph/{namespace}/{kind}/{name}` | Relationship graph: owner chain (Deployment → ReplicaSet → Pods), Services whose selector matches, Ingresses routing to them, HPAs, and the ConfigMap/Secret/PVC names the pod template references. `kind` ∈ `deployment\|statefulset\|daemonset\|replicaset\|job\|cronjob\|pod` |

Namespaced like other resource reads (namespace-restricted accounts may only query their namespaces). Only names and short statuses are returned — never ConfigMap or Secret contents. Response: `{"root":"deployment/ns/name","nodes":[{"id","kind","namespace","name","status"}],"edges":[{"from","to","relation"}],"truncated":false}` where `relation` ∈ `owns|selects|routes|scales|mounts`; at most 30 pods / 120 nodes are listed (`truncated` is set when more exist).

## Custom resources (CRDs)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/crds` | List installed CustomResourceDefinitions (group, version, kind, plural, scope, age) |
| GET | `/api/v1/crds/{group}/{version}/{plural}?namespace=...` | List instances of a CRD. Omit `namespace` for cluster-scoped CRDs or to list across all namespaces for a namespaced CRD |
| GET | `/api/v1/crds/{group}/{version}/{plural}/{name}?namespace=...` | Get one CRD instance's raw manifest |

Listing CRD *definitions* only requires the `apiextensions.k8s.io` RBAC grant Axiom always ships with. Listing/getting CRD *instances* requires the ServiceAccount to actually have RBAC for that specific group/resource — `deploy/k8s/rbac.yaml` grants a curated allowlist (NetworkPolicies, PrometheusRule, KubeVirt, Argo CD, Flux, etc.), not a wildcard. Browsing a CRD outside that allowlist returns the real Kubernetes `403 Forbidden` message from the API server, not a misleading empty list.

## Nodes

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/nodes` | List nodes |
| GET | `/api/v1/nodes/{name}` | Node detail (includes `taints`) |
| POST | `/api/v1/nodes/{name}/cordon` | Mark unschedulable |
| POST | `/api/v1/nodes/{name}/uncordon` | Mark schedulable |
| POST | `/api/v1/nodes/{name}/drain` | Cordon and evict eligible Pods |
| GET | `/api/v1/nodes/{name}/metrics` | Live CPU/memory for the node plus a short in-memory rolling time series (not historical Prometheus data) — shares the per-pod metrics ring buffer, keyed `"node:<name>"` |
| POST | `/api/v1/nodes/{name}/taints` | Add a taint: `{"key","value","effect"}` (`effect` one of `NoSchedule`\|`PreferNoSchedule`\|`NoExecute`) |
| DELETE | `/api/v1/nodes/{name}/taints/{key}?effect=...` | Remove a taint matching `key` and `effect` |

A cordoned node's `status` reports `"SchedulingDisabled"` (and `unschedulable: true`) instead of its usual `"Ready"`/`"NotReady"` — this is how the Nodes page decides whether to show Cordon or Uncordon. Uncordon reverts `status` to the normal condition-derived value.

## Pod diagnostics

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/pods/{namespace}/{name}/diagnosis` | Aggregates the pod's `PodScheduled` condition, matching warning Events, and container waiting reasons into a `{"scheduled","phase","diagnosis",...}` plain-English answer to "why is this pod not running" |

## Storage objects

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/storageclasses` | List StorageClasses (provisioner, reclaim policy, binding mode, default flag) |
| GET | `/api/v1/persistentvolumes` | List cluster-scoped PersistentVolumes (capacity, access modes, claim ref, storage class) |
| GET | `/api/v1/priorityclasses` | List PriorityClasses (value, global-default flag, description) — read-only |

## Gateway API

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/gateways?namespace=...` | List GatewayClass/Gateway/HTTPRoute objects. Returns `[]` (not an error) when the Gateway API CRDs aren't installed |

## Admission webhooks

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/admission-webhooks` | List every ValidatingWebhookConfiguration and MutatingWebhookConfiguration, flattened to one row per webhook entry (read-only) |

## Network and namespaces

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/namespaces` | List namespaces |
| POST | `/api/v1/namespaces` | Create namespace |
| DELETE | `/api/v1/namespaces/{name}` | Delete namespace |
| GET | `/api/v1/services?namespace=...` | List Services |
| POST | `/api/v1/services` | Create a Service: `{"namespace","name","type","port","targetPort","selector":{...}}` |
| DELETE | `/api/v1/services/{namespace}/{name}` | Delete a Service |
| GET | `/api/v1/network?namespace=...` | Service→Pod topology graph |
| GET | `/api/v1/packetwolf/status` | PacketWolf connectivity |
| GET | `/api/v1/packetwolf/proxy/{path...}` | Authenticated proxy to PacketWolf API |

Set `AXIOM_PACKETWOLF_URL` (and optional `AXIOM_PACKETWOLF_API_KEY`) to enable PacketWolf.

## Virtual machines (KubeVirt)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/vms?namespace=...` | List VirtualMachines |
| POST | `/api/v1/vms` | Create a VM: `{"namespace","name","cpu","memory","image","running"}` (`cpu` is a string, e.g. `"1"`, not a JSON number) |
| DELETE | `/api/v1/vms/{namespace}/{name}` | Delete a VM |
| POST | `/api/v1/vms/{namespace}/{name}/start` | Start VM |
| POST | `/api/v1/vms/{namespace}/{name}/stop` | Stop VM |
| POST | `/api/v1/vms/{namespace}/{name}/migrate` | Live migration |
| GET | `/api/v1/vms/{namespace}/{name}/console` | virtctl / VNC hints |
| GET | `/api/v1/vms/{namespace}/{name}/vnc` | Proxied VNC WebSocket path for a browser VNC client (no embedded noVNC UI yet — this is the path an external client connects to) |
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
| POST | `/api/v1/volumes` | Create a PVC: `{"namespace","name","size","storageClass","accessMode"}` (`storageClass`/`accessMode` optional) |
| DELETE | `/api/v1/volumes/{namespace}/{name}` | Delete a PVC |
| GET | `/api/v1/persistentvolumes` | Cluster-scoped PersistentVolumes (capacity, access modes, claim, storage class) |
| GET | `/api/v1/atlas` | Atlas summary (policies, pools, volumes) |
| GET | `/api/v1/atlas/{path...}` | Atlas API proxy |
| POST | `/api/v1/atlas/volumes` | Request Atlas volume (+ optional PVC) |

## Secrets and backups

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/secrets?namespace=...` | List Secrets (keys only) |
| POST | `/api/v1/secrets` | Create Secret |
| GET | `/api/v1/secrets/{namespace}/{name}` | Secret detail |
| PUT | `/api/v1/secrets/{namespace}/{name}` | Replace a Secret's data: `{"data":{...},"type":"..."}` |
| DELETE | `/api/v1/secrets/{namespace}/{name}` | Delete Secret |
| GET | `/api/v1/external-secrets?namespace=...` | ExternalSecret CRs |
| GET | `/api/v1/backups?namespace=...` | Volume snapshots / backups |
| POST | `/api/v1/backups` | Create backup |
| POST | `/api/v1/backups/restore` | Restore PVC from snapshot |
| GET | `/api/v1/velero/backups?namespace=...` | Velero backups |
| GET | `/api/v1/velero/schedules?namespace=...` | Velero schedules |
| POST | `/api/v1/velero/schedules` | Create a schedule: `{"namespace","name","schedule","ttl"}` (`schedule` is a cron string, `ttl` is optional, e.g. `"720h0m0s"`) — builds the Velero `Schedule` object directly via the dynamic client, same as the other Velero/Flux/Argo access in this API |

## Helm, GitOps, ops

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/helm/releases` | Helm releases |
| GET | `/api/v1/helm/releases/{namespace}/{name}/history` | Release revision history |
| POST | `/api/v1/helm/releases/{namespace}/{name}/rollback` | Rollback (`{"revision": N}`) |
| POST | `/api/v1/helm/releases/{namespace}/{name}/uninstall` | Uninstall release |
| POST | `/api/v1/helm/install` | Install a chart: `{"namespace","releaseName","repoUrl","chartName","chartVersion","values"}` (`chartVersion`/`values` optional, `values` is raw YAML). Runs `helm install --repo <repoUrl>` as a background Job under Axiom's own ServiceAccount — the chart can only create resources that ServiceAccount is itself allowed to create. No chart-repo browsing in this pass; supply the repo URL and chart name directly, the same as `helm install --repo` on the CLI |
| POST | `/api/v1/helm/upgrade` | Upgrade an existing release: same body shape as install (`{"namespace","releaseName","repoUrl","chartName","chartVersion","values"}`) — runs `helm upgrade --repo <repoUrl>` under the same ServiceAccount constraints |
| GET | `/api/v1/gitops` | GitOps applications (Flux/Argo when present) |
| POST | `/api/v1/gitops/{kind}/{namespace}/{name}/sync` | Force a re-sync. `kind` is one of `Application` (Argo — patches the top-level `operation.sync` field, the same mechanism `argocd app sync` uses) or `Kustomization`\|`HelmRelease` (Flux — bumps the `reconcile.fluxcd.io/requestedAt` annotation, the same convention `flux reconcile` uses) |
| GET | `/api/v1/alerts?namespace=...` | Alertmanager / Prometheus alerts |
| GET | `/api/v1/audit` | Axiom audit log (JSON array; the Ops → Audit tab exports the filtered view as CSV or JSON) |
| GET | `/api/v1/rbac?namespace=...` | Roles and bindings |
| POST | `/api/v1/rbac/roles` | Create a namespaced Role: `{"namespace","name","rules":[{"apiGroups","resources","verbs"}]}` |
| DELETE | `/api/v1/rbac/roles/{namespace}/{name}` | Delete a Role |
| POST | `/api/v1/rbac/rolebindings` | Create a namespaced RoleBinding: `{"namespace","name","roleName","subjects":[{"kind","name","namespace"}]}` |
| DELETE | `/api/v1/rbac/rolebindings/{namespace}/{name}` | Delete a RoleBinding |
| GET | `/api/v1/cost` | Cost / utilization estimates — `namespaces` (by namespace) and `workloads` (by owning Deployment/StatefulSet/etc., derived from the same per-pod CPU/memory-request pass) |
| POST | `/api/v1/validate` | Dry-run manifest validation |

### RBAC viewer (read-only)

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/rbac/subjects` | Every user, group and ServiceAccount named by a binding (plus every ServiceAccount): `kind`, `name`, `namespace`, `bindings`, `clusterWide`, `clusterAdmin`, `roles` |
| GET | `/api/v1/rbac/subjects/{kind}/{name}` | Effective permissions of one subject: `cluster[]` and `namespaced[]` grants, each with `binding`, `roleRef`, `namespace`, resolved `rules`, and `via` when granted through an implicit group (`system:authenticated`, `system:serviceaccounts[:ns]`). `kind` is `User`\|`Group`\|`ServiceAccount` (case-insensitive); a ServiceAccount needs `?namespace=` |
| GET | `/api/v1/rbac/who-can?verb=&resource=&namespace=&group=&name=` | Every subject allowed `verb` on `resource` (`pods`, `pods/exec`, …), via which binding. `namespace` empty = any namespace; `group` empty = any API group (`core` for the core group); rules restricted by `resourceNames` match only when `name` is given |
| GET | `/api/v1/serviceaccounts?namespace=...` | ServiceAccounts with their secret and binding counts (read-only) |

These enumerate the whole access graph, so they require the **operator or admin** role: read-only viewers and namespace-restricted accounts get `403` (`/api/v1/rbac` is a restricted-sensitive prefix). Operators can read but, as before, still cannot write RBAC. Group membership asserted by an external IdP is not visible to Kubernetes, so a User/Group view shows only bindings that name that subject. A who-can query needs a complete snapshot: if Axiom cannot list every Role/binding the request fails (`403`/`5xx`) instead of answering from partial data.

RBAC write access is deliberately namespaced-only — `ClusterRole`/`ClusterRoleBinding` have no create/delete endpoint and stay read-only via `GET /api/v1/rbac`. See [Security](../README.md#security) for why.

## Security reports

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/security/reports?namespace=...` | Aggregated scanner output: `available`, `sources[]`, `vulnerabilities` / `misconfigurations` severity counts, `policy` result counts, `workloads[]` (riskiest first), `installHint` |
| GET | `/api/v1/security/reports/{namespace}/{kind}/{name}` | Drill-down for one workload: `vulnerabilities[]` (sorted by severity, de-duplicated, capped at 300), `misconfigurations[]`, `policyResults[]` (failing/warning only), `images` |

Read-only aggregation of Trivy Operator `VulnerabilityReport` / `ConfigAuditReport` (`aquasecurity.github.io`) and `wgpolicyk8s.io` `PolicyReport` / `ClusterPolicyReport` (Kyverno, Gatekeeper-compatible) objects through the dynamic client. Axiom runs no scanner. Missing CRDs are not an error: `available` is `false`, each source says why (`CRD not installed`, or an RBAC hint on `Forbidden`) and `installHint` explains what to install. Trivy reports a Deployment's scan against its current ReplicaSet; the pod-template-hash suffix is stripped so the row shows the Deployment. Scoped like other namespaced lists: namespace-restricted accounts must pass `?namespace=` for a namespace they hold (cluster-scoped `ClusterPolicyReport` results appear only in the all-namespaces view). Reports are paged (200 per page, 10,000 per source).

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
| POST | `/api/v1/integrations/slack/test` | Send a real test message to the configured Slack webhook |
| POST | `/api/v1/integrations/webhook/test` | Send a real test POST to the configured alert webhook |

`PUT /api/v1/integrations` also accepts `slackWebhookUrl`, `alertWebhookUrl`, and `notifyOnWarnings` (bool). When `notifyOnWarnings` is true and at least one webhook is set, a background dispatcher polls `GET /api/v1/alerts`-equivalent Warning events every 60s and forwards new ones to both configured channels.

## Users

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/users` | List local accounts (username, role, namespaces, createdAt — never the password hash) |
| POST | `/api/v1/users` | Create a local account: `{"username","password","role":"admin"\|"operator"\|"viewer","namespaces":[...]}` (password ≥ 8 chars; `namespaces` is ignored unless `role` is `viewer`) |
| PATCH | `/api/v1/users/{username}` | Update role, password, and/or namespaces independently: `{"role":"viewer","password":"...","namespaces":["team-a"]}` — include only the fields to change |
| DELETE | `/api/v1/users/{username}` | Delete a local account |

Three roles: `admin` (full access), `operator` (can mutate day-2 resources but not users, integrations, or the cluster registry — see [Clusters](#clusters)), and `viewer` (read-only, optionally namespace-restricted).

Local accounts are independent of the single env-configured operator account (`AXIOM_USERNAME`/`AXIOM_PASSWORD`, always admin). A `viewer` account authenticates and can call every `GET` endpoint, but any mutating request returns `403 {"error":"read-only user: this account cannot perform write actions"}`.

OIDC logins get a role too when `AXIOM_OIDC_ADMIN_GROUP`/`AXIOM_OIDC_VIEWER_GROUP` are configured (mapped from the ID token's `groups` claim, or whatever `AXIOM_OIDC_GROUPS_CLAIM` names) — unset (the default), every OIDC session is admin. SAML has no session-issuing code in this project, so it carries no role.

LDAP binds get a role the same way, via `AXIOM_LDAP_ADMIN_GROUP`/`AXIOM_LDAP_VIEWER_GROUP` matched against the `AXIOM_LDAP_GROUP_ATTR` attribute (default `memberOf`) read back off the bound user's own entry — a group value matches by exact DN or by its `cn` component, admin checked before viewer. Both unset (the default), every successful bind keeps the long-standing `operator` role. Group lookup only works for the `AXIOM_LDAP_BASE_DN`-constructed bind form (`uid=alice,<base>`); an AD-style `AXIOM_LDAP_DOMAIN` (NetBIOS or UPN) bind isn't a real DN to search on, so it always gets the default role regardless of group config.

### Namespace-restricted viewers

A `viewer` account's `namespaces` field restricts it to reading only the listed namespaces. Enforcement happens in the auth middleware, before any handler runs, against a fixed allowlist of namespaced-resource paths (workloads, pods, secrets, configmaps, resourcequotas, limitranges, ingresses, jobs, cronjobs, events, network, volumes, backups, external-secrets, vm-snapshots/clones/exports/restores, vms, alerts, agent-runs, platform-intents, resources, helm/releases, velero/backups, velero/schedules, haven/identity) — checked via the `?namespace=` query param on list endpoints, or the namespace path segment on single-resource GETs like `/api/v1/secrets/{namespace}/{name}`. Omitting the namespace, or passing `all`, is treated as a cross-namespace request and rejected with `403 {"error":"this account is restricted to specific namespaces"}`. Cluster-wide administrative surfaces (Nodes, Users, Clusters, Settings/Integrations, RBAC, Cost, Audit) are intentionally left unenforced — namespace-scoping is about workload visibility, not administrative access, which is already governed by the `admin`/`viewer` role.

## Intent placement

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/intent/parse` | Parse natural-language launch intent |
| POST | `/api/v1/intent/place` | Score and place workload from intent |

## Events

`GET /api/v1/events?namespace=...` returns cluster or namespace events sorted by time.

## Troubleshooting

### Change timeline

`GET /api/v1/timeline?namespace=&object=&since=&limit=` merges four sources into one newest-first array of `{time, source, severity, namespace, object, summary, actor}`:

| `source` | From |
|---|---|
| `event` | Kubernetes events (`Normal` and any `Warning` not already reported as an alert) |
| `alert` | Warning events as reported by `GET /alerts` (severity `warning`) |
| `audit` | Axiom's own audit entries (`actor` is the Axiom user). `pod.exec` entries omit the command line |
| `rollout` | Deployment rollout revisions ("Rollout revision N created") |

`namespace` scopes every source (cluster-wide audit actions such as `node.cordon` only appear when no namespace is given). `object` is `Kind/name` (for example `Deployment/payments-api`) and also matches the ReplicaSets and pods that workload owns. `since` is an RFC 3339 time or a lookback such as `90m`, `6h` or `7d`; `limit` defaults to 200 (max 1000). Any role may call it; namespace-restricted accounts must pass a `namespace` they own (an unscoped or `all` query is `403`).

### Cluster health check

`GET /api/v1/diagnose?namespace=` runs deterministic, rule-based analyzers (no LLM, nothing leaves the cluster) and returns `{namespace, generatedAt, status, counts, analyzers, findings, errors?}`. `status` is `healthy`, `warning` or `critical`; each finding is `{id, analyzer, severity, kind, namespace, name, object, title, message, suggestion}` with `severity` one of `critical`, `warning`, `info`. Findings are sorted most severe first.

| `analyzer` | Reports |
|---|---|
| `pod-status` | Crash-looping / `OOMKilled` containers (from the last termination state), image pull failures, container-creation errors, unschedulable pods |
| `service-endpoints` | Services with a selector but no ready endpoints (no matching pod vs. pods not Ready) |
| `pvc-pending` | PVCs stuck `Pending` (a `WaitForFirstConsumer` class is reported as `info`) |
| `ingress-backend` | Ingress rules whose backend Service is missing or does not expose the port |
| `hpa` | HPAs whose scale target is missing, that cannot scale or compute metrics, or sit at `maxReplicas` |
| `node-pressure` | Nodes that are not Ready or under memory/disk/PID pressure (only in an unscoped check) |

An analyzer that cannot run (for example RBAC) is listed in `errors` instead of failing the whole check, so a partial report is never mistaken for a clean one. Namespace-restricted accounts must pass an owned `namespace`.

### Workload logs

`GET /api/v1/workloads/{namespace}/{kind}/{name}/logs?container=&tail=100&follow=false&previous=false` streams the merged logs of every pod of a `Deployment`, `StatefulSet`, `DaemonSet`, `ReplicaSet` or `Job` as chunked `text/plain`. Each line is `[pod/container] <RFC 3339 timestamp> <message>`. Without `follow`, lines are sorted chronologically across pods; with `follow=true` they are forwarded in arrival order. `tail` applies per container (max 5000); at most 40 pod/container streams are opened. `previous=true` reads the previous terminated instance and skips containers that have none (`404` when none do). Same access as pod logs (viewers may read). Local Basic-auth sessions opening this in a browser need `?access_token=` like the other log routes.

### Ephemeral debug container

`POST /api/v1/pods/{namespace}/{name}/debug` (operator or admin; audited as `pod.debug`) adds an ephemeral container to a running pod through the `pods/ephemeralcontainers` subresource. Body (optional): `{"target":"container"}` to share that container's process namespace. The image is fixed by the server (`AXIOM_DEBUG_IMAGE`, default `busybox`) — a client-supplied `image` is rejected as an unknown field. The container runs `sleep` as non-root with all capabilities dropped (it satisfies the Pod Security `restricted` profile). Returns `201` with `{namespace, pod, container, image, target?, ready, message}`; connect a terminal to it with `GET /pods/{ns}/{name}/terminal?container=<container>`. A terminated or deleting pod is `409`; an unknown pod or target is `404`. Ephemeral containers cannot be removed from a pod.

`GET /api/v1/debug/capabilities` returns `{ephemeralContainers, debugImage, nodeShell, nodeShellNamespace?, nodeShellTimeoutSeconds?, interactive}` so the UI knows what to offer.

### Node shell

`GET /api/v1/nodes/{name}/shell?shell=/bin/sh` (**kubernetes build only**, WebSocket, same framing as the pod terminal) opens a root shell on a node. It is **off by default**: with `AXIOM_ENABLE_NODE_SHELL` unset it answers `403` with a message naming the variable. When enabled it is **admin-only** (operators, viewers and namespace-restricted accounts get `403`), audited as `node.shell`, and implemented as a short-lived privileged pod (`hostPID`, `nsenter` into the host) pinned to the node in a dedicated namespace (`AXIOM_NODE_SHELL_NAMESPACE`, default `axiom-node-shell`, created with the Pod Security `privileged` labels). The pod is deleted when the socket closes or after `AXIOM_NODE_SHELL_TIMEOUT` (default `15m`, max `1h`); leftovers from a crashed Axiom are swept on the next session. See [DEPLOYMENT.md](DEPLOYMENT.md#troubleshooting-tools-debug-containers-and-the-node-shell).

## Error shape

```json
{"error":"human-readable message"}
```

Accepted async mutations (Helm uninstall/rollback, some VM ops) return `202` with a `jobName` when the provider tracks background work.
