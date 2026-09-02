# Page-by-page guides

Each guide follows: Purpose → When to use it → How to get there → Operate from the console (UX) → Related pages.

Every route is also listed in the [complete page index](../PAGE_INDEX.md).

## Admin

| Page | What it covers |
|------|----------------|
| [Ops hub](admin/ops.md) | Day-2 ops tabs — GitOps/Helm/RBAC/Identity and related operator tools. |
| [Settings](admin/settings.md) | Integrations, clusters, theme, and console configuration. |
| [Users](admin/users.md) | Local user accounts with admin or viewer roles. |

## Auth

| Page | What it covers |
|------|----------------|
| [Login](auth/login.md) | Sign in to the Axiom console — local Basic auth, SSO (OIDC/SAML), or lab demo helper. |

## Cluster

| Page | What it covers |
|------|----------------|
| [Custom Resources](cluster/custom-resources.md) | Browse CRD kinds and list custom resource instances. |
| [Namespaces](cluster/namespaces.md) | Namespace inventory — create and delete tenancy boundaries. |
| [Node detail](cluster/nodes-name.md) | Single-node conditions, capacity, taints, and related workloads. |
| [Nodes](cluster/nodes.md) | Kubernetes node inventory — Ready state, capacity, and entry into node detail. |

## Compute

| Page | What it covers |
|------|----------------|
| [Virtual Machines](compute/vms.md) | KubeVirt VM inventory and day-2 actions in the Axiom console. |

## Config

| Page | What it covers |
|------|----------------|
| [ConfigMaps](config/configmaps.md) | ConfigMap inventory — create, edit keys, delete. |
| [Quotas & Limits](config/quotas.md) | ResourceQuotas, LimitRanges, PDBs, PriorityClasses. |
| [Secrets](config/secrets.md) | Secret inventory — create typed secrets, add keys, delete. |

## Console

| Page | What it covers |
|------|----------------|
| [Getting Started](console/getting-started.md) | Live setup checklist — integration status and first tasks after sign-in. |
| [Overview](console/overview.md) | Product home for the activated cluster — marketing-style hero, capacity, inventory, and utilization, with links into observe pages. |

## Networking

| Page | What it covers |
|------|----------------|
| [Ingresses](networking/ingresses.md) | Ingress and Gateway API exposure inventory. |
| [Networking · Cilium](networking/networking.md) | Service graph, NetworkPolicies, and live PacketWolf flows when configured. |
| [Services](networking/services.md) | Kubernetes Service inventory and endpoints overview. |

## Observe

| Page | What it covers |
|------|----------------|
| [Alerts](observe/alerts.md) | Alert inventory and webhook-driven notifications. |
| [Events](observe/events.md) | Kubernetes warning/normal events stream for triage. |
| [Pod Logs](observe/logs.md) | Stream and inspect pod/container logs from the console. |

## Protect

| Page | What it covers |
|------|----------------|
| [Backups](protect/backups.md) | Volume snapshots, Velero backups, and schedules. |

## Storage

| Page | What it covers |
|------|----------------|
| [Atlas](storage/atlas.md) | Atlas storage control-plane panel — backends, pools, volumes, buckets, policies. |
| [Storage](storage/storage.md) | PVC / storage class intelligence for the connected cluster. |

## Workloads

| Page | What it covers |
|------|----------------|
| [Autoscaling](workloads/autoscaling.md) | Horizontal Pod Autoscalers — create and delete. |
| [Jobs & CronJobs](workloads/jobs.md) | Batch Jobs and CronJobs — trigger runs, inspect status, delete. |
| [Platform Intents](workloads/platform-intents.md) | Intent-driven placement / platform requests surfaced in the console. |
| [Pod detail](workloads/pods-namespace-name.md) | Container specs, metrics, multi-tab PTY terminal, manifest Save, port Open. |
| [Pods](workloads/pods.md) | Pod inventory across namespaces — phase, restarts, entry into pod detail. |
| [Workload detail](workloads/workloads-namespace-kind-name.md) | Single controller — owned pods, events, labels, rollout, raw YAML. |
| [Workloads](workloads/workloads.md) | Deployments, StatefulSets, DaemonSets — scale, restart, seal, rollout, and Run workload wizard. |

---

30 guides. Regenerate: `node scripts/customer-docs/generate-guide-index.mjs`.
