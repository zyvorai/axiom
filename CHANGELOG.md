# Changelog

All notable changes to Zyvor Axiom are documented here.

## [Unreleased]

### Fixed
- Demo mode (`AXIOM_MODE=demo`): ~17 mutating actions across node cordon/drain/taint, namespace create, VM lifecycle (start/stop/snapshot/clone/export/restore), rollout pause/resume/restart/undo, Helm install/uninstall/rollback, backup restore, GitOps sync, and CRD instance delete previously returned a success response without persisting the change — the paired list/get endpoint read from a static fixture or an untouched field, so the action silently reverted on the next page load. All now persist for the lifetime of the process.
- Demo mode: the workload detail page's Pods and Events tabs always showed empty, because `DemoProvider.GetResource` never populated them (unlike the real Kubernetes provider). Fixed to look up the workload's actual pods/events.
- Multi-cluster mode: `SwitchableProvider`'s active-cluster selection was a single value shared by every request — two browser tabs (or two users) switching clusters would silently stomp on each other's selection. Now scoped per browser tab via a session header (`X-Axiom-Session`, set once per tab and sent on every request), falling back to a per-username default for header-less API/script access. See `docs/ARCHITECTURE.md`.
- Real `KubernetesProvider.ListNodes` now reports node `status` as `"SchedulingDisabled"` when a node is cordoned — previously this was never set, so the Nodes page's Cordon/Uncordon button never flipped even though the underlying cordon succeeded.
- Every API error was reported to clients as a generic HTTP `500`, regardless of cause — including a real Kubernetes `403 Forbidden` from browsing a CRD outside the RBAC allowlist. Provider errors are now classified into the matching status (`403`/`404`/`409`/`400`/etc.) via `platform.HTTPStatus`, backed by real `apierrors` checks in the Kubernetes build and a message-phrasing heuristic in demo mode.

## [0.1.0] - 2026-08-29

First versioned release. Prior work was unversioned (`dev`-tagged images only).

### Added
- Tier 1 declarative CRD-view plugin architecture (`AxiomPlugin` custom resource, `deploy/k8s/plugins/`)
- Self-hosted enterprise packaging: a Helm chart (`deploy/helm/axiom/`)
- Self-observability: `GET /api/v1/metrics` (Prometheus text format) reporting Axiom's own request counts/latency and build info
- `SECURITY.md` vulnerability disclosure policy
- `docs/QUICKSTART.md` — a customer-facing install path that assumes only `helm`, `kubectl`, and a browser

### Notes
- Image tags before this release were the floating `dev` tag with no corresponding source tag — treat any `dev`-tagged image as pre-release and unsupported for upgrades.
