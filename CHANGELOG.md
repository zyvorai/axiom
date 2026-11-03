# Changelog

All notable changes to Zyvor Axiom are documented here.

## [Unreleased]

### Added — troubleshooting
- **Change timeline**: `GET /api/v1/timeline` merges Kubernetes events, Axiom audit entries, alert warnings and Deployment rollout revisions into one newest-first feed (`namespace`, `object`, `since`, `limit` filters; namespace-restricted accounts must name an owned namespace). New Timeline page under Observe, a "What changed" tab on Workload detail, and an "Open timeline" link from Events.
- **Cluster health check**: `GET /api/v1/diagnose?namespace=` runs deterministic analyzers (crash-loop / `OOMKilled` / image-pull from the last termination state, Services without endpoints, Pending PVCs, Ingresses with a missing backend, HPAs that cannot scale, node pressure) and returns findings with a severity and a suggested fix. Shown on Overview and on a new Health check page. Works in demo and Kubernetes mode.
- **Workload logs**: `GET /api/v1/workloads/{namespace}/{kind}/{name}/logs` merges the logs of every pod and container (prefixed `[pod/container]`); the Logs page gained a Workload mode, a regex filter, a timestamps toggle, a previous-container toggle and Download.
- **Ephemeral debug containers**: `POST /api/v1/pods/{namespace}/{name}/debug` (operator+, audited `pod.debug`) adds a non-root, capability-dropped container (`AXIOM_DEBUG_IMAGE`, default `busybox`) through the `pods/ephemeralcontainers` subresource, with a terminal attached on the Pod page. RBAC: `pods/ephemeralcontainers` `update` in both `deploy/k8s/rbac.yaml` and the Helm chart.
- **Node shell** (off by default): `AXIOM_ENABLE_NODE_SHELL=true` enables an admin-only, audited (`node.shell`) root shell on a node through a short-lived privileged pod in a dedicated namespace that is deleted on disconnect or timeout; refused for operators, viewers and namespace-restricted accounts. It conflicts with the Pod Security `restricted` profile on its own namespace (documented in `docs/DEPLOYMENT.md`).
- **Port-forward helper**: "Copy kubectl port-forward" button next to each port's Open button on the Pod page (a browser cannot open a local listener, so TCP forwarding stays a kubectl job).
### Added
- Opt-in, bring-your-own-LLM "Explain with AI" (off by default): Settings → AI assistant (OpenAI-compatible, Anthropic or Ollama endpoint, model, write-only API key stored in the integrations Secret, redaction switch, connection test); an "Explain with AI" action on the pod diagnosis banner and on Warning events opens a right-hand drawer that states where the data will be sent and shows the exact redacted payload before anything leaves the cluster. Context is limited to the caller's namespace (events, diagnosis, container status, a short log tail), never includes Secret/ConfigMap data or env values, and is scrubbed of tokens, keys, JWTs, kubeconfig text, base64 blobs and (optionally) IPs/hostnames. Read-only (no tool calls), rate limited per user, audited as `ai.explain` (target only). New `POST /api/v1/ai/explain`, `GET /api/v1/ai/status`, `POST /api/v1/integrations/ai/test`. See `docs/DEPLOYMENT.md` (AI assistant and privacy).

