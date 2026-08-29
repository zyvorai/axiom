# Changelog

All notable changes to Zyvor Axiom are documented here.

## [Unreleased]

### Fixed
- Demo mode (`AXIOM_MODE=demo`): ~17 mutating actions across node cordon/drain/taint, namespace create, VM lifecycle (start/stop/snapshot/clone/export/restore), rollout pause/resume/restart/undo, Helm install/uninstall/rollback, backup restore, GitOps sync, and CRD instance delete previously returned a success response without persisting the change — the paired list/get endpoint read from a static fixture or an untouched field, so the action silently reverted on the next page load. All now persist for the lifetime of the process.
- Demo mode: the workload detail page's Pods and Events tabs always showed empty, because `DemoProvider.GetResource` never populated them (unlike the real Kubernetes provider). Fixed to look up the workload's actual pods/events.
- Multi-cluster mode: `SwitchableProvider`'s active-cluster selection was a single value shared by every request — two browser tabs (or two users) switching clusters would silently stomp on each other's selection. Now scoped per browser tab via a session header (`X-Axiom-Session`, set once per tab and sent on every request), falling back to a per-username default for header-less API/script access. See `docs/ARCHITECTURE.md`.
- Real `KubernetesProvider.ListNodes` now reports node `status` as `"SchedulingDisabled"` when a node is cordoned — previously this was never set, so the Nodes page's Cordon/Uncordon button never flipped even though the underlying cordon succeeded.
- Every API error was reported to clients as a generic HTTP `500`, regardless of cause — including a real Kubernetes `403 Forbidden` from browsing a CRD outside the RBAC allowlist. Provider errors are now classified into the matching status (`403`/`404`/`409`/`400`/etc.) via `platform.HTTPStatus`, backed by real `apierrors` checks in the Kubernetes build and a message-phrasing heuristic in demo mode.
- Demo mode: an unbounded replica count passed to the scale or launch endpoints (e.g. `{"replicas": 999999999}`) crashed the whole server — `rebuildPodsLocked` allocated a `[]Pod` slice sized to the caller-supplied value, and a multi-gigabyte allocation triggers a fatal, unrecoverable Go runtime out-of-memory error that no HTTP handler can catch or recover from. `Scale` and `Launch` now reject replica counts above 1000 with a `400`, and `rebuildPodsLocked` itself clamps to that cap as defense in depth. Found via randomized/exploratory testing.
- ConfigMaps, Secrets, and Workloads tables: a single very long name (with no whitespace to break on) blew out that table's column widths, visually pushing or hiding the other columns. Long names are now truncated with an ellipsis and shown in full via a `title` tooltip.
- Demo mode: `GetResource` (the workload/pod detail page) built a synthetic manifest for literally any name, including one that was never created or was already deleted — so a URL to a nonexistent resource silently rendered a fake detail page instead of a not-found error. It now checks the resource actually exists first.
- Demo mode: rapid-clicking "Launch" before the button disabled sent multiple create requests, and `DemoProvider.Launch` had no existence check — every duplicate silently overwrote the prior workload and returned `201`, unlike a real cluster's `409 AlreadyExists`. Launch now rejects a name that's already taken. The same gap existed in `CreateConfigMap`, `CreateSecret`, `CreateHPA`, `CreateRole`, and `CreateRoleBinding` — all fixed the same way. Also fixed nine API handlers across these endpoints (`launch`, the Intent-tab launch path, `createNamespace`, `podWindow`, `createSecret`, `createConfigMap`, `createHPA`, `createRole`, `createRoleBinding`) that still reported a flat `400` for every provider error instead of using the status-code classifier added for the fix above.
- Demo mode: deleting an already-deleted (or never-existing) workload, RBAC role, or role binding silently reported success instead of a `404`, inconsistent with every other demo-mode delete method in the codebase (Secret, Job, CronJob, HPA, NetworkPolicy, Pod, CRD instance all already checked). Fixed to match that convention.
- Demo mode: the same duplicate-name gap also existed in Velero schedule create (silent overwrite) and Backup create (append-only storage, so a duplicate name produced two identical-looking rows instead). Both now reject a name already in use.
- Demo mode: a duplicate agent-run name was the worst case of this bug — `ApproveAgentRun`/`DenyAgentRun` look a run up by name and only ever act on the first match, so every run after the first with the same name got permanently stuck at "Pending" with no way to approve or deny it. `CreateAgentRun` now rejects a name already in use.

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
