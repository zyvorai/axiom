# Zyvor Axiom API Reference

All endpoints except `GET /api/v1/health` are protected by the configured authentication middleware in this reference build. JSON mutation bodies are limited to 1 MiB and reject unknown fields.

## Session and health

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/health` | unauthenticated liveness/readiness |
| GET | `/api/v1/session` | authenticated user, role, mode and cluster |
| GET | `/api/v1/overview` | cloud health summary |

## Workloads

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/workloads?namespace=...` | list Deployments/StatefulSets |
| POST | `/api/v1/workloads` | launch a Deployment and optional Service |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/scale` | set desired replicas |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/start` | restore saved replicas |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/stop` | save replicas and scale to zero |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/restart` | trigger rollout through template annotation |
| DELETE | `/api/v1/workloads/{namespace}/{kind}/{name}` | delete controller and Axiom-owned Service |

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
| GET | `/api/v1/pods?namespace=...` | list Pods |
| GET | `/api/v1/pods/{namespace}/{name}/logs?container=&tail=200&follow=false` | read/stream logs |
| POST | `/api/v1/pods/{namespace}/{name}/exec` | run a command |
| DELETE | `/api/v1/pods/{namespace}/{name}` | terminate Pod |
| POST | `/api/v1/pods/{namespace}/{name}/evict` | request policy-aware eviction |

Exec accepts either a shell string or argv array:

```json
{"container":"app","command":"whoami"}
```

```json
{"container":"app","command":["/bin/sh","-lc","env | sort"]}
```

## Nodes

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/nodes` | list nodes |
| POST | `/api/v1/nodes/{name}/cordon` | mark unschedulable |
| POST | `/api/v1/nodes/{name}/uncordon` | mark schedulable |
| POST | `/api/v1/nodes/{name}/drain` | cordon and evict eligible Pods |

## Events

`GET /api/v1/events?namespace=...` returns cluster or namespace events sorted by the provider.

## Error shape

```json
{"error":"human-readable message"}
```