### Security
- Login throttling is now also per attempted account name (on top of per client IP), keyed whether or not the account exists, with an injectable clock and a bounded table that does not evict locked entries; a valid login from another account can no longer reset the counter of an account being guessed (closes the interleaved-success bypass listed in `docs/GAPS.md`)
- Single-use, 30-second WebSocket/SSE/log-stream tickets (`POST /api/v1/ws-ticket`, `?ticket=`) replace credentials in the URL; the terminal now uses them. `?access_token=`, `?axiom_session=` and `?api_key=` keep working but are deprecated (`Deprecation` header + one-time server warning)
- OIDC ID tokens are now signature-verified against the provider's JWKS (RS/PS/ES algorithms only, `kid` required to match, cached with timeouts and a re-fetch floor) and bound to the browser by a `nonce`; `alg=none`, HMAC-with-public-key, wrong key, unknown `kid`, expired and nonce-mismatch tokens are rejected
- Helm install/upgrade values are stored in a Secret owned by the Job (garbage-collected with it) instead of a permanent plaintext ConfigMap; Helm Jobs honour the Axiom namespace, `AXIOM_SERVICE_ACCOUNT` and `AXIOM_HELM_IMAGE`, get DNS-safe names of at most 63 characters, and refuse release/chart/repo/version/namespace values that start with `-`
- One-shot exec output is capped at 1 MiB, interactive terminals at 64 concurrent sessions and 1 MiB frames
- The VM VNC proxy re-reads the rotating ServiceAccount token and has a handshake timeout
- Webhook/Slack errors and logs no longer contain the (secret) webhook URL
- Registering a cluster rejects kubeconfigs that use exec/auth-provider plugins, `tokenFile`, client-cert/CA file paths, impersonation or `proxy-url`, and bounds the request body
- OIDC: all IdP calls have a 10 s timeout; the ID token's `iss`, `aud` and `exp` are validated
- Local accounts are checked before LDAP, so local users' passwords are no longer sent to the directory on every request
- Login rate limiter keys on the rightmost (proxy-appended) trusted-header address, groups IPv6 by /64 and is size-bounded
- Metrics series are capped and methods/paths sanitised; request logs quote the path (no log forging)
- Outside demo mode `/auth/providers` no longer advertises `demo / demo`, the default username, the API-server URL or other cluster names; TLS 1.2 minimum; startup warning for default credentials

