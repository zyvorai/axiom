# Axiom — Complete page index

Every primary navigable dashboard route.

_Generated: 2026-09-02 · 30 routes_

Regenerate: `node scripts/customer-docs/generate-page-index.mjs`

## AUTH

| Page | Route | Purpose | Guide |
|------|-------|---------|-------|
| Login | `/login` | Sign in to the Axiom console — local Basic auth, SSO (OIDC/SAML), or lab demo helper. | [Open](pages/auth/login.md) |

## CONSOLE

| Page | Route | Purpose | Guide |
|------|-------|---------|-------|
| Getting Started | `/getting-started` | Live setup checklist — integration status and first tasks after sign-in. | [Open](pages/console/getting-started.md) |
| Overview | `/overview` | Product home — hero, capacity, inventory, utilization; links into Alerts / Events / Logs. | [Open](pages/console/overview.md) |

## CLUSTER

| Page | Route | Purpose | Guide |
|------|-------|---------|-------|
| Nodes | `/nodes` | Kubernetes node inventory — Ready state, capacity, and entry into node detail. | [Open](pages/cluster/nodes.md) |
| Node detail | `/nodes/:name` | Single-node conditions, capacity, taints, and related workloads. | [Open](pages/cluster/nodes-name.md) |
| Namespaces | `/namespaces` | Namespace inventory — create and delete tenancy boundaries. Click a card for namespace detail. | [Open](pages/cluster/namespaces.md) |
| Namespace detail | `/namespaces/{name}` | Pods, workloads, and services inside one namespace. | [Open](pages/cluster/namespaces-name.md) |
| Custom Resources | `/custom-resources` | Browse CRD kinds and list custom resource instances. | [Open](pages/cluster/custom-resources.md) |

## WORKLOADS

| Page | Route | Purpose | Guide |
|------|-------|---------|-------|
| Workloads | `/workloads` | Deployments, StatefulSets, DaemonSets — scale, restart, seal, rollout, and Run workload wizard. | [Open](pages/workloads/workloads.md) |
| Workload detail | `/workloads/:namespace/:kind/:name` | Single controller — owned pods, events, labels, rollout, raw YAML. | [Open](pages/workloads/workloads-namespace-kind-name.md) |
| Jobs & CronJobs | `/jobs` | Batch Jobs and CronJobs — trigger runs, inspect status, delete. Click a name for detail. | [Open](pages/workloads/jobs.md) |
| Job detail | `/jobs/{namespace}/{name}` | Job status, completions, owned pods. | [Open](pages/workloads/jobs.md) |
| CronJob detail | `/cronjobs/{namespace}/{name}` | Schedule, suspend, trigger, recent jobs. | [Open](pages/workloads/jobs.md) |
| Pods | `/pods` | Pod inventory across namespaces — phase, restarts, entry into pod detail. | [Open](pages/workloads/pods.md) |
| Pod detail | `/pods/:namespace/:name` | Container specs, metrics, multi-tab PTY terminal, manifest Save, port Open. | [Open](pages/workloads/pods-namespace-name.md) |
| Platform Intents | `/platform-intents` | Intent-driven placement / platform requests surfaced in the console. | [Open](pages/workloads/platform-intents.md) |
| Autoscaling | `/autoscaling` | Horizontal Pod Autoscalers — create and delete. | [Open](pages/workloads/autoscaling.md) |

## NETWORKING

| Page | Route | Purpose | Guide |
|------|-------|---------|-------|
| Networking · Cilium | `/networking` | Service graph, NetworkPolicies, and live PacketWolf flows when configured. | [Open](pages/networking/networking.md) |
| Services | `/services` | Kubernetes Service inventory; click a row for ports, endpoints, and matching pods. | [Open](pages/networking/services.md) |
| Service detail | `/services/{namespace}/{name}` | Service ports, selector, endpoints, and matching pods. | [Open](pages/networking/services.md) |
| Ingresses | `/ingresses` | Ingress and Gateway API exposure inventory. | [Open](pages/networking/ingresses.md) |

## COMPUTE

| Page | Route | Purpose | Guide |
|------|-------|---------|-------|
| Virtual Machines | `/vms` | KubeVirt VM inventory and day-2 actions in the Axiom console. Click a name for detail. | [Open](pages/compute/vms.md) |
| VM detail | `/vms/{namespace}/{name}` | VM status, placement, start/stop/migrate, console. | [Open](pages/compute/vms.md) |

## STORAGE

| Page | Route | Purpose | Guide |
|------|-------|---------|-------|
| Storage | `/storage` | PVC / storage class intelligence for the connected cluster. | [Open](pages/storage/storage.md) |
| Atlas | `/atlas` | Atlas storage control-plane panel — backends, pools, volumes, buckets, policies. | [Open](pages/storage/atlas.md) |

## OBSERVE

| Page | Route | Purpose | Guide |
|------|-------|---------|-------|
| Pod Logs | `/logs` | Stream and inspect pod/container logs from the console. | [Open](pages/observe/logs.md) |
| Events | `/events` | Kubernetes warning/normal events stream for triage. | [Open](pages/observe/events.md) |
| Alerts | `/alerts` | Alert inventory and webhook-driven notifications. | [Open](pages/observe/alerts.md) |

## CONFIG

| Page | Route | Purpose | Guide |
|------|-------|---------|-------|
| ConfigMaps | `/configmaps` | ConfigMap inventory — create, view keys, delete. Click a name for detail. | [Open](pages/config/configmaps.md) |
| ConfigMap detail | `/configmaps/{namespace}/{name}` | ConfigMap keys and values. | [Open](pages/config/configmaps.md) |
| Secrets | `/secrets` | Secret inventory — create typed secrets, reveal values on detail, delete. | [Open](pages/config/secrets.md) |
| Secret detail | `/secrets/{namespace}/{name}` | Secret keys; reveal values on demand. | [Open](pages/config/secrets.md) |
| Quotas & Limits | `/quotas` | ResourceQuotas, LimitRanges, PDBs, PriorityClasses. | [Open](pages/config/quotas.md) |

## PROTECT

| Page | Route | Purpose | Guide |
|------|-------|---------|-------|
| Backups | `/backups` | Volume snapshots, Velero backups, and schedules. | [Open](pages/protect/backups.md) |

## ADMIN

| Page | Route | Purpose | Guide |
|------|-------|---------|-------|
| Ops hub | `/ops` | Day-2 ops tabs — GitOps/Helm/RBAC/Identity and related operator tools. | [Open](pages/admin/ops.md) |
| Users | `/users` | Local user accounts with admin or viewer roles. | [Open](pages/admin/users.md) |
| Settings | `/settings` | Clusters (health / activate), appearance, integrations, and console configuration. | [Open](pages/admin/settings.md) |

## Related

- [Customer docs home](README.md)
- [Page-by-page guides](pages/README.md)
