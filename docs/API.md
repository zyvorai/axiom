# Zyvor Axiom API Reference

All endpoints except `GET /api/v1/health` require authentication (HTTP Basic — including an LDAP simple-bind fallback when `AXIOM_LDAP_URL` is set — an OIDC or SAML session cookie, or an API key when configured). JSON mutation bodies are limited to 1 MiB and reject unknown fields unless noted. Authenticated callers with a `viewer` role (see [Users](#users)) can call every `GET` route but get `403` on any mutating one.

## Errors and validation

Errors are JSON, `{"error": "message"}`. Status classes: `400` invalid input, `401` / `403` not authenticated / not allowed, `404` not found, `409` already exists or nothing to do (e.g. rollback / undo with no previous revision), `429` blocked by a PodDisruptionBudget (eviction, drain), `5xx` a real server or upstream failure. Both the demo and Kubernetes builds classify the same way (see [ARCHITECTURE.md](ARCHITECTURE.md#http-status-mapping)). Notable validation rules:

- Workload `name` must be an RFC 1123 label (lowercase letters, digits, `-`; max 63); replicas must be an integer >= 0.
- `POST /hpas`: `maxReplicas` below `minReplicas`, above 1000, a CPU/memory target outside 1-100, `scaleDownStabilizationSeconds` outside 0-3600, a `targetKind` other than `Deployment`/`StatefulSet`/`ReplicaSet`, or a name/namespace that is not DNS-1123 is a `400` (an omitted max defaults to min; no CPU and no memory target defaults to 80% CPU).
- `POST /secrets` needs at least one `data` / `keys` entry.
- `POST /users`: `username` must not contain `:` or control characters (HTTP Basic cannot carry them); password at least 8 characters.
- `POST /apply` and `POST /validate` take exactly one object — a multi-document (`---`) manifest is reported invalid.
- Sizes and quantities (`size`, `cpu`, `memory`, quota `hard`) that do not parse are `400`.

## Session and health

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/health` | Unauthenticated liveness/readiness |
| GET | `/api/v1/session` | Authenticated user, role, mode, cluster, and `mfa: {required, enrolled}` (see the require-MFA policy below) |
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
| GET | `/api/v1/fleet` | Every registered cluster with its latest health probe and a roll-up (see below). Any role except namespace-restricted accounts (`403`: it is cluster-wide). Never cached |
| POST | `/api/v1/fleet/refresh` | Probe every cluster now and return the fresh fleet. Reads from the clusters only, so read-only viewers may use it; rate-limited to one probe round every 3 s |
| GET | `/api/v1/fleet/history?cluster=&window=24h` | Uptime, incidents and a health/latency/utilisation series per cluster from the in-memory probe history (see below). Same access as `/fleet` (namespace-restricted accounts get `403`); `400` for a bad `window` or `cluster`, `404` for an unknown cluster. Never cached |
| GET | `/api/v1/fleet/search?q=&kind=&limit=` | Search workloads, services and namespaces across every registered cluster (see below). Same access as `/fleet` (namespace-restricted accounts get `403`); `400` for a bad `q`, `kind` or `limit`; `429` with `Retry-After` beyond 20 searches per user per 10 s. Never cached |

**Fleet health.** Axiom probes every registered cluster in the background (every `AXIOM_FLEET_PROBE_INTERVAL`, default 30 s, up to 8 clusters at once, 8 s timeout each so one dead cluster never stalls the rest) by asking it for its overview — "reachable" means the API server answers, not merely that a kubeconfig exists. Each cluster is `healthy`, `degraded` (reachable, but some nodes are not ready or the cluster reports an unhealthy status; `reason` says which — warning events alone do not degrade a cluster), `unreachable` (the probe failed or timed out; `error` carries the message. A cluster that was reachable needs two consecutive failed probes before it is declared unreachable, so one slow response is not an outage; a cluster's very first probe reports immediately) or `unknown` (not probed yet; a read of `/fleet` probes such clusters first). A cluster entry has `name`, `label`, `current`, `server`, `state`, `reason`, `error`, `checkedAt`, `since` (when the state last changed), `latencyMs` and a headline summary (`nodes`, `readyNodes`, `pods`, `runningPods`, `workloads`, `vms`, `warnings`, `cpuPercent`, `memoryPercent`); `summary` rolls them up, counting resources only for clusters that answered. `GET /api/v1/clusters` carries `state`, `latencyMs` and `error` too, and its `healthy` flag is now false only for an unreachable cluster. Changes of state are written to the audit log (`fleet.cluster_unreachable`, `fleet.cluster_degraded`, `fleet.cluster_recovered`) and, when notifications are on (`notifyOnWarnings` plus a Slack or alert webhook), announced there too — see *Notifications* under Integrations. A cluster registered from Settings is probed immediately.

**Fleet history.** Every probe (background, "Check now", or the lazy probe of a never-probed cluster) appends one sample per cluster **after** the two-failure rule, so the history shows the state the console showed: `{"t" (unix seconds),"state","latencyMs","readyNodes","nodes","runningPods","pods","warnings","cpuPercent","memoryPercent"}`. Samples are kept for 24 h, at most 3,000 per cluster, in memory only (lost on restart; each replica keeps its own), and dropped when a cluster is unregistered. `GET /api/v1/fleet/history` takes `window` `1h`, `6h` or `24h` (default) and an optional `cluster` (omitted = every cluster, in registration order; 1–253 characters, no control characters) and returns `{"window","intervalSeconds","clusters":[{"cluster","label","samples":[...],"uptimePercent","checks","failedChecks","currentState","since","incidents":[{"state","from","to","durationSeconds","reason"}]}]}`. `samples` has at most 240 points: when the window holds more, it is split into 240 equal time buckets and each non-empty bucket becomes one point with the worst state in it (`unreachable` > `degraded` > `healthy`), the average latency, CPU and memory, the last counts and the time of its last sample. `uptimePercent` is the share of time the cluster was reachable (`healthy` or `degraded`), each sample weighted by the gap to the next one capped at 2 × the probe interval (the newest counts one interval), two decimals, `null` with no samples. `checks` counts the samples in the window; `failedChecks` the probes that failed, including a single failure the two-failure rule absorbed (that sample still shows the held state). `incidents` (oldest first) are runs of non-healthy samples: `state` is the worst state of the run, `from` its first sample, `to` the first healthy sample after it (`null` while ongoing; `durationSeconds` then runs to now), `reason` the latest degraded reason / probe error of that state; runs separated by less than 2 probe intervals of health are merged. `currentState` / `since` are the latest probe's. Demo mode records the three demo clusters like any others; `AXIOM_DEMO_FLEET_HISTORY_SEED=true` backfills 24 h of samples at 30 s with one 20-minute incident about 3 h ago (unreachable on the first `AXIOM_DEMO_FLEET_DOWN` cluster if set, else degraded on `ap-south-1`).

**Fleet search.** `GET /api/v1/fleet/search?q=pay` answers "where is `payments` running?". `q` is required (2–64 characters after trimming) and is matched case-insensitively as a substring of the name and namespace, and for workloads also the image. `kind` narrows to `Workload`, `Service` or `Namespace` (comma-separated, case-insensitive; default all three); `limit` is 1–200 (default 50). Every registered cluster is searched through its own provider (never the caller's active cluster), up to 8 at once with a 5 s timeout each. Clusters whose last probe was `unreachable` are not waited on but listed under `clusters.skipped` (`{"cluster","reason":"unreachable"}`); never-probed clusters are probed first. A cluster that fails or times out is listed under `clusters.errors` (`{"cluster","error"}`, single-line, URL credentials stripped) and never fails the call; `clusters.searched` counts the clusters asked. Each result has `cluster`, `clusterLabel`, `clusterState`, `kind`, `workloadKind` (workloads only), `namespace`, `name`, `status` and a one-line `detail` (e.g. `12/12 ready · ghcr.io/zyvorai/payments:v2.8.4`). Results are ordered exact name match, name prefix, name contains, then namespace/image matches, and within each by cluster registration order, namespace and name; `total` counts every match and `truncated` is true when `limit` cut the list. Without a multi-cluster registry the answer is an empty result with `searched: 0`.

Register/unregister (and Settings → Clusters, which drives them) are restricted to `admin` — see [Users](#users)'s operator-role note. The backend `Provider` is always a `SwitchableProvider` delegating to whichever cluster is active *for the caller*, not one global switch — see `docs/ARCHITECTURE.md`'s Multi-cluster section. The frontend sends an `X-Axiom-Session` header (one random ID per browser tab) so switching clusters in one tab never affects another tab or another user; a request sent with no such header (e.g. a scripted `curl`) is scoped per authenticated username instead. A normal in-cluster (ServiceAccount) deployment registers exactly one cluster — this is not a stub, it's the honest single-cluster case. A kubeconfig with multiple contexts registers one provider per context. Demo mode registers three independent demo clusters (set `AXIOM_DEMO_FLEET_DOWN=ap-south-1` and/or `AXIOM_DEMO_FLEET_DEGRADED=us-east-1` to simulate an outage for testing).

## Authentication

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/auth/providers` | OIDC, SAML, LDAP, lab operator login flags |
| GET | `/api/v1/auth/oidc/login` | Start OIDC authorize redirect |
| GET | `/api/v1/auth/oidc/callback` | OIDC callback (IdP redirect target) |
| GET | `/api/v1/auth/saml/login` | Redirect to the SAML IdP (`AXIOM_SAML_LOGIN_URL`) with an AuthnRequest (HTTP-Redirect binding, `SAMLRequest` query parameter; `?next=` becomes `RelayState`). Its random ID is recorded in a signed, HttpOnly, 5-minute `axiom_saml_req` cookie (`SameSite=None; Secure` over HTTPS so it survives the IdP's cross-site POST, `Lax` over plain HTTP) and in a single-use in-memory set on this replica |
| POST | `/api/v1/auth/saml/acs` | SAML Assertion Consumer Service (HTTP-POST binding only) — verifies the response's XML-DSig signature against `AXIOM_SAML_IDP_CERT`, checks audience and validity window, requires every `SubjectConfirmation` to be bearer with `SubjectConfirmationData` `Recipient` equal to this ACS URL and `NotOnOrAfter` not passed, and requires the signed `InResponseTo` to equal the ID in the `axiom_saml_req` cookie and still be outstanding (it is consumed, so a response works once). The cookie is cleared on every attempt; a missing, mismatched, expired, unknown or replayed request answers `401` and no session is issued. Responses with no `InResponseTo` (IdP-initiated) get `401` unless `AXIOM_SAML_ALLOW_IDP_INITIATED=true`. On success it mints the same session cookie an OIDC login does. Lab-only `AXIOM_SAML_ALLOW_UNSIGNED=true` skips signature verification (the request and SubjectConfirmation checks still apply). |
| POST | `/api/v1/auth/login` | Sign in and receive a signed session: `{"username","password","code"?}` → `{"ok","user","role","mfa","token":"axiom.…","expiresIn"}` plus an `axiom_session` cookie. Use `token` as `Authorization: Bearer axiom.…` from scripts. This is the only way an account with two-factor sign-in can authenticate (see below); other accounts may keep using HTTP Basic. Throttled like every credential check |
| POST | `/api/v1/auth/logout` | End *this* session (it is revoked server-side, not just cleared from the browser) and clear the cookie |
| POST | `/api/v1/auth/logout-all` | Sign the caller out everywhere: every session the account has open is revoked. Any role |
| GET | `/api/v1/auth/mfa` | The caller's two-factor state: `{"available","enabled","recoveryCodes","managed"?}`. `available` is false (with `managed` naming where it lives) for SSO/LDAP accounts and for the built-in admin account |
| POST | `/api/v1/auth/mfa/setup` | Start enrolment: `{"password"}` → `{"secret","uri","issuer"}`. The password must be the caller's current one, so a stolen session cannot enrol an attacker's device. Nothing changes until `enable` confirms a code. Any role |
| POST | `/api/v1/auth/mfa/enable` | Confirm the secret with a code from the authenticator: `{"code"}` → `{"ok","recoveryCodes":[…10 codes…]}`. The recovery codes are shown **once**. The account's existing sessions are revoked. Any role |
| POST | `/api/v1/auth/mfa/disable` | Turn it off: `{"password","code"}` (a TOTP or an unused recovery code). Revokes the account's sessions. Any role |

**Two-factor sign-in (TOTP).** Local accounts can add a time-based one-time code (RFC 6238: HMAC-SHA1, 6 digits, 30 s, one step of clock drift accepted). Once enabled, the account **cannot authenticate with Basic credentials any more**: a request with the right password but no session gets `401` with `X-Axiom-MFA: required`, and the client signs in through `POST /auth/login` with the code, which returns a session. A TOTP code is accepted once per time step (a replay is refused), and each of the ten recovery codes works once. Wrong codes count toward the same per-IP and per-account throttling as wrong passwords; a right password with a missing code does not. Sign-ins and MFA changes are audited (`auth.login`, `auth.mfa_failed`, `auth.mfa_setup`, `auth.mfa_enabled`, `auth.mfa_disabled`, `user.mfa_reset`). The built-in admin account can be given a second factor with `AXIOM_TOTP_SECRET` (base32, ≥ 16 characters); API keys, LDAP and SSO accounts are not affected (use your IdP's MFA for SSO). The secret and the hashed recovery codes are stored with the account in the `zyvor-axiom-users` Secret and never returned by any endpoint.

**Require-MFA policy.** `AXIOM_REQUIRE_MFA` = `off` (default) | `admin` | `operator` (operators and admins) | `all` (every local account, viewers included) makes a second factor mandatory for local accounts in those roles. A covered account that has not enrolled can still authenticate (Basic or `POST /auth/login` without a code) so that it can enrol, but every request except `GET /session`, `GET /auth/providers`, `POST /auth/logout`, `POST /auth/logout-all`, `GET /auth/mfa` and `POST /auth/mfa/setup|enable|disable` (plus the public health and metrics) is refused by the guard with:

```json
{"error":"two-factor sign-in is required for your role: set it up under Account security","mfaEnrollmentRequired":true}
```

(`403`). Disabling MFA is allowed, but a covered account that does so is blocked again from its next request. SSO and LDAP accounts and API keys are never affected. The built-in admin is break-glass: without `AXIOM_TOTP_SECRET` it is exempt (logged at start-up); with it, it signs in with a code and satisfies the policy. `GET /session` reports the state as `"mfa": {"required": bool, "enrolled": bool}` — `required` is true when the policy covers this identity (always false for SSO, LDAP and API-key identities, and for the exempt built-in admin), `enrolled` when it has a second factor — so the UI can force enrolment.

**Sessions and revocation.** Sessions are signed tokens that last `AXIOM_SESSION_TTL` (default 12 h, minimum 1 minute, maximum 30 days). Each has an ID, so logout revokes exactly that session, and `logout-all` revokes every session a user issued before now. Axiom also revokes a user's sessions when an administrator changes their role, namespaces or password, deletes them, or when their MFA is enabled, disabled or reset. Sessions from the local sign-in are re-resolved against the live account on every request, so a role or namespace change and a deleted account apply immediately. Revocations are kept in the `zyvor-axiom-revocations` ConfigMap (survive restarts). Every replica merges its changes into the ConfigMap (a union, with resourceVersion-checked updates so concurrent revocations are all kept) and re-reads it every `AXIOM_REVOCATION_REFRESH` (default 15 s), so a revocation made on one replica applies on the others within that interval; it applies at once on the replica that handled it.

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
| GET | `/api/v1/hpas?namespace=...` | List HorizontalPodAutoscalers (read via `autoscaling/v2`): `namespace`, `name`, `targetKind`, `targetName`, `minReplicas`, `maxReplicas`, `currentReplicas`, `desiredReplicas`, `metrics[]` (`type` `Resource`\|`ContainerResource`\|`Pods`\|`Object`\|`External`, `name` `cpu`\|`memory`\|metric name, `targetType` `Utilization`\|`AverageValue`\|`Value`, rendered `target` such as `60%`/`500Mi`/`100`, and `current`, empty until the HPA reports it), `conditions[]` (`type`, `status`, `reason`, `message`: `AbleToScale`, `ScalingActive`, `ScalingLimited`), `lastScaleTime` (RFC 3339 or `""`), `behavior` (true when scale-up/down behavior is set) and `age`. `targetCpuPercent`/`currentCpuPercent` are still filled when a CPU Utilization metric exists |
| POST | `/api/v1/hpas` | Create an HPA: `{"namespace","name","targetKind","targetName","minReplicas","maxReplicas","targetCpuPercent","targetMemoryPercent","scaleDownStabilizationSeconds"}`. One `Resource` Utilization metric per CPU/memory target given; `scaleDownStabilizationSeconds` > 0 sets `behavior.scaleDown.stabilizationWindowSeconds` (0 keeps the Kubernetes default) |
| DELETE | `/api/v1/hpas/{namespace}/{name}` | Delete an HPA |
| GET | `/api/v1/poddisruptionbudgets?namespace=...` | List PodDisruptionBudgets (read-only) |
| GET | `/api/v1/networkpolicies?namespace=...` | List NetworkPolicies |
| DELETE | `/api/v1/networkpolicies/{namespace}/{name}` | Delete a NetworkPolicy |

### VPA and KEDA

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/vpas?namespace=...` | VerticalPodAutoscalers (`autoscaling.k8s.io` `v1`, falling back to `v1beta2`): `{"installed","items":[{"namespace","name","targetKind","targetName","updateMode","containers":[{"name","lowerBound","target","upperBound","uncappedTarget"}],"conditions","age"}]}`. Each bound is `{"cpu","memory"}` as Kubernetes quantity strings (`250m`, `512Mi`); `updateMode` is `Off`\|`Initial`\|`Recreate`\|`Auto` (`Auto` when unset, as in the VPA API) |
| GET | `/api/v1/scaledobjects?namespace=...` | KEDA ScaledObjects (`keda.sh/v1alpha1`): `{"installed","items":[{"namespace","name","targetKind","targetName","minReplicas","maxReplicas","currentReplicas","triggers":[{"type","name"}],"ready","active","fallback","conditions","age"}]}`. `currentReplicas` comes from the HPA KEDA manages (`status.hpaName`); `minReplicas`/`maxReplicas` default to 0/100 like KEDA |

Both are read-only. When the CRD is not installed the answer is `200` with `{"installed":false,"items":[]}`, not an error; any other failure (for example Axiom's ServiceAccount lacking `list`) is an error. A trigger's `name` is its own `name`, else its `metricName`/`queueName`/`topic`/`listName`/`streamName`/`subscriptionName` metadata value, else `""`; **no other trigger metadata and no `authenticationRef` is ever returned** (they can carry endpoints and credentials). Both routes are namespaced reads like `/api/v1/hpas`: a namespace-restricted account must pass `?namespace=` with one of its namespaces (`all` or no namespace is `403`).

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
| POST | `/api/v1/helm/install` | Install a chart: `{"namespace","releaseName","repoUrl","chartName","chartVersion","values"}` (`chartVersion`/`values` optional, `values` is raw YAML). Runs `helm install --repo <repoUrl>` as a background Job under Axiom's own ServiceAccount — the chart can only create resources that ServiceAccount is itself allowed to create. Install still takes the repo URL and chart name directly, the same as `helm install --repo` on the CLI; the [chart catalog](#helm-chart-catalog) below is how the UI finds them |
| POST | `/api/v1/helm/upgrade` | Upgrade an existing release: same body shape as install (`{"namespace","releaseName","repoUrl","chartName","chartVersion","values"}`) — runs `helm upgrade --repo <repoUrl>` under the same ServiceAccount constraints |
| GET | `/api/v1/gitops` | GitOps applications (Flux/Argo when present) |
| POST | `/api/v1/gitops/preview` | Render the objects a create would make, without touching the cluster — see [GitOps application creation](#gitops-application-creation-and-deletion) |
| POST | `/api/v1/gitops` | Create an Argo CD / Flux application (admin only) — see [GitOps application creation](#gitops-application-creation-and-deletion) |
| GET | `/api/v1/gitops/{kind}/{namespace}/{name}/delete-preview` | What deleting the application would do (cascade or not) — see [GitOps application creation and deletion](#gitops-application-creation-and-deletion) |
| DELETE | `/api/v1/gitops/{kind}/{namespace}/{name}?removeSource=&confirm=` | Delete an Argo CD / Flux application (admin only; `confirm=<name>` required when it cascades) — see [GitOps application creation and deletion](#gitops-application-creation-and-deletion) |
| POST | `/api/v1/gitops/{kind}/{namespace}/{name}/sync` | Force a re-sync. `kind` is one of `Application` (Argo — patches the top-level `operation.sync` field, the same mechanism `argocd app sync` uses) or `Kustomization`\|`HelmRelease` (Flux — bumps the `reconcile.fluxcd.io/requestedAt` annotation, the same convention `flux reconcile` uses) |
| GET | `/api/v1/alerts?namespace=...` | Alertmanager / Prometheus alerts |
| GET | `/api/v1/audit` | Axiom audit log (JSON array; the Ops → Audit tab exports the filtered view as CSV or JSON) |
| GET | `/api/v1/rbac?namespace=...` | Roles and bindings |
| POST | `/api/v1/rbac/roles` | Create a namespaced Role: `{"namespace","name","rules":[{"apiGroups","resources","verbs"}]}` |
| DELETE | `/api/v1/rbac/roles/{namespace}/{name}` | Delete a Role |
| POST | `/api/v1/rbac/rolebindings` | Create a namespaced RoleBinding: `{"namespace","name","roleName","subjects":[{"kind","name","namespace"}]}` |
| DELETE | `/api/v1/rbac/rolebindings/{namespace}/{name}` | Delete a RoleBinding |
| GET | `/api/v1/cost?window=30d` | Monthly cost — `namespaces` (by namespace) and `workloads` (by owning Deployment/StatefulSet/etc.), plus `totalMonthlyUsd`, `cpuRatePerCore`, `memoryRatePerGi`, `source` (`estimate` or `opencost`), `window`, `idleMonthlyUsd`, `unallocatedMonthlyUsd` (OpenCost only, omitted when 0) and `note`. `window` is `7d`, `14d` or `30d` (default); anything else is `400`, also when it has no effect. See *Actual cost from OpenCost* below |
| POST | `/api/v1/validate` | Dry-run manifest validation |

**Actual cost from OpenCost.** Without OpenCost, `source` is `estimate`: each row prices the pods' CPU/memory **requests** at the flat rates (`cpuCores` × `cpuRatePerCore` + `memoryGi` × `memoryRatePerGi`), `window` is echoed but has no effect, and `idleMonthlyUsd` is 0. With `AXIOM_OPENCOST_URL` set (see DEPLOYMENT.md), Axiom reads `GET {url}/allocation/compute?window=<window>&aggregate=namespace,controllerKind,controller&accumulate=true` and `source` is `opencost`: `monthlyUsd` on every row is the **actual** allocation cost over the window scaled to a 30-day month (totalCost × 43,200 / minutes covered, where minutes covered is the longest allocation's `minutes` — or its `start`/`end` span — capped at the window, so a cluster with only 2 days of OpenCost data is extrapolated from those 2 days and `note` says so). Rows also carry a breakdown, `cpuUsd`, `memoryUsd`, `storageUsd` (PV), `networkUsd` and `otherUsd` (GPU, load balancer, shared, external and anything else in totalCost), each including OpenCost's adjustments; `cpuCores` / `memoryGi` stay the current requests so the table keeps its columns. OpenCost's `__idle__` bucket is reported as `idleMonthlyUsd` and `__unmounted__` / `__unallocated__` (no namespace) as `unallocatedMonthlyUsd`; neither is part of `totalMonthlyUsd` (the sum of the namespace rows) or any namespace. Pods without a controller are one workload row per namespace, `(pods without a controller)`, kind `Pod`; a namespace with requests but no OpenCost cost is listed at 0. Unknown fields are ignored and missing numbers count as 0. One answer per window is cached for 60 s (a failure for 15 s) and concurrent requests share one fetch. If OpenCost is unreachable, slow (10 s), answers non-200 / a non-200 `code` / malformed or empty data, the call still succeeds with the estimate (`source: "estimate"`) and `note` says why; the OpenCost URL and token never appear in a response. OpenCost applies to the cluster Axiom started on only; clusters registered later show the estimate. Demo mode serves the estimate unless `AXIOM_DEMO_OPENCOST=true`, which makes the demo clusters return fixed OpenCost-shaped actuals (different from the estimate, with an idle bucket).

### Cost budgets and rightsizing

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/cost/budgets` | `{"budgets":[{"namespace","monthlyUsd","warnPercent","updatedAt","updatedBy","currentUsd","percent","state"}],"unbudgeted":[{"namespace","currentUsd"}],"totalBudgetUsd","totalCurrentUsd"}`. `currentUsd` comes from the same source as `GET /api/v1/cost` over 30 days — actual cost when OpenCost is configured and answering, else the estimate — and `source` (`estimate` / `opencost`) says which; `percent` = currentUsd / monthlyUsd × 100, one decimal; `state` is `over` at ≥ 100, `warning` at ≥ `warnPercent`, else `ok`. A budget for a namespace that no longer exists shows `currentUsd` 0. Budgets are sorted fullest first; `unbudgeted` lists estimated namespaces with no budget, biggest first, at most 20. `totalCurrentUsd` sums the budgeted namespaces only |
| PUT | `/api/v1/cost/budgets/{namespace}` | Set (create or replace) a budget: `{"monthlyUsd": >0 and ≤ 1e9, "warnPercent": 1..100 (default 80)}` → `200` with the budget row. **Admin only.** `{namespace}` must be a DNS-1123 label; unknown fields are refused (`400`); at most 1,000 budgets. Audited as `cost.budget_set` |
| DELETE | `/api/v1/cost/budgets/{namespace}` | Remove a budget → `{"ok":true,"namespace"}`; `404` if none. **Admin only.** Audited as `cost.budget_removed` |
| GET | `/api/v1/cost/rightsizing?namespace=&window=7d` | Request recommendations per Deployment / StatefulSet / DaemonSet container: `{"source","window","confidence","note","summary":{"workloads","overProvisioned","underProvisioned","monthlySavingsUsd"},"items":[{"namespace","workload","kind","container","replicas","cpu":{"requestMilli","recommendedMilli","usageMilli","basis"},"memory":{"requestMiB","recommendedMiB","usageMiB","basis"},"verdict","monthlyDeltaUsd","reason"}]}`. `namespace` empty = all (otherwise a DNS-1123 label); `window` is `1d`, `3d`, `7d` (default), `14d` or `30d`, anything else is `400` |
| POST | `/api/v1/cost/rightsizing/apply` | Apply one container's recommendation: `{"namespace","kind":"Deployment\|StatefulSet\|DaemonSet","workload","container","resources":["cpu","memory"],"window":"7d","dryRun":true,"observed":{"cpuRequestMilli","memoryRequestMiB"}}` → `{"ok":true,"dryRun","changes":[{"resource","field":"requests\|limits","from","to","note"}],"rollout","verdict","source","confidence"}`. **Operator or admin**; audited as `cost.rightsizing_apply` (not on a dry run). `400` bad input, `404` unknown workload/container, `409` already right-sized / stale `observed` / no usage data, `422` the API server rejected the patch |

Reads are open to every role except namespace-restricted accounts (`403`: `/api/v1/cost/budgets` and `/api/v1/cost/rightsizing` are restricted-sensitive prefixes — cluster-wide cost data; `GET /api/v1/cost` keeps its existing behaviour). Budget writes are admin-only, so operators and viewers get `403`. Responses carry `Cache-Control: no-store`. Budgets live in ConfigMap `zyvor-axiom-cost-budgets` (key `budgets.json`) in the kubernetes build and in memory in demo mode. Over-budget state is computed whenever the budgets are read, and also by a background budget watcher (every `AXIOM_BUDGET_CHECK_INTERVAL`, default 5 min, against the default active cluster) that audits crossings as `cost.budget_warning` (ok → warning), `cost.budget_over` (→ over) and `cost.budget_ok` (back to ok), user `budget-watcher`, detail `$X of $Y (Z%)`, and sends a notification for each (see *Notifications*). Over → warning is remembered but not announced.

**Applying a recommendation.** `POST /api/v1/cost/rightsizing/apply` never takes target numbers from the client: the server recomputes the recommendation for exactly that container with the same pipeline and `window` as the GET (default `7d`), then patches the workload's pod template (strategic merge, field manager `zyvor-axiom`) so the controller rolls its pods as usual (`rollout` says what to expect, e.g. a paused Deployment or an `OnDelete` update strategy). Only `resources.requests` of the chosen resources of that container change; other containers and fields are untouched. A limit is never lowered and never added: when an existing limit is below the new request it is raised to the request and reported as a `limits` change with `"note":"limit raised to stay >= the request"`. A missing request is set (`from` is `"none"`). `dryRun:true` returns the same `changes` and writes nothing — the UI always dry-runs first. `observed` carries the requests the caller saw (the GET's `cpu.requestMilli` / `memory.requestMiB`, both optional); if either differs from the live value the call is refused with `409` "the live requests changed since you looked". A container whose verdict is `ok`, or whose chosen resources already match, is `409` "already right-sized". Raising requests is allowed; ResourceQuota is enforced when the controller creates the new pods, so a quota refusal at patch time surfaces as `409` with the API server's message. Viewers (even for a dry run, which is a POST) and namespace-restricted accounts get `403`. In demo mode the apply is kept in memory, so the next GET reports the container as `ok`.

**Rightsizing sources.** One source is chosen per call and reported in `source`: `prometheus` when `AXIOM_PROMETHEUS_URL` is set — p95 CPU (`quantile_over_time(0.95, rate(container_cpu_usage_seconds_total[5m])[<window>:5m])`) and p99 working-set memory (`quantile_over_time(0.99, container_memory_working_set_bytes[<window>])`) per namespace/pod/container, two instant queries per call, `basis` `p95`/`p99`, `confidence` `high` for windows of 7 days or more, else `medium`; otherwise `metrics-snapshot`, one metrics.k8s.io sample (`basis` `current`, `confidence` `low`: one sample is not a profile); `none` when neither answers (empty `items`; `note` explains how to enable Prometheus). A Prometheus failure (unreachable, timeout, non-200, malformed or empty answer) never fails the call: `note` says why and the snapshot is used; the Prometheus URL and token never appear in a response. Pod samples map to workloads by owner reference for running pods and by the standard pod-name suffix rules for pods that are gone; a container's usage is the highest across its replicas, and its requests come from the newest running pod. Prometheus applies to the cluster Axiom started on only; clusters registered later use the snapshot. Demo mode serves fixed `metrics-snapshot` data.

**Rules.** Recommended CPU = usage × 1.2, rounded up to 10m (min 10m); memory = usage × 1.15, rounded up to 8 MiB (min 16 MiB). `verdict` is `under` when usage exceeds 90 % of the request for either resource (throttling / OOM risk), or when a request is missing ("no request set"); `over` when a request is more than 1.5× its recommendation and the gap is at least 50m CPU or 64 MiB memory; otherwise `ok`. `monthlyDeltaUsd` = (recommended − request) priced with the `GET /api/v1/cost` request rates (also when OpenCost is configured: a saving prices a change of requests, not a bill), times `replicas` (negative = savings); `summary.monthlySavingsUsd` sums the savings of the `over` rows, and `overProvisioned`/`underProvisioned` count container rows. Items are sorted by savings (most first), then namespace / workload, and capped at 500 (the summary covers every row). Recommendations are never applied.

### GitOps application creation and deletion

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/gitops/preview` | Validate and render: `200 {"objects":[{"apiVersion","kind","namespace","name"}],"yaml":"<documents separated by ---, in creation order>","warnings":[...]}`. Pure — works even when the engine is not installed |
| POST | `/api/v1/gitops` | Create the objects in order (source first): `201 {"ok":true,"created":[{"apiVersion","kind","namespace","name"}],"warnings":[...]}` |
| GET | `/api/v1/gitops/{kind}/{namespace}/{name}/delete-preview` | `200 {"kind","namespace","name","engine":"argocd\|flux","cascade","consequence","detail":[...],"source":{"kind","namespace","name","sharedWith":[{"kind","namespace","name"}],"managedByAxiom"}\|null,"managedByAxiom"}`. Admin, operator and viewer |
| DELETE | `/api/v1/gitops/{kind}/{namespace}/{name}?removeSource=true\|false&confirm=<name>` | `200 {"ok":true,"deleted":[{"kind","namespace","name"}],"cascade","warnings":[...]}` (`warnings` omitted when empty). **Admin only** |

Both take the same body (unknown fields are rejected; there is no way to send raw YAML):

```json
{
  "engine": "argocd | flux-kustomize | flux-helm",
  "name": "my-app",
  "namespace": "argocd",
  "source": {
    "repoUrl": "https://github.com/org/repo.git",
    "path": "deploy/overlays/prod",
    "revision": "main",
    "chart": "nginx",
    "chartVersion": "15.1.0",
    "secretRef": "repo-creds"
  },
  "destination": { "namespace": "prod", "server": "" },
  "project": "default",
  "interval": "5m",
  "values": "replicaCount: 2\n",
  "syncPolicy": { "automated": true, "prune": true, "selfHeal": true, "createNamespace": true }
}
```

| Field | Rules |
|---|---|
| `name` | DNS-1123 label, at most 53 characters. The Flux source object gets the same name |
| `namespace` | Where the objects live; default `argocd` / `flux-system`. A different namespace adds a warning |
| `source.repoUrl` | Required, at most 512 characters, no whitespace, no query or fragment. Git sources: `https`, `http`, `ssh`, `git` or scp-like `git@host:org/repo.git` (Argo CD); Flux git takes `https`, `http`, `ssh`. Helm sources: `https`, `http`, `oci`. **Credentials in the URL are refused** (`https://user:pass@…`, `https://token@…`) — use `secretRef` (Flux) or Argo CD repository credentials |
| `source.path` | Git sources; relative, no `..`, no leading `/`, no backslash, at most 256 characters; default `.`; cleaned |
| `source.revision` | Git sources; default `HEAD` (Argo CD) / `main` (Flux). Flux: `refs/tags/X` → `ref.tag`, a 40-hex SHA → `ref.commit`, anything else → `ref.branch` |
| `source.chart`, `source.chartVersion` | Helm sources: Argo CD with `chart` (then `chartVersion` is required and `path`/`revision` must be empty) and `flux-helm` (`chart` required, `chartVersion` optional) |
| `source.secretRef` | Flux only: the name of an existing Secret in `namespace`. On `argocd` it is a `400` |
| `destination.namespace` | DNS-1123 label; required for `createNamespace` |
| `destination.server` | Argo CD only: empty (`https://kubernetes.default.svc`) or an `https` URL without credentials |
| `project` | Argo CD only; default `default` |
| `interval` | Flux only: `<n>s`, `<n>m` or `<n>h`, at least `30s`; default `5m` |
| `values` | Helm sources only: a YAML mapping, at most 64 KiB (alias expansion and nesting are capped) |
| `syncPolicy` | Argo CD: `prune`/`selfHeal` need `automated`. Flux always reconciles and corrects drift, so `automated`/`selfHeal` are implied; `prune` applies to `flux-kustomize` only, `createNamespace` to `flux-helm` only |

Rendering:

| Engine | Objects | Notable fields |
|---|---|---|
| `argocd` | `argoproj.io` `Application` | `spec.project`, `spec.source{repoURL, path \| chart, targetRevision, helm.values}`, `spec.destination{server, namespace}`, `spec.syncPolicy{automated{prune, selfHeal}, syncOptions: [CreateNamespace=true]}`. No `resources-finalizer` is added |
| `flux-kustomize` | `source.toolkit.fluxcd.io` `GitRepository`, then `kustomize.toolkit.fluxcd.io` `Kustomization` | `GitRepository.spec{url, interval, ref, secretRef}`; `Kustomization.spec{interval, path: ./<path>, prune, sourceRef, targetNamespace, wait: false, timeout: 5m}` |
| `flux-helm` | `source.toolkit.fluxcd.io` `HelmRepository`, then `helm.toolkit.fluxcd.io` `HelmRelease` | `HelmRepository.spec{url, interval, type: oci (for oci://), secretRef}`; `HelmRelease.spec{interval, chart.spec{chart, version, sourceRef}, targetNamespace, install.createNamespace, values}` |

The preview uses the newest API version of each kind (`v1alpha1`, `source v1`, `kustomize v1`, `helm v2`); create uses the first version the cluster serves (falling back to `v1beta2` sources/Kustomizations and `v2beta2`/`v2beta1` HelmReleases). Warnings: automated prune over the whole repository (`path` `.`), a moving revision (`HEAD`/`main`) with automated sync (always for Flux), a plain `http://` repository, a non-default namespace.

Errors: `400` invalid input (the message never echoes the repository URL), `403` for anyone but an admin (operators, viewers and namespace-restricted accounts; creating an application deploys arbitrary workloads, like Helm install), `409` when an object already exists (if the second object fails, the first one this request created is deleted again and the message says so), `422` when the engine is not installed (`Argo CD is not installed: no applications.argoproj.io CRD`). A successful create is audited as `gitops.create` with target `<kind>/<namespace>/<name>` and the engine as detail. Every created object carries the label `app.kubernetes.io/managed-by: zyvor-axiom`.

**Deletion.** `kind` is `Application`, `Kustomization` or `HelmRelease` (anything else is `400`). `cascade` says whether the controller will also remove what the application deployed: an Argo CD `Application` cascades iff `metadata.finalizers` contains `resources-finalizer.argocd.argoproj.io` (or a `resources-finalizer.argocd.argoproj.io/*` variant such as `/background`); a Flux `Kustomization` iff `spec.prune` is true (its inventory is garbage-collected); a Flux `HelmRelease` always (the Helm release is uninstalled) unless `spec.suspend` is true. `consequence` is a plain-English sentence for each case, `detail` lists the facts behind it (`finalizer: …`, `spec.prune: …`, `spec.suspend: …`). `source` is, for Flux objects, the `GitRepository`/`HelmRepository` named by `spec.sourceRef` / `spec.chart.spec.sourceRef` (namespace defaults to the object's), with `sharedWith` = the other Kustomizations/HelmReleases in the cluster referencing the same source and `managedByAxiom` = whether the source has the managed-by label; `null` for Argo CD. The top-level `managedByAxiom` is the same check on the application. The DELETE recomputes all of this at delete time (the object may have changed since the preview) and, when `cascade` is true, requires `confirm` to equal the object name exactly (case-sensitive) — otherwise `400 this deletion also removes the application's resources; pass confirm=<name>`; non-cascading deletes need no `confirm`. The object is deleted with default propagation (what happens next is the controller's finalizer semantics) and a UID precondition. `removeSource=true` also deletes the Flux source afterwards, but only when `sharedWith` is empty and the source carries the managed-by label (`409` naming the sharing objects, or saying the source was not created by Axiom; `400` for an Argo CD Application); these checks run before anything is deleted. If the source delete itself fails the response is still `200` with a `warnings` entry (the application is already gone); a source that no longer exists is a warning too. `404` unknown object, `422` engine not installed, `403` for operators, viewers and namespace-restricted accounts (the preview: restricted accounts only). Audited as `gitops.delete` with target `<kind>/<namespace>/<name>` and detail `cascade=<bool>`.
### Helm chart catalog

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/helm/repos` | Chart repositories: `[{"name","url","addedAt","chartCount","fetchedAt","error"}]`. `chartCount`/`fetchedAt`/`error` come from the cached index; this call never fetches |
| POST | `/api/v1/helm/repos` | Add a repository: `{"name","url"}` → `201` with the repository object. **Admin only.** `name` is a DNS-1123 label of at most 40 characters; `url` must pass the fetch rules below and must not carry userinfo, a query string or a fragment (a trailing `/` is dropped). The index is fetched once and must parse (`400` with the reason otherwise, including an index over 20 MiB); `409` for a duplicate name or URL; at most 20 repositories. Audited as `helm.repo_add` |
| DELETE | `/api/v1/helm/repos/{name}` | Remove a repository and its cached index → `{"ok":true}`; `404` if unknown. **Admin only.** Audited as `helm.repo_remove` |
| GET | `/api/v1/helm/charts?repo=&q=&limit=&refresh=` | Search: `{"charts":[{"repo","name","version","appVersion","description","icon","home","keywords","deprecated","created"}],"total","fetchedAt","errors":{"<repo>":"msg"}}`. One entry per chart — its latest non-deprecated, non-prerelease version (the newest one if every version is deprecated or a prerelease). `repo` omitted = every repository; a repository that cannot be fetched is reported in `errors` and does not fail the call (its last good copy is still served). `q` is a case-insensitive substring over name, description and keywords, ranked exact name > name prefix > name contains > description/keyword, then by name. `limit` defaults to 60, max 200; `total` counts every match. `fetchedAt` is the oldest index among the answers. `refresh=true` re-fetches, at most once per repository every 30 s. `404` for an unknown `repo` |
| GET | `/api/v1/helm/charts/{repo}/{chart}` | `{"repo","name","description","home","sources","keywords","icon","maintainers":[{"name","email"}],"versions":[{"version","appVersion","created","deprecated"}]}` — metadata from the latest version, up to 100 versions newest first (semver order; prereleases below their release; unparseable versions last). `404` for an unknown repository or chart |
| GET | `/api/v1/helm/charts/{repo}/{chart}/values?version=` | `{"version","values","readme"}`: the chart's default `values.yaml` and `README.md` (truncated to 64 KiB; may be empty). `version` defaults to the latest. Downloads the archive the index names (`urls[0]`, relative to the repository URL) and reads only `<chart>/values.yaml` and `<chart>/README.md` from it in memory. `404` for an unknown version, `413` "chart archive too large" (archive over 20 MiB or `values.yaml` over 2 MiB), `502` when the repository or archive cannot be fetched or the index points somewhere the fetch rules refuse |

Reads are open to every role except namespace-restricted accounts (`403`: `/api/v1/helm/repos` and `/api/v1/helm/charts` are restricted-sensitive prefixes); writes are admin-only, so operators and viewers get `403`. Responses carry `Cache-Control: no-store`.

**Fetch rules.** The server fetches URLs an administrator typed and URLs a repository index names, so every request is checked: `https://` only (`http://` with `AXIOM_HELM_ALLOW_INSECURE_REPOS=true`); no userinfo; no numeric hosts that are not dotted quads (`2130706433`, `0x7f000001`, `127.1`); and the host's resolved addresses must all be public — loopback, `localhost`, link-local (`169.254.0.0/16` including cloud metadata, `fe80::/10`), private (`10/8`, `172.16/12`, `192.168/16`, `fc00::/7`), CGNAT (`100.64/10`), unspecified, multicast and broadcast are refused unless `AXIOM_HELM_ALLOW_PRIVATE_REPOS=true` (for an in-cluster ChartMuseum or Harbor). The check runs at connect time on the address actually dialed, so a DNS answer that changes between check and connect cannot bypass it, and it applies to every redirect (at most 3, each re-validated) and to chart archive URLs on other hosts. No proxy, cookies or credentials are used; each request has a 15 s timeout and the index is capped at 20 MiB. Parsed indexes are cached in memory for 10 minutes per repository (one fetch at a time per repository; a failing repository is retried at most every 30 s), and at most 5,000 charts are read from one index. The repository list is kept in ConfigMap `zyvor-axiom-helm-repos` (key `repos.json`) in the kubernetes build and in memory in demo mode, where it starts empty.

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
| POST | `/api/v1/integrations/ai/test` | Send a fixed, cluster-data-free prompt to the configured AI endpoint (admin only) |

`PUT /api/v1/integrations` also accepts `slackWebhookUrl`, `alertWebhookUrl`, and `notifyOnWarnings` (bool). When `notifyOnWarnings` is true and at least one webhook is set, a background dispatcher polls `GET /api/v1/alerts`-equivalent Warning events every 60s and forwards new ones to both configured channels.

**Notifications.** The same channels and switch carry Axiom's own notifications: a cluster becoming unreachable or degraded and recovering (from the fleet prober), and a cost budget reaching `warning`, `over` or back to `ok` (from the budget watcher). Slack receives three lines — `:warning: *Axiom: cluster eu-west-1 is unreachable*`, the one-line detail, and ``In Axiom: `/fleet` ``; the generic webhook receives `{"source":"zyvor-axiom","kind":"fleet.cluster_unreachable","severity":"warning","title","detail","path","time"}` where `kind` is the matching audit action and `path` is an Axiom-relative UI path (`/fleet`, `/ops/cost`). URLs inside a detail are cut down to scheme and host. A cluster or namespace is not re-announced in the same state within 10 minutes; a recovery / back-to-ok message is sent whenever the problem was announced; conditions present when Axiom starts are not announced. Delivery is best effort: one attempt per channel, failures are logged with the webhook URL redacted, and each replica sends its own.

`PUT /api/v1/integrations` also accepts the AI assistant fields `aiEnabled` (bool), `aiProvider` (`openai-compatible` | `anthropic` | `ollama`), `aiBaseUrl` (http/https only, no embedded credentials, no query), `aiModel`, `aiRedact` (bool, default `true`), `aiApiKey` (write-only) and `aiApiKeyClear`. `GET` returns `aiApiKeySet` instead of the key. Invalid scheme/provider is a `400`.

## AI assistant (opt-in)

Off unless an admin enables it under Settings → AI assistant. Bring-your-own endpoint; see [DEPLOYMENT.md](DEPLOYMENT.md#ai-assistant-and-privacy) for what leaves the cluster.

| Method | Path | Role | Notes |
|---|---|---|---|
| GET | `/api/v1/ai/status` | any signed-in user | `{enabled, configured, provider, baseUrl, model, redact, demo}`; never contains the API key. |
| POST | `/api/v1/ai/explain?namespace=<ns>` | viewer, operator, admin | Read-only. `namespace` is **required in the query** (no `all`); the body is `{kind: "pod"\|"event", name, reason?, question?, preview?}` where `name` is the pod name, or `Kind/name` for `event`. Unknown body fields (including `namespace`) are rejected so the namespace the auth guard authorizes is the namespace that is read. |

`POST /ai/explain` behaviour:

- `preview: true` returns the exact redacted payload and destination without contacting the model (and is not audited); otherwise the payload is sent and `{destination, payload, answer}` is returned. `answer` is plain text.
- Disabled or unconfigured: `409`. Per-user rate limit: 10 sends and 30 previews per minute, `429` with `Retry-After`. Unknown pod: `404`. Model endpoint error or timeout: `502`.
- Audit entry `ai.explain` with target `<namespace>/<kind>/<name>` only. Prompt and answer text are never stored.
- Namespace-restricted identities may only ask about their own namespaces (route is in the namespaced-route list); `/ai/status` and the settings routes follow the usual rules (settings and `/integrations/ai/test` are admin only).

## Users

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/users` | List local accounts (username, role, namespaces, createdAt — never the password hash) |
| POST | `/api/v1/users` | Create a local account: `{"username","password","role":"admin"\|"operator"\|"viewer","namespaces":[...]}` (password ≥ 8 chars; `namespaces` is ignored unless `role` is `viewer`) |
| DELETE | `/api/v1/users/{username}/mfa` | Admin only. Remove a person's second factor (and any pending enrolment) and revoke their sessions — for someone who has lost their device and recovery codes. Audited as `user.mfa_reset` |
| POST | `/api/v1/users/{username}/revoke-sessions` | Admin only. Sign the user out everywhere. Audited as `user.sessions_revoked` |
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
