# Zyvor Axiom API Reference

All endpoints except `GET /api/v1/health` require authentication (HTTP Basic — including an LDAP simple-bind fallback when `AXIOM_LDAP_URL` is set — an OIDC or SAML session cookie, or an API key when configured). JSON mutation bodies are limited to 1 MiB and reject unknown fields unless noted. Authenticated callers with a `viewer` role (see [Users](#users)) can call every `GET` route but get `403` on any mutating one.

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
| POST \| GET | `/api/v1/auth/saml/acs` | SAML Assertion Consumer Service — the IdP posts (or redirects) its response here; mints the same session cookie an OIDC login does. Validated against the pinned IdP cert (`AXIOM_SAML_IDP_CERT`), or accepted unsigned when `AXIOM_SAML_ALLOW_UNSIGNED=true` (lab only) |
| POST | `/api/v1/auth/logout` | Clear session cookie |

`GET /api/v1/auth/providers` returns `lab.operator_login: true` and `lab.hint: "demo / demo"` when no SSO is configured, and an `ldap` block (`enabled`/`domain`/`url`) when `AXIOM_LDAP_*` is set — LDAP has no separate login form; the same username/password fields on the local sign-in form simple-bind against `AXIOM_LDAP_URL` first, falling through to local accounts on failure.

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

`/logs`, `/exec`, `/terminal` and `/window/{port}/` are all reached through a plain browser navigation or WebSocket, neither of which can carry a custom `Authorization` header — for local Basic-auth sessions, append `?access_token=<base64 user:pass>` (see [Authentication](#authentication) below). OIDC/SAML cookie sessions need nothing extra.

## Autoscaling, disruption budgets, network policies

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/hpas?namespace=...` | List HorizontalPodAutoscalers |
| POST | `/api/v1/hpas` | Create an HPA: `{"namespace","name","targetKind","targetName","minReplicas","maxReplicas","targetCpuPercent"}` |
| DELETE | `/api/v1/hpas/{namespace}/{name}` | Delete an HPA |
| GET | `/api/v1/poddisruptionbudgets?namespace=...` | List PodDisruptionBudgets (read-only) |
| GET | `/api/v1/networkpolicies?namespace=...` | List NetworkPolicies |
| DELETE | `/api/v1/networkpolicies/{namespace}/{name}` | Delete a NetworkPolicy |

Creating a NetworkPolicy has no dedicated form endpoint — use `POST /api/v1/apply` with a NetworkPolicy manifest (see [Generic apply](#generic-apply) below).

## Generic apply

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/apply` | Server-side apply of any manifest: `{"manifest":"<yaml or json>"}` |

**kubernetes build only.** Parses the manifest, resolves its GVK to a GVR via discovery, and applies it with `types.ApplyPatchType` / `FieldManager: "zyvor-axiom"` — a real `kubectl apply`, not a dry run (see [`POST /api/v1/validate`](#helm-gitops-ops) for dry-run-only validation). In the demo build this always returns `{"valid":false,"message":"apply is not available in demo mode — requires a real cluster (build with -tags kubernetes)"}`.

Response shape:

```json
{"valid":true,"kind":"Deployment","apiVersion":"apps/v1","namespace":"apps","name":"hello-axiom","message":"applied"}
```

```json
{"valid":false,"errors":["human-readable parse or apply error"]}
```

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
| GET | `/api/v1/audit` | Axiom audit log |
| GET | `/api/v1/rbac?namespace=...` | Roles and bindings |
| POST | `/api/v1/rbac/roles` | Create a namespaced Role: `{"namespace","name","rules":[{"apiGroups","resources","verbs"}]}` |
| DELETE | `/api/v1/rbac/roles/{namespace}/{name}` | Delete a Role |
| POST | `/api/v1/rbac/rolebindings` | Create a namespaced RoleBinding: `{"namespace","name","roleName","subjects":[{"kind","name","namespace"}]}` |
| DELETE | `/api/v1/rbac/rolebindings/{namespace}/{name}` | Delete a RoleBinding |
| GET | `/api/v1/cost` | Cost / utilization estimates — `namespaces` (by namespace) and `workloads` (by owning Deployment/StatefulSet/etc., derived from the same per-pod CPU/memory-request pass) |
| POST | `/api/v1/validate` | Dry-run manifest validation |

RBAC write access is deliberately namespaced-only — `ClusterRole`/`ClusterRoleBinding` have no create/delete endpoint and stay read-only via `GET /api/v1/rbac`. See [Security](../README.md#security) for why.

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

## Error shape

```json
{"error":"human-readable message"}
```

Accepted async mutations (Helm uninstall/rollback, some VM ops) return `202` with a `jobName` when the provider tracks background work.
