# Zyvor Axiom Architecture

```text
Browser
  |  HTTP + JSON
  |  log stream / terminal actions
  v
Zyvor Axiom API (Go net/http)
  |
  +-- Auth middleware
  +-- Workload lifecycle API
  +-- Pod API
  +-- Node API
  +-- Events API
  +-- Embedded UX
  |
Provider interface
  |
  +-- DemoProvider (stdlib, deterministic local lab)
  |
  +-- KubernetesProvider (-tags kubernetes)
        |
        +-- Deployments / StatefulSets
        +-- Pods / Logs / Exec
        +-- Services
        +-- Evictions
        +-- Nodes / Events
        v
      k3s / Kubernetes API server
```

## Provider boundary

`internal/platform/provider.go` is the contract between the API and runtime. The UX never knows whether it is connected to the deterministic lab or a real cluster.

This makes local product development possible without faking HTTP endpoints in the frontend.

## Kubernetes lifecycle semantics

Axiom does not expose a misleading VM-like "power" abstraction for Kubernetes Pods.

- Workload Stop scales Deployments/StatefulSets to zero.
- The previous desired replica count is persisted and restored by Start.
- Restart modifies the Pod template annotation and lets the controller perform a rollout.
- Pod Evict uses the policy eviction subresource.
- Node Drain first cordons the node, skips DaemonSets/mirror Pods/system namespace Pods, then requests evictions.

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
