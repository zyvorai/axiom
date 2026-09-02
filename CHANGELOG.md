# Changelog

All notable changes to Zyvor Axiom are documented here.

## [Unreleased]

### Added
- List → detail drill-downs for ConfigMaps, Secrets, Services, VMs, Jobs, and CronJobs (plus namespace services tab links). Command palette, Alerts, and Events deep-link to known object kinds. Breadcrumbs cover multi-segment detail paths.

### Fixed
- Pod interactive terminal: WebSocket upgrade always failed with 500 ("closed" in the UI) because the metrics `statusRecorder` wrapper did not implement `http.Hijacker`/`http.Flusher`, so gorilla/websocket could not hijack the connection. One-shot exec kept working; only the Terminal tab was broken.

### Changed
- Appearance theme toggle and cluster Healthy/activate moved from the left rail menu to **Settings**; sidebar is a thin flat rail (no card chrome).
- Dark theme detail surfaces: colorful status chips, tinted metric tiles, syntax-highlighted manifests (Pod/Workload YAML + CRD/plugin JSON), and env/label chips. Secret-looking env values are masked in the Details tab.
- Typography: apple.com **SF Pro** stack sitewide (dropped Inter); headlines use SF Pro Display; mono prefers SF Mono.
- Surfaces: dropped card/box chrome sitewide — flat canvas with hairline dividers only (light and dark), apple.com-style underline tabs.
- Appearance: dark theme is **GitHub Primer dark** (`#0d1117` / `#161b22` / `#30363d` / accent `#2f81f7`), not charcoal. Light stays Apple.com. Theme toggle restored (default dark).
- Buttons / primary accent: dropped mist-blue, indigo, and the cosmic→sky gradient. `default` and `brand` are solid blue CTAs (Apple Action Blue in light, GitHub accent blue in dark).
- Namespaces: clicking a namespace opens `/namespaces/{name}` with pods, workloads, and services for that namespace (rows drill into pod/workload/service detail).

### Changed
- Overview: apple.com-style marketing home — centered hero (no stats strip), capacity rings, oversized inventory, utilization charts, and **Stay informed** links to Alerts / Events / Logs. Alert and activity log dumps no longer appear on `/overview`; breadcrumbs are hidden on that route.
- Shell: Zyvor mark + Axiom wordmark move to the top bar (left); compact Search chip (⌘K) sits with Connect and the user menu on the right. Brand mark removed from the sidebar (cluster switcher is the first chrome there). Header height tightened to `h-12`.
- Dark theme: soft charcoal (~14% lightness) instead of near-pure black so Overview and chrome do not read as a void.
- Login: Apple Account–style product gate above the card (Zyvor mark, **Axiom**, hero **Sign in to Axiom.**) so the destination product is unambiguous; password-step CTA matches.
- `MetricRing` gains optional `layout="stack"`; `Sparkline` accepts a `height` prop for Overview’s taller charts.

### Fixed
- Login page: two hardcoded orange hex literals in the brand-mark gradient survived the zeus-os design port (they were never wired to the `i17-cosmic` token, unlike the rest of the mark) — the sign-in page kept showing the old orange mark. Retinted to match.

### Changed
- Login page: sign-in is now a two-step flow matching Apple ID's pattern — enter your username first (step 1, alongside the SSO buttons), then a second screen shows an editable identity chip ("demo · Not you?") and just the password field. The underlying auth call is unchanged; this only changes how the two values are collected.
- Login page: replaced the two-column marketing layout (brand/stats panel + animated aurora glow) with a single minimal centered card on a flat background — closer to how appleid.apple.com's actual sign-in page looks.
- Design system: replaced the "Liquid Glass" iPhone-17 palette with zeus-os's design system and WebShell sidebar+content layout — new color tokens (soft-charcoal dark theme, indigo/plasma light theme, `success`/`warning`/`info`/`danger` semantics), self-hosted Inter/JetBrains Mono variable fonts, pill-shaped buttons, and a restructured shell: a full-width sticky header, a collapsible grouped sidebar (persisted across reloads), and breadcrumbs rendered in the content area. Every page kept working unchanged since they consume shared `PageHeader`/`DetailHeader`/`GlassCard`/`Button` primitives rather than owning their own styling.

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
- Demo mode: closed out the same gap in VM snapshot/clone/export/restore create — all four accepted a caller-supplied name that collided with an existing one and silently overwrote it.
- Workloads and Pods bulk actions (Delete/Restart/Evict): both pages poll every 10s, so a checkbox selection could go stale — a selected row already gone by click time was silently dropped from the batch with no indication, so "Deleted 2 workloads" could mean 3 were selected. The toast now says so explicitly when it happens.
- User management: creating a duplicate username, or updating a role/password/namespace-restriction for a nonexistent user, reported a flat `400` instead of `409`/`404` — the same hardcoded-status-code gap as the earlier API fixes, just in `internal/api/users.go`.
- Audited every remaining always-`400` handler across `internal/api`: `approveAgentRun`, `denyAgentRun`, `triggerCronJob`, `restoreBackup`, `helmUninstall`, `helmRollback`, `helmInstall`, and `migrateVM` all now classify errors via `platform.HTTPStatus` instead of hardcoding `400`. Also found and fixed one more duplicate-name gap this turned up: `InstallHelmChart` silently overwrote a release name already in use instead of rejecting it like real Helm does.
- The Ops page's "Access" tab used the route segment `/ops/rbac`, not `/ops/access` — navigating to the URL matching its visible label silently rendered the Helm tab instead (Ops.jsx falls back to Helm for any unrecognized tab segment). Renamed the route key to `access` to match its label.