### Changed
- Full frontend redesign: the app shell (top bar, sidebar) and all 37 pages rebuilt around a flat, dark apple.com visual language — plain black `#000`/`#1d1d1f` (dark) or white/`#f5f5f7` (light) surfaces, hairline borders, pill buttons and status badges, no glass panels, no ambient gradient washes, no glow rings. Dark theme is no longer GitHub-Primer-derived; light and dark now share one palette source. New `frontend/src/axiom-shell.css` carries the component vocabulary (`.bar`/`.side`/`.card`/`.pill`/`.tbl`/`.tile`/`.vm`/`.tabs`/etc); `PageHeader`/`DetailHeader`/`StatusBadge`/`GlassCard`/`AutoTable` were re-skinned once and cascade the look everywhere. Sidebar gained live workload/pod/alert counts, collapsible groups, and a mobile slide-in drawer; top bar gained an in-bar theme toggle and an alerts bell. `MetricRing.jsx` was removed (Overview's capacity band now uses meter bars). Every API call, mutation, dialog, and `data-testid` was preserved — this is a markup/styling pass, not a behavior change. See `docs/DESIGN-SYSTEM.md`.
- Login page: SSO is now a plain "Sign in with SSO"/"Sign in with SAML" text link under the password form instead of per-provider (Google/GitHub/Okta) icon buttons; added a "Remember me on this device" checkbox (persists only the username) and an inline error banner on failed sign-in.
### Docs
- GitHub Pages: full marketing landing page with real product screenshots, and the docs rendered as a browsable site (`scripts/build-site.py`)


### Fixed
- `scripts/deploy-remote.sh` reuses the existing `zyvor-axiom-auth` Secret on the target instead of resetting it to demo/demo on every redeploy
- Log follow is no longer cut off at the API timeout; every client path (in-cluster, multi-context, remote clusters) now has the same timeout and a 50 QPS / 100 burst limit
- Persisted audit ring is trimmed (oldest first) to stay under the 1 MiB ConfigMap limit instead of silently failing to save
- Helm release listing selects `owner=helm` Secrets and decodes only each release's newest revision
- Utilization no longer invents 42.8%/60% when metrics-server is absent (reports `unavailable`) or pads short histories with a decorative curve, and its history is per cluster
- Deployment "undo" is a single conflict-safe update without the spurious annotation rollout and drops `pod-template-hash`
- Alerts ask the API server for Warning events (the 200-event cap used to apply before the filter, hiding warnings behind Normal events)
- `CodeEditor.jsx`'s editable manifest tabs (Pod/Workload detail YAML) now open straight into edit mode instead of requiring an extra "Edit" click first, matching their documented behavior as editable tabs.
- `DELETE /api/v1/vms/{namespace}/{name}` was rejected with a 403 in real (non-demo) deployments — the `zyvor-axiom` ClusterRole granted `get/list/watch/update/patch/create` on `kubevirt.io` VMs but not `delete`. Found by validating the enterprise write endpoints against a real cluster (`e2e/scripts/validate-enterprise-writes.sh`) rather than only the demo provider/fake clientset unit tests; VM create/list, Service/Ingress/ResourceQuota create+list+delete, fleet cluster register/activate/unregister, and Helm install/upgrade/uninstall all round-tripped correctly against the real cluster.

### Added
- Read-only RBAC viewer: `GET /api/v1/rbac/subjects`, `/rbac/subjects/{kind}/{name}` (effective cluster-wide and namespaced permissions, including implicit groups) and `/rbac/who-can?verb=&resource=&namespace=`, plus a read-only ServiceAccount list; Ops → Access gets "By subject" and "Who can…" views. Operator/admin only (viewers and namespace-restricted accounts get 403)
- NetworkPolicy analysis: `GET /api/v1/networking/policies/{namespace}/analysis` computes each pod's ingress/egress isolation and allowed peers/ports from the policies' selectors, flags pods not covered by any policy, and is shown on the Networking page
- Security page (`/security`): aggregates Trivy Operator VulnerabilityReports / ConfigAuditReports and `wgpolicyk8s.io` PolicyReports (Kyverno, Gatekeeper-compatible) with severity counts and per-workload drill-down, and an install hint when the CRDs are absent (`GET /api/v1/security/reports`). `zyvor-axiom` ClusterRole gains read access to ServiceAccounts and those CRD groups (Helm chart and `deploy/k8s/rbac.yaml`)
- Audit log can be exported as JSON as well as CSV (Ops → Audit)
- OIDC SSO validated end-to-end: minted a public (PKCE, no client secret) `axiom` client in a real Keycloak realm, wired `AXIOM_OIDC_ISSUER`/`AXIOM_OIDC_CLIENT_ID` via `PUT /api/v1/integrations`, and confirmed a full browser login mints a real Axiom session (`GET /api/v1/session` returns `"auth":"oidc"`). See `e2e/scripts/verify-sso.js`.
- LDAP group-based role mapping: `AXIOM_LDAP_ADMIN_GROUP`/`AXIOM_LDAP_VIEWER_GROUP` (plus `AXIOM_LDAP_GROUP_ATTR`, default `memberOf`) map a bound user's directory group membership to `admin`/`viewer`, matched by full DN or `cn`. Implemented as a minimal hand-rolled LDAP Search (base-scope, present filter), extending the existing hand-rolled bind — no new dependency. Only applies to the `AXIOM_LDAP_BASE_DN`-constructed bind form; both env vars unset (the default) keeps every bind at the previous blanket `operator` role, so this is backward compatible. See `docs/GAPS.md`.

### Added
- Enterprise gap closure pass: real SAML ACS sessions, LDAP bind login, operator role, fleet cluster register/unregister from kubeconfig, HA Helm guard (`sessionSecret` required when `replicaCount > 1`), audit ring default 5000, ConfigMap/Secret update, Velero on-demand backup, VM create/delete + VNC WebSocket proxy path, PVC/Service/Ingress/Quota writes, Helm upgrade. See `docs/GAPS.md`.
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