- Login rate limiting: the `Retry-After` header on a `429` was hardcoded to `60` regardless of the actual lockout, which ranges from 1 second (first offense past the grace count) up to a 5-minute cap under exponential backoff. A client honoring the header would wait far longer than necessary on a short lockout, or too briefly on a long one. Now reports the real remaining lockout time.

### Changed
- Design system: an Apple-adapted refinement of the existing "Liquid Glass" theme. Fixed the `body` font stack (it declared unlicensable "SF Pro Display"/"SF Pro Text" names that never resolved; now leads with `-apple-system`/`BlinkMacSystemFont`, the same system-font technique apple.com itself uses, so Mac/iOS/Safari render real San Francisco). Activated a full light/dark "iPhone 17" color palette that was defined in `index.css` but never wired to any component — it's now registered as Tailwind tokens and consolidated ~100 hardcoded hex literals across every page into it. Added a `brand` gradient variant to the shared `Button` component (defined once) and reserved it for the single most prominent action per page/dialog instead of every primary button carrying the same gradient. Softened card corner radii slightly (`--radius` 16px → 20px).
- UX pass inspired by apple.com and zyvor.dev:
  - **Confirmation dialogs before every destructive action.** Every delete/evict button in the app (18 call sites across 13 pages) previously fired immediately on click with no confirmation — a misclick permanently destroyed a real ConfigMap/Secret/Namespace/Workload. Added a shared `confirm()` dialog on the shadcn `AlertDialog` primitive (already installed, never used) and wired it into all of them, each naming the specific resource and consequence, with bulk actions naming the count.
  - A segmented (solid/striped/muted) progress bar — from zyvor.dev's migration-progress widget — in the workload Rollout dialog, replacing a bare stat grid with an at-a-glance available/updating/pending visualization.
  - The Run-workload wizard's Form/Intent/Validate-YAML tabs are now a numbered "01/02/03" stepper instead of a plain tab strip.
  - `PodDetail`'s "Logs" button (pure navigation, sitting next to the real "Evict" action) is now a plain text link, so the one destructive action per row reads unambiguously.
  - Workloads' empty-list message now distinguishes a genuinely empty cluster (icon + "Run your first workload" CTA) from filters just matching nothing (unchanged plain message).
  - Tactile motion polish: every button now gets a subtle press-down (`active:scale`) — previously only the Login page's SSO buttons had this. `GlassCard` now lifts slightly on hover (`.glass` already declared the transition, nothing ever triggered it). `Skeleton` loaders use a shimmer sweep instead of a flat opacity pulse.
  - Bigger, bolder page headers: a new shared `PageHeader` component (an eyebrow label above an oversized `text-4xl`/`5xl` title) generalizes the hero treatment that previously only `Overview` had, rolled out across all 26 other top-level pages in place of their old plain `text-3xl` heading. Every page's outer section spacing also grew from `space-y-4` to `space-y-6` for more breathing room, trading a little table density for a layout closer to apple.com's own product pages. Resource-detail pages (Pod/Node/Workload) got a matching `DetailHeader` component instead — same eyebrow/mono-name/meta-row identifier pattern they already used, scaled up (`text-3xl` → `text-4xl`) to match, with the same `space-y-6` spacing bump.

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
