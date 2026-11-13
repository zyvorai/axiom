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
| GET | `/api/v1/fleet/history?cluster=&window=24h` | Uptime, incidents and a health/latency/utilisation series per cluster from the probe history, persisted across restarts (see below). Same access as `/fleet` (namespace-restricted accounts get `403`); `400` for a bad `window` or `cluster`, `404` for an unknown cluster. Never cached |
| GET | `/api/v1/fleet/search?q=&kind=&limit=` | Search workloads, services and namespaces across every registered cluster (see below). Same access as `/fleet` (namespace-restricted accounts get `403`); `400` for a bad `q`, `kind` or `limit`; `429` with `Retry-After` beyond 20 searches per user per 10 s. Never cached |

**Fleet health.** Axiom probes every registered cluster in the background (every `AXIOM_FLEET_PROBE_INTERVAL`, default 30 s, up to 8 clusters at once, 8 s timeout each so one dead cluster never stalls the rest) by asking it for its overview — "reachable" means the API server answers, not merely that a kubeconfig exists. Each cluster is `healthy`, `degraded` (reachable, but some nodes are not ready or the cluster reports an unhealthy status; `reason` says which — warning events alone do not degrade a cluster), `unreachable` (the probe failed or timed out; `error` carries the message. A cluster that was reachable needs two consecutive failed probes before it is declared unreachable, so one slow response is not an outage; a cluster's very first probe reports immediately) or `unknown` (not probed yet; a read of `/fleet` probes such clusters first). A cluster entry has `name`, `label`, `current`, `server`, `state`, `reason`, `error`, `checkedAt`, `since` (when the state last changed), `latencyMs` and a headline summary (`nodes`, `readyNodes`, `pods`, `runningPods`, `workloads`, `vms`, `warnings`, `cpuPercent`, `memoryPercent`); `summary` rolls them up, counting resources only for clusters that answered. `GET /api/v1/clusters` carries `state`, `latencyMs` and `error` too, and its `healthy` flag is now false only for an unreachable cluster. Changes of state are written to the audit log (`fleet.cluster_unreachable`, `fleet.cluster_degraded`, `fleet.cluster_recovered`) and, when notifications are on (`notifyOnWarnings` plus a Slack or alert webhook), announced there too — see *Notifications* under Integrations. A cluster registered from Settings is probed immediately.

**Fleet history.** Every probe (background, "Check now", or the lazy probe of a never-probed cluster) appends one sample per cluster **after** the two-failure rule, so the history shows the state the console showed: `{"t" (unix seconds),"state","latencyMs","readyNodes","nodes","runningPods","pods","warnings","cpuPercent","memoryPercent"}`. Samples are kept for 24 h, at most 3,000 per cluster, in memory (see *Durable fleet history* below for restarts), and dropped when a cluster is unregistered. `GET /api/v1/fleet/history` takes `window` `1h`, `6h` or `24h` (default) and an optional `cluster` (omitted = every cluster, in registration order; 1–253 characters, no control characters) and returns `{"window","intervalSeconds","durable","restoredFrom","clusters":[{"cluster","label","samples":[...],"uptimePercent","checks","failedChecks","currentState","since","incidents":[{"state","from","to","durationSeconds","reason"}],"gaps":[{"from","to"}]}]}`. `samples` has at most 240 points: when the window holds more, it is split into 240 equal time buckets and each non-empty bucket becomes one point with the worst state in it (`unreachable` > `degraded` > `healthy`), the average latency, CPU and memory, the last counts and the time of its last sample. `uptimePercent` is the share of time the cluster was reachable (`healthy` or `degraded`), each sample weighted by the gap to the next one capped at 2 × the probe interval (the newest counts one interval), two decimals, `null` with no samples. `checks` counts the samples in the window; `failedChecks` the probes that failed, including a single failure the two-failure rule absorbed (that sample still shows the held state). `incidents` (oldest first) are runs of non-healthy samples: `state` is the worst state of the run, `from` its first sample, `to` the first healthy sample after it (`null` while ongoing; `durationSeconds` then runs to now), `reason` the latest degraded reason / probe error of that state; runs separated by less than 2 probe intervals of health are merged. `currentState` / `since` are the latest probe's. Demo mode records the three demo clusters like any others; `AXIOM_DEMO_FLEET_HISTORY_SEED=true` backfills 24 h of samples at 30 s with one 20-minute incident about 3 h ago (unreachable on the first `AXIOM_DEMO_FLEET_DOWN` cluster if set, else degraded on `ap-south-1`).

**Durable fleet history.** In `kubernetes` mode every replica saves a downsampled copy of its history every `AXIOM_FLEET_HISTORY_PERSIST_INTERVAL` (default `5m`, minimum `1m`, `off` disables saving and restoring) and once on graceful shutdown: per cluster, 5-minute buckets over the last 24 h (at most 288; a bucket keeps the worst state, average latency / CPU / memory, the last counts, its probe and failed-probe counts and the latest reason), as JSON → gzip → base64 under its own key `history-<replica id>` (the replica id is `POD_NAME`, else the hostname) of the ConfigMap `zyvor-axiom-fleet-history` in the console's namespace. The whole object is kept under 900 KiB by dropping the oldest buckets of the largest cluster first. A key whose newest sample is older than 24 h (a replica that is gone) is deleted on the next save. At startup, before the first probe, every replica's key is read and merged into memory by timestamp (identical timestamps are kept once; clusters no longer registered are dropped); a replica started later therefore sees the others' data as of their last save. Restored samples carry `"restored": true` and count toward `checks` / `failedChecks` with the probes they stand for. The time between the last restored sample and the first live probe is a restart gap, listed in `gaps` and never counted as downtime: in `uptimePercent` it counts at most one probe interval of the state before it (consecutive restored buckets are weighted up to 5 minutes each), and an incident still open at the last restored sample ends there (`to` = that sample) instead of spanning the gap. Top-level `durable` is `true` when a persistent store is configured (never in demo mode) and the last save succeeded within 3 persistence intervals (so it is `false` until the first save after a start); `restoredFrom` is when the newest snapshot that contributed restored samples was saved (RFC 3339), `null` when nothing was restored. A save failure is logged at most every 10 minutes and never affects the console.

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
| GET | `/api/v1/auth/mfa` | The caller's two-factor state: `{"available","enabled","recoveryCodes","managed"?}`. `available` is true for local and directory (LDAP) accounts, and false (with `managed` naming where it lives) for SSO accounts and for the built-in admin account |
| POST | `/api/v1/auth/mfa/setup` | Start enrolment: `{"password"}` → `{"secret","uri","issuer"}`. The password must be the caller's current one, so a stolen session cannot enrol an attacker's device. Nothing changes until `enable` confirms a code. Any role |
| POST | `/api/v1/auth/mfa/enable` | Confirm the secret with a code from the authenticator: `{"code"}` → `{"ok","recoveryCodes":[…10 codes…]}`. The recovery codes are shown **once**. The account's existing sessions are revoked. Any role |
| POST | `/api/v1/auth/mfa/disable` | Turn it off: `{"password","code"}` (a TOTP or an unused recovery code). Revokes the account's sessions. Any role |

**Two-factor sign-in (TOTP).** Local accounts can add a time-based one-time code (RFC 6238: HMAC-SHA1, 6 digits, 30 s, one step of clock drift accepted). Once enabled, the account **cannot authenticate with Basic credentials any more**: a request with the right password but no session gets `401` with `X-Axiom-MFA: required`, and the client signs in through `POST /auth/login` with the code, which returns a session. A TOTP code is accepted once per time step (a replay is refused), and each of the ten recovery codes works once. Wrong codes count toward the same per-IP and per-account throttling as wrong passwords; a right password with a missing code does not. Sign-ins and MFA changes are audited (`auth.login`, `auth.mfa_failed`, `auth.mfa_setup`, `auth.mfa_enabled`, `auth.mfa_disabled`, `user.mfa_reset`). The built-in admin account can be given a second factor with `AXIOM_TOTP_SECRET` (base32, ≥ 16 characters); API keys and SSO accounts are not affected (use your IdP's MFA for SSO). The secret and the hashed recovery codes are stored with the account in the `zyvor-axiom-users` Secret and never returned by any endpoint.

**Directory (LDAP) accounts.** A person who signs in with an LDAP bind enrols exactly like a local account (`/auth/mfa/setup|enable|disable` from the user menu; `setup` and `disable` re-check the *directory* password with a bind). Once enabled the directory password alone no longer works over Basic (`401` + `X-Axiom-MFA: required`) and `POST /auth/login` collects the code. The directory password is always verified first, so a wrong password never reveals whether the account has a second factor. Sessions issued for directory accounts carry `Issuer: "ldap"` and the role mapped from the directory at sign-in (a group change applies at the next sign-in; revoking works as for any session), and `GET /session` reports `"auth":"ldap"`. Their second factors live in the `zyvor-axiom-external-mfa` Secret (key `records.json`, keyed by lower-cased directory username; memory in demo), written with resourceVersion-checked updates retried on conflict, so replicas never lose each other's change or accept a replayed code. A local account with the same username takes precedence: its own second factor applies and the directory record is not used. MFA audit entries for directory accounts carry `detail: "ldap"`.

**Require-MFA policy.** `AXIOM_REQUIRE_MFA` = `off` (default) | `admin` | `operator` (operators and admins) | `all` (every local account, viewers included) makes a second factor mandatory for local and directory (LDAP) accounts in those roles (for LDAP, the role mapped from the directory). A covered account that has not enrolled can still authenticate (Basic or `POST /auth/login` without a code) so that it can enrol, but every request except `GET /session`, `GET /auth/providers`, `POST /auth/logout`, `POST /auth/logout-all`, `GET /auth/mfa` and `POST /auth/mfa/setup|enable|disable` (plus the public health and metrics) is refused by the guard with:

```json
{"error":"two-factor sign-in is required for your role: set it up under Account security","mfaEnrollmentRequired":true}
```

(`403`). Disabling MFA is allowed, but a covered account that does so is blocked again from its next request. SSO accounts and API keys are never affected. The built-in admin is break-glass: without `AXIOM_TOTP_SECRET` it is exempt (logged at start-up); with it, it signs in with a code and satisfies the policy. `GET /session` reports the state as `"mfa": {"required": bool, "enrolled": bool}` — `required` is true when the policy covers this identity (always false for SSO and API-key identities, and for the exempt built-in admin), `enrolled` when it has a second factor — so the UI can force enrolment.

**Sessions and revocation.** Sessions are signed tokens that last `AXIOM_SESSION_TTL` (default 12 h, minimum 1 minute, maximum 30 days). Each has an ID, so logout revokes exactly that session, and `logout-all` revokes every session a user issued before now. Axiom also revokes a user's sessions when an administrator changes their role, namespaces or password, deletes them, or when their MFA is enabled, disabled or reset. Sessions from the local sign-in are re-resolved against the live account on every request, so a role or namespace change and a deleted account apply immediately. Revocations are kept in the `zyvor-axiom-revocations` ConfigMap (survive restarts). Every replica merges its changes into the ConfigMap (a union, with resourceVersion-checked updates so concurrent revocations are all kept) and re-reads it every `AXIOM_REVOCATION_REFRESH` (default 15 s), so a revocation made on one replica applies on the others within that interval; it applies at once on the replica that handled it.

**OIDC verification.** The callback verifies the ID token's signature against the provider's JWKS (fetched from the discovery document's `jwks_uri`, 10 s timeout, cached 15 min, re-fetched at most every 30 s on an unknown `kid`), accepts only RS256/384/512, PS256/384/512 and ES256/384/512 (never `none` or HMAC), then checks `iss`, `aud` (and `azp` for multi-audience tokens), `exp`, `sub` and the `nonce` bound to the sign-in by the `axiom_oidc_nonce` cookie. Any failure rejects the login with `401`.

**Login throttling.** Failed credentials are throttled twice: per client IP (grace of 3, then exponential backoff to 5 min) and per attempted account name (grace of 5, same backoff, keyed on the lower-cased username whether or not the account exists). A successful login clears the per-IP counter and only the account counter of the account whose own credentials were verified, so interleaving a valid login from another account no longer defeats throttling. A throttled request gets `429` with `Retry-After`; lockouts are audited as `auth.rate_limited` / `auth.account_rate_limited`. Lockout is per account, so someone who knows a username can temporarily lock it out - it clears by itself within 5 minutes. In kubernetes mode counters are shared across replicas via ConfigMap `zyvor-axiom-login-throttle` (best-effort merge; a brief race can still under-count).

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
| GET | `/api/v1/workloads/{namespace}/{kind}/{name}/still?action=` | Preview Still actions (hush/sleep/seal/end), including live terminal/window counts and Atlas volumes |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/still` | Execute Still (`?action=` or JSON `{"action"}`): hush/seal → isolate, sleep → scale to zero, end → delete; audits blast counts; seal writes a PacketWolf flow receipt |
| POST | `/api/v1/workloads/{namespace}/{kind}/{name}/seal` | Apply deny-all NetworkPolicy isolation (also records PacketWolf seal receipt when configured) |
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
| GET | `/api/v1/pods/{namespace}/{name}/window/{port}/` | HTTP window proxy into Pod (reverse-proxies into the pod's container port). **`410 Gone` when the separate pod proxy origin is configured**; use `window-link` |
| POST | `/api/v1/pods/{namespace}/{name}/window-link` | Only with the [pod proxy origin](#pod-proxy-origin) on (`404` otherwise). Body `{"port":8080,"path":"/"}` (`path` optional, an absolute path, may carry a query) → `{"url":"<origin>/p/<ticket>/","expiresIn":60}` (in host mode `<origin>` is a fresh `https://<sid>.<base>` per link), `Cache-Control: no-store`. Same policy as the window route: operator or admin, namespace-checked (viewers and accounts restricted to other namespaces get `403`). `404` unknown pod, `403` undeclared port or hostNetwork pod, `429` too many outstanding links. The pod is resolved on the calling tab's active cluster (`X-Axiom-Session`), and the link and every session made from it stay pinned to that cluster by its registered name. Audited as `pod.window` (with the cluster name on a multi-cluster install) |
| GET | `/api/v1/pods/{namespace}/{name}/metrics` | Live CPU/memory for the pod and each container, plus a short in-memory rolling time series (not historical Prometheus data) |

Exec accepts a shell string or argv array:

```json
{"container":"app","command":"whoami"}
```

```json
{"container":"app","command":["/bin/sh","-lc","env | sort"]}
```

`/logs`, `/exec`, `/terminal` and `/window/{port}/` are all reached through a plain browser navigation or WebSocket, neither of which can carry a custom `Authorization` header. For `/terminal` and other streams use a single-use [stream ticket](#stream-tickets) (`?ticket=`); for the `/window/{port}/` HTTP proxy, which loads many sub-resources, local Basic-auth sessions still append the deprecated `?access_token=<base64 user:pass>` (see [Authentication](#authentication) below). OIDC/SAML cookie sessions need nothing extra. With the pod proxy origin on, the in-origin route is gone and no URL token is involved.

### Pod proxy origin

Opt-in (`AXIOM_POD_PROXY_ADDR` + `AXIOM_POD_PROXY_ORIGIN`, see `docs/DEPLOYMENT.md`): pod web content is served by a second listener on its own origin instead of the console's, so a malicious pod UI cannot script the console (its session token, cookies or API). `GET /api/v1/realtime/capabilities` reports `"podProxy": true` when it is on, plus `"podProxyMode": "path"` (one fixed origin, the default) or `"host"` (one origin per pod session, below); the UI's "Open" button then calls `window-link` and opens the returned URL, which it treats as opaque.

The proxy listener serves only (path mode):

| Method | Path | Purpose |
|---|---|---|
| GET | `/p/<ticket>/` | Redeems the single-use ticket from `window-link` (256-bit, 60 s, bound to user, namespace, pod, port and base path), sets an HttpOnly, `SameSite=Lax` session cookie scoped to `Path=/s/<sid>/` (`__Secure-axiom_pod_proxy` on https, `axiom_pod_proxy` on http) and answers `302` to `/s/<sid>/<path>`. An unknown, expired or reused ticket is `403`; a ticket for a cluster this replica no longer has (or never had) registered is `410` and creates no session |
| GET, HEAD, POST, PUT, PATCH, DELETE | `/s/<sid>/…` | Proxied to the pod (the path below `/s/<sid>` is forwarded as is; WebSocket upgrades pass through). Needs the session cookie of *that* session id: missing, mismatched or expired is `403`. The session lasts 10 minutes from its last use and 1 hour at most, and dies as soon as the user who created it is revoked (sign out everywhere, password/role/namespace change, deletion). The pod is looked up on the cluster recorded when the link was minted, whatever any console tab or replica has selected since; once that cluster is unregistered (or on a replica that does not know it) requests are `410` |

Everything else is `404`. The listener has no CORS and never reads the console's `axiom_session` cookie or `Authorization`/API-key credentials; `Authorization`, `Cookie`, `X-Api-Key`, `X-Axiom-Session` and URL tokens are removed before the request reaches the pod. Every response carries `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer` and `Cross-Origin-Opener-Policy: same-origin`, and proxied content gets `Content-Security-Policy: sandbox allow-scripts allow-forms allow-same-origin allow-popups; frame-ancestors 'none'`. The pod's `Set-Cookie`, `Access-Control-*`, `Service-Worker-Allowed`, `Clear-Site-Data`, HSTS and its own copies of those policy headers are dropped, and service-worker script requests are refused. Request headers are capped at 32 KiB and bodies at 32 MiB.

**Host mode (one origin per pod session).** When `AXIOM_POD_PROXY_ORIGIN` has the placeholder `{id}` as its leftmost host label (`https://{id}.pods.example.com`, or `http://{id}.localhost:5091` in a lab), `window-link` picks the session id up front (128-bit random, 32 lowercase hex characters) and answers `{"url":"https://<sid>.pods.example.com/p/<ticket>/","expiresIn":60}`; the ticket is bound to that id. On the listener, the `Host` header selects the session, and only `<32 hex>.<base domain>` on the configured port is served; any other host, and an unknown or expired id, is an empty `404`.

| Method | Path (on host `<sid>.<base>`) | Purpose |
|---|---|---|
| GET | `/p/<ticket>/` | Only while session `<sid>` does not exist yet: redeems the ticket, which must have been minted for `<sid>` (a ticket presented on another session's host is `403` and burnt), sets a host-only session cookie (`Path=/`, no `Domain`, HttpOnly, `SameSite=Lax`; `__Host-axiom_pod_proxy` with `Secure` on https, `axiom_pod_proxy` on http) and answers `302` to the link's base path at the root of the host |
| GET, HEAD, POST, PUT, PATCH, DELETE | `/…` | Once the session exists: proxied to the pod at the same path, so pod UIs that use absolute paths (`/static/app.js`, `/api/…`) work. `/p/…` is an ordinary pod path from then on. The session cookie is required (`403` otherwise); a cookie from another session never matches, even when replayed against this host |

Session lifetimes, revocation, header stripping, response policy headers (including `allow-same-origin` in the sandbox: the origin belongs to this one session), the service-worker refusal and the limits are the same as in path mode.

**Several replicas.** With `AXIOM_SESSION_SECRET` set, the ticket and the session cookie are opaque HMAC-signed tokens (base64url, no separators) that any replica sharing the secret accepts, so a link minted by one replica can be opened on another and the session continues on any of them. Treat both as opaque: their content (user, role, namespace restriction, pod, port, base path, session id, issue and expiry times) is integrity-protected, not encrypted. A ticket is still single use across replicas (best effort, see `docs/GAPS.md`) and the response codes above are unchanged, except that in host mode a replica that did not create the session answers a request without a valid cookie with the empty `404` instead of `403`. While the session is in use, a response may carry a re-issued session cookie (same name and attributes, a later signed idle deadline, at most once a minute, `Max-Age` up to the session's 1-hour limit); browsers store it automatically. Without a session secret tickets and sessions stay in the issuing process's memory.

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

When the CRD is not installed the list answer is `200` with `{"installed":false,"items":[]}`, not an error; any other failure (for example Axiom's ServiceAccount lacking `list`) is an error. A trigger's `name` is its own `name`, else its `metricName`/`queueName`/`topic`/`listName`/`streamName`/`subscriptionName` metadata value, else `""`; **no other trigger metadata and no `authenticationRef` is ever returned** (they can carry endpoints and credentials). Both routes are namespaced reads like `/api/v1/hpas`: a namespace-restricted account must pass `?namespace=` with one of its namespaces (`all` or no namespace is `403`).

#### Creating and deleting VPAs and ScaledObjects

| Method | Path | Purpose |
|---|---|---|
| POST | `/api/v1/vpas/preview` | Validate and render a VPA without touching the cluster: `{"objects":[{"apiVersion","kind","namespace","name"}],"yaml","warnings":[]}` |
| POST | `/api/v1/vpas` | Create it: `201` `{"ok":true,"created":[...],"warnings":[]}` |
| DELETE | `/api/v1/vpas/{namespace}/{name}` | Delete one: `{"ok":true}` (the target workload is not touched) |
| POST | `/api/v1/scaledobjects/preview` | Validate and render a KEDA ScaledObject (no cluster access) |
| POST | `/api/v1/scaledobjects` | Create it: `201` `{"ok":true,"created":[...],"warnings":[]}` |
| DELETE | `/api/v1/scaledobjects/{namespace}/{name}` | Delete one: `{"ok":true}` |
| GET | `/api/v1/triggerauthentications?namespace=...` | KEDA TriggerAuthentication **names** for the form's dropdown: `{"installed","items":[{"namespace","name","kind":"TriggerAuthentication","age"}]}` (never the spec or its secret references; ClusterTriggerAuthentications are not listed) |

The bodies are typed; raw YAML is never accepted (unknown JSON fields are `400`). Preview works without the CRD; create and delete answer `422` when the CRD is absent, create `409` when the object exists, delete `404` when it does not. Existing objects can be edited in place (below). Created objects carry `app.kubernetes.io/managed-by: zyvor-axiom`. Audited as `autoscale.vpa_create` / `vpa_delete` / `keda_create` / `keda_delete` with target `<namespace>/<name>` (trigger settings are never logged).

VPA body (`autoscaling.k8s.io` `v1`, else `v1beta2`):

```json
{"namespace":"commerce","name":"checkout-vpa","targetKind":"Deployment","targetName":"checkout","updateMode":"Off",
 "controlledResources":["cpu","memory"],"containers":[{"name":"app","minCpu":"50m","maxCpu":"2","minMemory":"64Mi","maxMemory":"2Gi"}]}
```

`targetKind` is `Deployment`\|`StatefulSet`\|`DaemonSet`\|`ReplicaSet`; names and namespaces are DNS-1123. `updateMode` defaults to `Off` (recommend only); `Initial`/`Recreate`/`Auto` add the warning "pods may be restarted to apply new requests". `controlledResources` defaults to both; restricting it without `containers` renders a `containerName: "*"` policy. `containers` is optional (`name` a container name or `*`); each quantity is optional, parsed as a Kubernetes quantity, positive, cpu at most 256 cores and memory at most `4Ti`, min not above max, and only for a controlled resource. Create also warns "VPA and HPA on the same resource conflict" when an HPA in the namespace scales the same workload on a controlled resource (preview cannot know).

ScaledObject body (`keda.sh/v1alpha1`):

```json
{"namespace":"payments","name":"payments-api-scaler","targetKind":"Deployment","targetName":"payments-api","minReplicas":0,"maxReplicas":20,"pollingInterval":30,"cooldownPeriod":300,
 "triggers":[{"type":"cron","name":"business-hours","settings":{"timezone":"Europe/Berlin","start":"0 8 * * 1-5","end":"0 18 * * 1-5","desiredReplicas":"5"}},
             {"type":"prometheus","settings":{"serverAddress":"http://prometheus.monitoring:9090","query":"sum(rate(http_requests_total[2m]))","threshold":"100"},"authenticationRef":"prom-auth"}]}
```

`targetKind` is `Deployment`\|`StatefulSet`. `maxReplicas` is required, 1–1000; `minReplicas` 0–max (default 0 when an event trigger is present, else 1), and **0 is only allowed when at least one trigger is not `cpu`/`memory`** (they are measured on running pods, so KEDA cannot wake a workload from zero on them). `pollingInterval` (1–3600, default 30) and `cooldownPeriod` (1–86400, default 300) are seconds. 1–10 triggers; a trigger `name` is optional, DNS-1123 and unique. It renders `spec.scaleTargetRef{kind,name}`, `minReplicaCount`, `maxReplicaCount`, `pollingInterval`, `cooldownPeriod` and `triggers[{type,name?,metadata,authenticationRef{name}?,metricType?}]`.

| `type` | Required settings | Optional | `authenticationRef` |
|---|---|---|---|
| `cpu`, `memory` | `type` (`Utilization`\|`AverageValue`, rendered as `metricType`), `value` (1–1000 % or a quantity) | | not allowed |
| `cron` | `timezone` (IANA), `start`, `end` (5-field cron), `desiredReplicas` | | not allowed |
| `prometheus` | `serverAddress` (http/https URL, no userinfo), `query`, `threshold` | `activationThreshold`, `namespace` | optional |
| `kafka` | `bootstrapServers` (`host:port,...`), `consumerGroup`, `topic`, `lagThreshold` | `activationLagThreshold` | optional |
| `redis` (lists) | `address` (`host:port`), `listName`, `listLength` | `activationListLength` | optional |
| `rabbitmq` | `queueName`, `mode` (`QueueLength`\|`MessageRate`), `value` | `protocol` (`auto`\|`amqp`\|`http`) | **required**: the host (it carries the password) is supplied only through it, never as `host` |
| `aws-sqs-queue` | `queueURL` (https), `queueLength`, `awsRegion` | | optional |

Every setting value is a string of at most 512 characters with no control characters. Any other type or key is `400` naming the allowed ones. **Credentials are never accepted in settings**: a key that looks like one (`password`, `token`, `secret`, `key`, `auth`, ...) and any value with a URL carrying userinfo (`scheme://user:pass@host`, `user@host:port`) are `400`, and error messages never echo setting values. Credentials go through `authenticationRef`, the name of an existing TriggerAuthentication in the same namespace; create checks it exists (`400` "TriggerAuthentication X not found in namespace Y"), preview does not. Create warns "KEDA creates its own HPA; two autoscalers on one workload fight each other" when a plain HPA (not a KEDA-owned `keda-hpa-*`) already targets the workload.

Operators and admins may create and delete (viewers get `403`). For a namespace-restricted operator the `POST` bodies' `namespace` must be one of its namespaces (checked by the handler, `403` otherwise; this applies to `POST /api/v1/hpas` as well) and the `DELETE` path namespace is checked by the guard. `/api/v1/triggerauthentications` is a namespaced read like the lists above.

#### Editing VPAs and ScaledObjects in place

Same design as the GitOps edit: the typed creation fields read back from the live object, a managed-field diff, and a `resourceVersion`-guarded JSON Patch of only those fields.

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/vpas/{namespace}/{name}/edit` | The VPA's current typed fields: `{"namespace","name","targetKind","targetName","updateMode","controlledResources","containers":[{"name","minCpu"?,"maxCpu"?,"minMemory"?,"maxMemory"?}],"resourceVersion","editable","reasons":[],"readOnly":["namespace","name","targetKind","targetName"]}` |
| POST | `/api/v1/vpas/{namespace}/{name}/edit/preview` | Diff an edit without changing anything: `{"changes":[{"object":"VerticalPodAutoscaler/commerce/checkout-vpa","field":"spec.updatePolicy.updateMode","from":"Off","to":"Auto"}],"yaml","warnings":[],"noop"}` |
| PUT | `/api/v1/vpas/{namespace}/{name}` | Apply it: `200` `{"ok":true,"noop":false,"changed":["spec.updatePolicy.updateMode"],"warnings":[],"resourceVersion":"..."}` |
| GET | `/api/v1/scaledobjects/{namespace}/{name}/edit` | `{"namespace","name","targetKind","targetName","minReplicas","maxReplicas","pollingInterval","cooldownPeriod","triggers":[{"type","name"?,"settings":{...},"authenticationRef"?}],"resourceVersion","editable","reasons","readOnly"}` |
| POST | `/api/v1/scaledobjects/{namespace}/{name}/edit/preview` | e.g. `{"changes":[{"object":"ScaledObject/payments/payments-api-scaler","field":"spec.maxReplicaCount","from":"20","to":"30"}],"yaml","warnings":[],"noop":false}` |
| PUT | `/api/v1/scaledobjects/{namespace}/{name}` | `200` `{"ok":true,"noop":false,"changed":["spec.maxReplicaCount"],"warnings":[],"resourceVersion":"..."}` |

The preview and PUT body is the creation body plus `"resourceVersion"` from `GET .../edit`. `namespace`, `name`, `targetKind` and `targetName` are fixed: echoing them is fine, a different value is `400` naming the field (`"targetName cannot be changed ..."`), and a body `namespace` different from the path's is `400`. An omitted field keeps its current value (`updateMode`, `controlledResources`, `containers` when absent or `null`; `minReplicas` when absent; `maxReplicas`, `pollingInterval`, `cooldownPeriod` when `0`; `triggers` when absent); `"containers":[]` removes every container policy. Validation is exactly create's (the body is validated by building the object), including the trigger allow-list, the credential screen and the scale-to-zero rule; a PUT that changes a ScaledObject also checks every `authenticationRef` exists. Warnings are create's: restarting update modes in preview and PUT, the HPA conflict warnings in the PUT only.

Managed fields: VPA `spec.updatePolicy.updateMode` and `spec.resourcePolicy.containerPolicies` (replaced as a whole; reported per container as `spec.resourcePolicy.containerPolicies[<name>]`, `from`/`to` like `controlledResources=cpu,memory maxAllowed.cpu=2`); ScaledObject `spec.minReplicaCount`, `maxReplicaCount`, `pollingInterval`, `cooldownPeriod` and `spec.triggers` (replaced as a whole; reported per position as `spec.triggers[<i>]` with a one-line `type=... name=... <settings> authenticationRef=...`, so a reorder changes both positions). An unset field equal to its default (VPA `updateMode` `Auto`; KEDA 0/100/30/300) is not a change. Labels, annotations, finalizers, status and every other spec key are untouched: the patch is a JSON Patch whose first operation `test`s `metadata.resourceVersion`. A `resourceVersion` different from the live one, or a failed test operation, is `409` "the object changed since you opened it: reload it and apply your edit again"; a missing one is `400`. The preview `yaml` omits `managedFields` and the `kubectl.kubernetes.io/last-applied-configuration` annotation.

**Not editable** (`"editable":false` with `reasons`; preview and PUT are `400`): VPAs with a container policy `mode` or `controlledValues`, bounds other than cpu/memory, policies controlling different resources, other `resourcePolicy` keys, `recommenders`, `updatePolicy.evictionRequirements`, a `selector`, an `updateMode` outside Off/Initial/Recreate/Auto, or a `targetRef` that is not an `apps/v1` Deployment/StatefulSet/DaemonSet/ReplicaSet; ScaledObjects with `advanced` (HPA behavior, scaling modifiers, `restoreToOriginalReplicaCount`), `fallback`, `scaleTargetRef.envSourceContainerName` or a non-`apps/v1` `apiVersion`, a trigger type outside the allow-list, a setting key outside the type's allow-list, other trigger keys, `useCachedMetrics`, `metricType` on a trigger other than cpu/memory (or the deprecated `metadata.type`), or a `ClusterTriggerAuthentication`. **Credential-looking live settings are never returned**: a value with URL/host userinfo, or a key that looks like a credential, makes the trigger non-editable, the value is left out of `settings`, and the reason names the key only.

Roles and namespaces are create's: `GET .../edit` is a namespaced read (viewer, operator, admin); preview and PUT are operator/admin (viewers `403`). For namespace-restricted accounts the guard checks the path namespace on all three (`GET /api/v1/scaledobjects/other-ns/x/edit` and `PUT /api/v1/vpas/other-ns/x` are `403`) and the handler re-checks it. Audited as `autoscale.vpa_edit` / `autoscale.keda_edit` with detail `fields=<changed field names>` (never values); a no-op is not audited. The ClusterRole gains `patch` on VPAs and ScaledObjects.

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
| GET | `/api/v1/gateways?namespace=...` | List GatewayClass/Gateway/HTTPRoute objects (`controller`, `provider` e.g. `envoy`, `addresses`). Returns `[]` (not an error) when the Gateway API CRDs aren't installed. Prefer Envoy Gateway (`provider=envoy`) over Ingress NGINX. |
| GET | `/api/v1/accelerators` | NVIDIA GPU inventory: operator/device-plugin/DCGM, GPU nodes (product, sharing, MIG), GPU pods (incl. FailedScheduling), GPU ResourceQuotas, optional DRA counts, DCGM util snapshot, and `waste[]` GPU over-request hints when Prometheus is configured |
| POST | `/api/v1/accelerators/rightsizing/apply` | Lower `nvidia.com/gpu` requests on a Deployment/StatefulSet (`{namespace,kind,name,container?,dryRun}`). Operator/admin; audited as `accelerators.gpu_rightsizing` |
| GET | `/api/v1/dra` | Read-only DRA inventory (DeviceClass / ResourceSlice / ResourceClaim[/Template]) when `resource.k8s.io` is present |
| GET | `/api/v1/fleet/outage-narrative?cluster=&from=&state=` | Compose who-poked-it + recent agent runs for a fleet incident window |
| GET | `/api/v1/certificates` | Read-only cert-manager view (`cert-manager.io/v1`): Certificates (issuer, secret, Ready, notAfter, renewalTime, `daysToExpiry`, `expiryBucket` = expired/lt7d/lt30d/ok), Issuers/ClusterIssuers, and failing CertificateRequests/Orders with messages. `available:false` when cert-manager is absent. Needs get/list on cert-manager.io |
| GET | `/api/v1/admission-policies` | Native ValidatingAdmissionPolicy + Binding inventory when the API is present |
| POST | `/api/v1/helm/registry-credentials/cloud-token` | Refresh a short-lived OCI registry password via local CLI (`{provider:ecr\|gar\|acr, registry}`) — admin; write-only follow-up with registry-credentials |
| POST | `/api/v1/integrations/probe/matrix` | Data-plane reachability matrix: `{urls[], pods:[{namespace,name,container?}]}` → cells with ok/latency from each pod (or Axiom when pods empty). Cap 8 URLs × 12 pods |

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
| POST | `/api/v1/volumes` | Create a PVC: `{"namespace","name","size","storageClass","accessMode","sourcePVC"}` (`storageClass`/`accessMode`/`sourcePVC` optional; `sourcePVC` = CSI clone of that same-namespace claim) |
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
| POST | `/api/v1/velero/backups` | Create a Velero Backup: `{"namespace","name","includedNamespaces","ttl","storageLocation"}` (`includedNamespaces` defaults to `["*"]`; `ttl` / `storageLocation` optional). Object validated against upstream Velero CRD OpenAPI in unit tests |
| POST | `/api/v1/velero/schedules` | Create a schedule: `{"namespace","name","schedule","ttl"}` (`schedule` is a cron string, `ttl` is optional, e.g. `"720h0m0s"`) — builds the Velero `Schedule` object directly via the dynamic client, same as the other Velero/Flux/Argo access in this API |

## Helm, GitOps, ops

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/helm/releases` | Helm releases |
| GET | `/api/v1/helm/releases/{namespace}/{name}/history` | Release revision history |
| POST | `/api/v1/helm/releases/{namespace}/{name}/rollback` | Rollback (`{"revision": N}`) |
| POST | `/api/v1/helm/releases/{namespace}/{name}/uninstall` | Uninstall release |
| POST | `/api/v1/helm/install` | Install a chart: `{"namespace","releaseName","repoUrl","chartName","chartVersion","values"}` (`chartVersion`/`values` optional, `values` is raw YAML). Runs `helm install --repo <repoUrl>` as a background Job under Axiom's own ServiceAccount — the chart can only create resources that ServiceAccount is itself allowed to create. Install still takes the repo URL and chart name directly, the same as `helm install --repo` on the CLI; the [chart catalog](#helm-chart-catalog) below is how the UI finds them. For an OCI chart send `ociRef` (`oci://host[:port]/path/chart`, the catalog entry's `ref`) instead of `repoUrl`/`chartName`: the Job then runs `helm install <release> <ociRef> -n <ns> --create-namespace [--version v]` with no `--repo`. `ociRef` must use the `oci://` scheme with a DNS or IP host, lower-case path components and no userinfo, query, fragment, tag, whitespace or leading `-`; sending it together with `repoUrl` is `400`. `/helm/upgrade` accepts `ociRef` the same way. When `ociRef` belongs to a stored OCI repository that was added over plain http or with registry credentials, the Job also gets `--plain-http` or a mounted registry config — see [Installing from an OCI repository](#helm-chart-catalog) |
| POST | `/api/v1/helm/upgrade` | Upgrade an existing release: same body shape as install (`{"namespace","releaseName","repoUrl","chartName","chartVersion","values"}`) — runs `helm upgrade --repo <repoUrl>` under the same ServiceAccount constraints |
| GET | `/api/v1/gitops` | GitOps applications (Flux/Argo when present) |
| POST | `/api/v1/gitops/preview` | Render the objects a create would make, without touching the cluster — see [GitOps application creation](#gitops-application-creation-and-deletion) |
| POST | `/api/v1/gitops` | Create an Argo CD / Flux application (admin only) — see [GitOps application creation](#gitops-application-creation-and-deletion) |
| GET | `/api/v1/gitops/{kind}/{namespace}/{name}/delete-preview` | What deleting the application would do (cascade or not) — see [GitOps application creation and deletion](#gitops-application-creation-and-deletion) |
| DELETE | `/api/v1/gitops/{kind}/{namespace}/{name}?removeSource=&confirm=` | Delete an Argo CD / Flux application (admin only; `confirm=<name>` required when it cascades) — see [GitOps application creation and deletion](#gitops-application-creation-and-deletion) |
| GET | `/api/v1/gitops/{kind}/{namespace}/{name}/edit` | The application's current editable fields — see [GitOps application creation and deletion](#gitops-application-creation-and-deletion) |
| POST | `/api/v1/gitops/{kind}/{namespace}/{name}/edit/preview` | Managed-field diff of an edit (admin only) |
| PUT | `/api/v1/gitops/{kind}/{namespace}/{name}` | Edit an Argo CD / Flux application in place (admin only, `resourceVersion`-guarded) |
| GET / POST | `/api/v1/gitops/credentials` | List / create write-only GitOps repository credentials (admin only) |
| DELETE | `/api/v1/gitops/credentials/{engine}/{namespace}/{name}` | Delete an Axiom-created repository credential (admin only) |
| POST | `/api/v1/gitops/{kind}/{namespace}/{name}/sync` | Force a re-sync. `kind` is one of `Application` (Argo — patches the top-level `operation.sync` field, the same mechanism `argocd app sync` uses) or `Kustomization`\|`HelmRelease` (Flux — bumps the `reconcile.fluxcd.io/requestedAt` annotation, the same convention `flux reconcile` uses) |
| GET | `/api/v1/alerts?namespace=...` | Alertmanager / Prometheus alerts |
| GET | `/api/v1/audit` | Axiom audit log (JSON array; the Ops → Audit tab exports the filtered view as CSV or JSON) |
| GET | `/api/v1/rbac?namespace=...` | Roles and bindings |
| POST | `/api/v1/rbac/roles` | Create a namespaced Role: `{"namespace","name","rules":[{"apiGroups","resources","verbs"}]}` |
| DELETE | `/api/v1/rbac/roles/{namespace}/{name}` | Delete a Role |
| POST | `/api/v1/rbac/rolebindings` | Create a namespaced RoleBinding: `{"namespace","name","roleName","subjects":[{"kind","name","namespace"}]}` |
| DELETE | `/api/v1/rbac/rolebindings/{namespace}/{name}` | Delete a RoleBinding |
| GET | `/api/v1/cost?window=30d` | Monthly cost — `namespaces` (by namespace) and `workloads` (by owning Deployment/StatefulSet/etc.), plus `totalMonthlyUsd`, `cpuRatePerCore`, `memoryRatePerGi`, `gpuRatePerGpu` (estimate path), `source` (`estimate` or `opencost`), `window`, `idleMonthlyUsd`, `unallocatedMonthlyUsd` (OpenCost only, omitted when 0), `opencostConfigured` / `opencostReachable` / `opencostError`, and `note`. Estimate prices `nvidia.com/gpu` (+ MIG) at `gpuRatePerGpu` (default 100, `AXIOM_COST_GPU_RATE`). `window` is `7d`, `14d` or `30d` (default); anything else is `400`. See *Actual cost from OpenCost* below |
| POST | `/api/v1/gitops/{kind}/{namespace}/{name}/suspend` | Suspend / resume: `{"suspend":bool,"resourceVersion"}` → `{"ok","changed"}`. Flux sets `spec.suspend`; Argo Application clears/sets `spec.syncPolicy.automated`. **Admin only.** ApplicationSet → `422`. Audited as `gitops.suspend` / `gitops.resume` |
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
| GET | `/api/v1/cost/invoice` | **Experimental.** The cloud invoice for the previous and the current month next to the allocated cost: `{"provider":"aws\|azure\|gcp","configured","currency","allocatedCurrency":"USD","allocatedSource":"estimate\|opencost","months":[{"month":"YYYY-MM","invoiceTotal","allocated","unallocated","asOf","partial"}],"experimental":true,"demo","error","note"}`. Always `200`; failures are in `error`. See *Actual bill (experimental)* below |

Reads are open to every role except namespace-restricted accounts (`403`: `/api/v1/cost/budgets` and `/api/v1/cost/rightsizing` are restricted-sensitive prefixes — cluster-wide cost data; `GET /api/v1/cost` keeps its existing behaviour). Budget writes are admin-only, so operators and viewers get `403`. Responses carry `Cache-Control: no-store`. Budgets live in ConfigMap `zyvor-axiom-cost-budgets` (key `budgets.json`) in the kubernetes build and in memory in demo mode. Over-budget state is computed whenever the budgets are read, and also by a background budget watcher (every `AXIOM_BUDGET_CHECK_INTERVAL`, default 5 min, against the default active cluster) that audits crossings as `cost.budget_warning` (ok → warning), `cost.budget_over` (→ over) and `cost.budget_ok` (back to ok), user `budget-watcher`, detail `$X of $Y (Z%)`, and sends a notification for each (see *Notifications*). Over → warning is remembered but not announced.

**Applying a recommendation.** `POST /api/v1/cost/rightsizing/apply` never takes target numbers from the client: the server recomputes the recommendation for exactly that container with the same pipeline and `window` as the GET (default `7d`), then patches the workload's pod template (strategic merge, field manager `zyvor-axiom`) so the controller rolls its pods as usual (`rollout` says what to expect, e.g. a paused Deployment or an `OnDelete` update strategy). Only `resources.requests` of the chosen resources of that container change; other containers and fields are untouched. A limit is never lowered and never added: when an existing limit is below the new request it is raised to the request and reported as a `limits` change with `"note":"limit raised to stay >= the request"`. A missing request is set (`from` is `"none"`). `dryRun:true` returns the same `changes` and writes nothing — the UI always dry-runs first. `observed` carries the requests the caller saw (the GET's `cpu.requestMilli` / `memory.requestMiB`, both optional); if either differs from the live value the call is refused with `409` "the live requests changed since you looked". A container whose verdict is `ok`, or whose chosen resources already match, is `409` "already right-sized". Raising requests is allowed; ResourceQuota is enforced when the controller creates the new pods, so a quota refusal at patch time surfaces as `409` with the API server's message. Viewers (even for a dry run, which is a POST) and namespace-restricted accounts get `403`. In demo mode the apply is kept in memory, so the next GET reports the container as `ok`.

**Rightsizing sources.** One source is chosen per call and reported in `source`: `prometheus` when `AXIOM_PROMETHEUS_URL` is set — p95 CPU (`quantile_over_time(0.95, rate(container_cpu_usage_seconds_total[5m])[<window>:5m])`) and p99 working-set memory (`quantile_over_time(0.99, container_memory_working_set_bytes[<window>])`) per namespace/pod/container, two instant queries per call, `basis` `p95`/`p99`, `confidence` `high` for windows of 7 days or more, else `medium`; otherwise `metrics-snapshot`, one metrics.k8s.io sample (`basis` `current`, `confidence` `low`: one sample is not a profile); `none` when neither answers (empty `items`; `note` explains how to enable Prometheus). A Prometheus failure (unreachable, timeout, non-200, malformed or empty answer) never fails the call: `note` says why and the snapshot is used; the Prometheus URL and token never appear in a response. Pod samples map to workloads by owner reference for running pods and by the standard pod-name suffix rules for pods that are gone; a container's usage is the highest across its replicas, and its requests come from the newest running pod. Prometheus applies to the cluster Axiom started on only; clusters registered later use the snapshot. Demo mode serves fixed `metrics-snapshot` data.

**Rules.** Recommended CPU = usage × 1.2, rounded up to 10m (min 10m); memory = usage × 1.15, rounded up to 8 MiB (min 16 MiB). `verdict` is `under` when usage exceeds 90 % of the request for either resource (throttling / OOM risk), or when a request is missing ("no request set"); `over` when a request is more than 1.5× its recommendation and the gap is at least 50m CPU or 64 MiB memory; otherwise `ok`. `monthlyDeltaUsd` = (recommended − request) priced with the `GET /api/v1/cost` request rates (also when OpenCost is configured: a saving prices a change of requests, not a bill), times `replicas` (negative = savings); `summary.monthlySavingsUsd` sums the savings of the `over` rows, and `overProvisioned`/`underProvisioned` count container rows. Items are sorted by savings (most first), then namespace / workload, and capped at 500 (the summary covers every row). Recommendations are never applied.

**Actual bill (experimental).** `GET /api/v1/cost/invoice` compares the cluster's allocated cost with what the cloud provider actually invoiced, configured by an admin in Settings (see *Integrations* below and DEPLOYMENT.md). Not configured, it answers `{"configured":false,"months":[],"experimental":true}` and the Cost page hides the card. Configured, it asks the provider for the previous calendar month and the current month to date (UTC): AWS Cost Explorer `GetCostAndUsage` (`UnblendedCost`, `MONTHLY`), Azure Cost Management `query` (`ActualCost`, `Monthly`, subscription scope) or a fixed parameterised BigQuery query over the GCP billing export (cost plus credits per `invoice.month`), optionally filtered by one tag / label (`billingTagKey` = `billingTagValue`). `invoiceTotal` is that amount in `currency`. `allocated` is the current 30-day run-rate from `GET /api/v1/cost` (`allocatedSource` says OpenCost or the request estimate) prorated to the days the month covers (whole days for the previous month, elapsed time for the current one) — Axiom keeps no allocation history, so the previous month is an approximation too. `unallocated` = `invoiceTotal − allocated` (idle capacity, other services and accounts); it is `null` when the invoice is not in USD (the allocation's currency). `partial` marks the current month. Invoice answers are cached for one hour per configuration (a failure for five minutes); a settings change is a new cache entry. `error` only ever holds Axiom's own text plus the provider's error code (e.g. `AWS Cost Explorer answered HTTP 403 (AccessDeniedException)`), never a URL, credential or response body. Same read policy as budgets: every role except namespace-restricted accounts (`403`). Demo mode returns deterministic canned figures (`demo:true`). The three clients have been tested **only against httptest fixtures** shaped after the providers' documented requests and responses (and the SigV4 signer against AWS's published example); they have **not been verified against real AWS, Azure or Google Cloud accounts**.

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
**Editing in place.**

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/gitops/{kind}/{namespace}/{name}/edit` | `200` the creation body shape filled from the live object, plus `resourceVersion`, `sourceObject` (`{"kind","namespace","name"}` of the Flux source, `null` for Argo CD), `sourceResourceVersion`, `editable`, `reasons` and `readOnly`. Admin, operator and viewer |
| POST | `/api/v1/gitops/{kind}/{namespace}/{name}/edit/preview` | `200 {"changes":[{"object":"Application/argocd/shop","field":"spec.source.targetRevision","from":"main","to":"v2"}],"yaml":"<the object(s) after the change>","warnings":[...],"noop":false}`. Changes nothing. **Admin only** |
| PUT | `/api/v1/gitops/{kind}/{namespace}/{name}` | Apply the edit: `200 {"ok":true,"noop","changes":[...],"warnings":[...],"resourceVersion":"<new>"}`. **Admin only** |

The preview and PUT take the creation body (same field rules, same validation, same warnings) plus `"resourceVersion"` from the GET (required) and optionally `"sourceResourceVersion"`; `engine`, `name`, `namespace` and `kind` may be echoed but must match the object (`400 name cannot be changed …`), and an Argo CD application cannot switch between a git `path` and a Helm `chart` source (`400 source.chart: …`). Omitted `project`, `destination.server`, `interval` and (git) `revision` keep their current value; the other fields mean what they mean on create (an empty `values` removes the values, an empty `secretRef` removes the reference). The server re-reads the object, and a `resourceVersion` (or `sourceResourceVersion`) that no longer matches is `409 the application changed since you opened it`.

Only the fields the typed model manages are written — Argo CD `spec.project`, `spec.source.{repoURL,path,chart,targetRevision,helm.values}`, `spec.destination.{server,namespace}`, `spec.syncPolicy.automated{prune,selfHeal}` and the `CreateNamespace=true` entry of `syncOptions` (other entries are kept); Kustomization `spec.{path,interval,prune,targetNamespace}`; HelmRelease `spec.{interval,chart.spec.chart,chart.spec.version,targetNamespace,install.createNamespace,values}` (values are replaced as a whole) — as a JSON Patch whose first operation tests `metadata.resourceVersion`, so labels, annotations, finalizers, status and any spec field the form does not show are left exactly as they are. For Flux the source fields (`repoUrl`, `revision`, `secretRef`, and `interval` when it changes) are patched on the `GitRepository`/`HelmRepository` first — but only when it carries the managed-by label and no other Kustomization/HelmRelease references it; otherwise they are listed in `readOnly` with a reason and changing one is `400 source.repoUrl cannot be changed …` (sending the current value or omitting it is fine).

`editable:false` (with `reasons`; the PUT is then `400 … cannot be edited here`) when the object uses something the typed model cannot represent: Argo CD `spec.sources`, an ApplicationSet owner, `destination.name`, `source.kustomize`/`directory`/`plugin`, `helm.valuesObject`/`parameters`/`fileParameters`/`valueFiles`, Helm settings on a git path; Kustomization `patches*`, `postBuild`, `components`, `images`, `namePrefix`/`nameSuffix`, `kubeConfig` or a non-`GitRepository` source; HelmRelease `chartRef`, `valuesFrom`, `postRenderers`, `kubeConfig`, `chart.spec.valuesFiles` or a non-`HelmRepository` source. Objects labelled by a parent (`argocd.argoproj.io/instance`, `kustomize.toolkit.fluxcd.io/name`) get a warning that the parent may revert the edit. `404` unknown object, `422` engine not installed, `403` for operators and viewers (the GET excepted) and namespace-restricted accounts. A successful, non-noop PUT is audited as `gitops.edit` with target `<kind>/<namespace>/<name>` and detail `fields=<changed field paths>` (never values).

**Repository credentials (write-only).**

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/gitops/credentials?engine=argocd\|flux&namespace=` | `200 [{"name","engine","kind":"git\|helm\|oci","url","authType":"basic\|token\|ssh","namespace","managedByAxiom","age"}]` — never any secret data |
| POST | `/api/v1/gitops/credentials` | Create: `201 {"ok":true,"name","namespace","engine"}` |
| DELETE | `/api/v1/gitops/credentials/{engine}/{namespace}/{name}` | `200 {"ok":true,"engine","namespace","name"}` |

```json
{ "engine": "argocd | flux", "name": "repo-creds", "namespace": "flux-system", "kind": "git | helm | oci",
  "url": "https://github.com/org/repo.git", "authType": "basic | token | ssh",
  "username": "bot", "password": "…", "token": "…",
  "sshPrivateKey": "-----BEGIN OPENSSH PRIVATE KEY-----…", "knownHosts": "github.com ssh-ed25519 AAAA…" }
```

All three routes are **admin only**, the listing included (it reveals which repositories are used); operators, viewers and namespace-restricted accounts get `403`. The listing never enumerates arbitrary Secrets: for Argo CD (namespace default `argocd`) only Secrets labelled `argocd.argoproj.io/secret-type: repository` or `repo-creds`, for Flux (default `flux-system`) only Secrets labelled `app.kubernetes.io/managed-by: zyvor-axiom` + `axiom.zyvor.dev/purpose: gitops-credentials`; without `engine` both engines are listed in their default namespaces (`namespace` requires `engine`). Rows carry the URL (Argo CD's non-secret `url` key, or Axiom's `axiom.zyvor.dev/repo-url` annotation), kind and auth type, never a key value. The UI offers these names for a Flux application's `source.secretRef` (same namespace); Argo CD picks repository Secrets up by URL on its own.

Creation rules: `name` and `namespace` are DNS-1123 labels; `kind` defaults to `git`; the URL follows the creation rules (no credentials in it, no query or fragment) and must match the auth type and kind (`ssh` needs an `ssh://` URL, or for Argo CD scp-like `git@host:org/repo`; `basic`/`token` need `https`/`http`; `oci` needs `oci://`); `basic` needs `username` + `password`; `token` needs `token` (`username` optional, default `git`); `ssh` needs `sshPrivateKey` (an unencrypted PEM/OpenSSH private key that parses) and, for Flux, `knownHosts` (known_hosts lines; Argo CD keeps known hosts in `argocd-ssh-known-hosts-cm`, so it is refused there); fields the auth type does not use must be empty; each value is at most 16 KiB (username 256 bytes) without control characters; the body is at most 64 KiB (`413`), unknown fields are rejected, and a malformed body gets a generic `400` that quotes nothing. Flux Secrets are `Opaque` with `username`/`password` (a token is the password) or `identity`/`known_hosts`; Argo CD Secrets carry `argocd.argoproj.io/secret-type: repository` and the keys `type` (`git`/`helm`; OCI is `helm` + `enableOCI: "true"` with `oci://` removed from `url`), `url`, `name`, and `username`/`password` or `sshPrivateKey`. Both also carry the managed-by and purpose labels and non-secret `axiom.zyvor.dev/repo-url|repo-kind|auth-type` annotations. `409` when a Secret of that name exists — it is never read or overwritten. There is no update: **to rotate a credential, delete it and create it again**. DELETE removes only Secrets Axiom labelled itself (Flux: managed-by + purpose; Argo CD: the repository label and the managed-by label), otherwise `409 … not created by Axiom …`; `404` when unknown. Secret values never appear in a response, an error message, the log or the audit trail: creation is audited as `gitops.credential_create` (target `<engine>/<namespace>/<name>`, detail `authType=…`) and deletion as `gitops.credential_delete`. In demo mode credentials are kept in memory and no secret value is stored at all.

### Helm chart catalog

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/helm/repos` | Chart repositories: `[{"name","url","kind","charts","addedAt","chartCount","fetchedAt","error","credentialsSecret","authenticated","plainHttp"}]` (`kind` is `index` or `oci`; `charts` only for an OCI repository added with an explicit list; `credentialsSecret` is the NAME of the [registry credential](#oci-registry-credentials) the repository uses, `""` for none, and `authenticated` says whether it uses one — nothing else about the credential is shown; `plainHttp: true` only for an OCI registry that answered over plain http when it was added). `chartCount`/`fetchedAt`/`error` come from the cached index; this call never fetches |
| POST | `/api/v1/helm/repos` | Add a repository: `{"name","url"}` → `201` with the repository object. **Admin only.** `name` is a DNS-1123 label of at most 40 characters; `url` must pass the fetch rules below and must not carry userinfo, a query string or a fragment (a trailing `/` is dropped). The index is fetched once and must parse (`400` with the reason otherwise, including an index over 20 MiB); `409` for a duplicate name or URL; at most 20 repositories. Audited as `helm.repo_add` |
| DELETE | `/api/v1/helm/repos/{name}` | Remove a repository and its cached index → `{"ok":true}`; `404` if unknown. **Admin only.** Audited as `helm.repo_remove` |
| GET | `/api/v1/helm/charts?repo=&q=&limit=&refresh=` | Search: `{"charts":[{"repo","name","version","appVersion","description","icon","home","keywords","deprecated","created","kind","ref"}],"total","fetchedAt","errors":{"<repo>":"msg"}}`. One entry per chart — its latest non-deprecated, non-prerelease version (the newest one if every version is deprecated or a prerelease). `repo` omitted = every repository; a repository that cannot be fetched is reported in `errors` and does not fail the call (its last good copy is still served). `q` is a case-insensitive substring over name, description and keywords, ranked exact name > name prefix > name contains > description/keyword, then by name. `limit` defaults to 60, max 200; `total` counts every match. `fetchedAt` is the oldest index among the answers. `refresh=true` re-fetches, at most once per repository every 30 s. `404` for an unknown `repo` |
| GET | `/api/v1/helm/charts/{repo}/{chart}` | `{"repo","name","kind","ref","description","home","sources","keywords","icon","maintainers":[{"name","email"}],"versions":[{"version","appVersion","created","deprecated"}]}` — metadata from the latest version, up to 100 versions newest first (semver order; prereleases below their release; unparseable versions last). `404` for an unknown repository or chart |
| GET | `/api/v1/helm/charts/{repo}/{chart}/values?version=` | `{"version","values","readme"}`: the chart's default `values.yaml` and `README.md` (truncated to 64 KiB; may be empty). `version` defaults to the latest. Downloads the archive the index names (`urls[0]`, relative to the repository URL) and reads only `<chart>/values.yaml` and `<chart>/README.md` from it in memory. `404` for an unknown version, `413` "chart archive too large" (archive over 20 MiB or `values.yaml` over 2 MiB), `502` when the repository or archive cannot be fetched or the index points somewhere the fetch rules refuse |

Reads are open to every role except namespace-restricted accounts (`403`: `/api/v1/helm/repos` and `/api/v1/helm/charts` are restricted-sensitive prefixes); writes are admin-only, so operators and viewers get `403`. Responses carry `Cache-Control: no-store`.

**OCI repositories.** A registry namespace such as `oci://ghcr.io/org/charts` can be added as a repository, each chart being the registry repository `<namespace>/<chart>` and each version a tag: `POST /api/v1/helm/repos` with `{"name","url":"oci://host[:port]/path","charts":["nginx","redis"]}` (`"kind":"oci"` is implied by the scheme; `"kind":"index"` with an `oci://` URL, or `charts` on an index repository, is `400`). The URL takes a DNS name or IP host, lower-case path components, no userinfo, query or fragment; `charts` holds at most 50 distinct names matching `[a-z0-9]([-._a-z0-9]*[a-z0-9])?`. Without `charts` the registry's `/v2/_catalog` is used (charts directly under the namespace, at most 50) — most hosted registries (ghcr.io, Docker Hub) do not offer it, and the add then fails telling you to list the charts. The add pings `GET /v2/` and reads every listed chart (tags, the newest release's manifest and config blob), so a wrong URL, a missing chart or a registry that needs credentials fails with `400` and the reason. Without credentials, a bearer challenge (`WWW-Authenticate: Bearer realm=…,service=…`) is answered by fetching an anonymous token for `repository:<name>:pull` (the token is never logged or returned); a `Basic` challenge or a refused anonymous token fails with `this registry needs credentials: add them under Helm → Registries`. A private registry is added with `"credentialsSecret":"<name>"`, the name of a stored [registry credential](#oci-registry-credentials): it must exist and its `registry` must equal the repository URL's host[:port] (`400 credentials are for registry X` otherwise; `credentialsSecret` on an index repository is `400`). In the chart endpoints an OCI chart's versions are its tags that parse as semver (`latest`, `main`, signature tags and the like are ignored; `_` in a tag reads as `+`), at most 500 tags per chart; `version` is the newest non-prerelease. Metadata (`description`, `appVersion`, `keywords`, `home`, `icon`, `maintainers`, `deprecated`) is the Helm config blob (`application/vnd.cncf.helm.config.v1+json`) of that version, so older versions carry no `appVersion`. Every chart entry carries `kind` (`index`/`oci`) and `ref` (`oci://host/path/chart` for OCI, `""` for index charts) — `ref` is what `POST /api/v1/helm/install` takes as `ociRef`. `…/values?version=` reads the manifest of that tag (OCI image manifest or Docker manifest v2) and downloads only the chart layer (`application/vnd.cncf.helm.chart.content.v1.tar+gzip`, at most 20 MiB) through the same archive reader. Every blob's sha256 digest and size are checked against the manifest while it streams and a mismatch is refused (`502`); manifests are capped at 4 MiB, config blobs at 1 MiB, tag and catalog listings at 20 MiB over all pages (`Link: rel="next"` pagination, same host and path only), token responses at 64 KiB. The registry is reached over `https://`; with `AXIOM_HELM_ALLOW_INSECURE_REPOS=true` a registry that does not speak TLS is retried over `http://`, and the repository is then stored with `plainHttp: true`.

**Installing from an OCI repository.** `POST /api/v1/helm/install` and `/helm/upgrade` with an `ociRef` look the reference up among the stored OCI repositories: same registry host[:port], and the repository's path a prefix of the reference's path at a `/` boundary (`oci://ghcr.io/org` matches `oci://ghcr.io/org/charts/nginx`, not `oci://ghcr.io/orgx/nginx`); the longest matching path wins. No match, or a match without credentials over https, runs exactly the anonymous Job described above. A match with `plainHttp` adds `--plain-http` (only then). A match with credentials makes the Job pull with them: a short-lived Secret `<job>-registry` holding a Docker-style `config.json` (`{"auths":{"<registry>":{"auth":"<base64 user:pass>"}}}`) is created next to the Job, owned by it (deleted with it when its 10-minute TTL expires, and removed at once if the Job cannot be created), mounted read-only at `/registry`, and `HELM_REGISTRY_CONFIG=/registry/config.json` is set (Helm 3.8+). The password is never in the Job's args or env and never in the audit entry, whose detail is `<ociRef> registry-credential=<name>`. A matched repository whose credential has been deleted, or no longer matches its registry, is `409`. The demo provider accepts the request and ignores the credentials.

**Fetch rules.** The server fetches URLs an administrator typed and URLs a repository index names, so every request is checked: `https://` only (`http://` with `AXIOM_HELM_ALLOW_INSECURE_REPOS=true`); no userinfo; no numeric hosts that are not dotted quads (`2130706433`, `0x7f000001`, `127.1`); and the host's resolved addresses must all be public — loopback, `localhost`, link-local (`169.254.0.0/16` including cloud metadata, `fe80::/10`), private (`10/8`, `172.16/12`, `192.168/16`, `fc00::/7`), CGNAT (`100.64/10`), unspecified, multicast and broadcast are refused unless `AXIOM_HELM_ALLOW_PRIVATE_REPOS=true` (for an in-cluster ChartMuseum or Harbor). The check runs at connect time on the address actually dialed, so a DNS answer that changes between check and connect cannot bypass it, and it applies to every redirect (at most 3, each re-validated) and to chart archive URLs on other hosts. The same rules cover OCI registries: every registry request, every blob redirect (signed storage URLs included) and the bearer-token realm, which must be on the registry's own host or be a public `https://` host (a realm on another host is held to the public rule even when private repositories are allowed); `Authorization` is never carried across a redirect. No proxy or cookies are used, and no credentials except a stored [registry credential](#oci-registry-credentials), sent only as described there; each request has a 15 s timeout and the index is capped at 20 MiB. Parsed indexes are cached in memory for 10 minutes per repository (one fetch at a time per repository; a failing repository is retried at most every 30 s), and at most 5,000 charts are read from one index. The repository list is kept in ConfigMap `zyvor-axiom-helm-repos` (key `repos.json`) in the kubernetes build and in memory in demo mode, where it starts empty.

### OCI registry credentials

Write-only usernames and passwords (or access tokens) for private OCI chart registries, used by the [chart catalog](#helm-chart-catalog) and by OCI installs. **Admin only for all three routes, the listing included**: operators and viewers get `403`, and namespace-restricted accounts are refused (`/api/v1/helm/registry-credentials` is a restricted-sensitive prefix).

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/helm/registry-credentials` | `[{"name","registry","username","age","inUse":["<repo>",…]}]` — never the password. `inUse` lists the chart repositories that reference the credential |
| POST | `/api/v1/helm/registry-credentials` | `{"name","registry","username","password"}` → `201 {"ok":true,"name","registry","username"}`. `409` if the name exists (an existing credential is never overwritten or read) |
| DELETE | `/api/v1/helm/registry-credentials/{name}` | → `{"ok":true,"name","registry"}`; `409 … remove or edit repository X first` while a repository references it; `404` if unknown |

Rules: `name` is a DNS-1123 label (at most 63 characters); `registry` is `host[:port]` only — a DNS name or IP literal with an optional port, no scheme, path, userinfo, query or whitespace — and is stored lower-cased; `username` (at most 256 bytes) must not contain whitespace, control characters or `:`; `password` is required, at most 4 KiB, without control characters or newlines; at most 50 credentials. The body is read once through a 16 KiB limit (`413` beyond), unknown fields are rejected, and a malformed body gets a generic `400` that quotes nothing. There is no update: **to rotate a credential, delete it and create it again** (remove the repositories that use it first, or add a second credential and re-add them). Deleting a repository never deletes its credential. All credentials live in one Secret, `zyvor-axiom-helm-registry-creds` (key `credentials.json`), in Axiom's namespace, written with conflict-safe updates; in demo mode they are kept in memory. Creation and deletion are audited as `helm.registry_credential_create` / `helm.registry_credential_delete` with the name as target and `registry=<host>` as detail. The password never appears in a response, an error message, the log or the audit trail.

**Where the password goes.** Only to the registry host named in `registry`: after a `401` with a `Basic` challenge from that host the request is retried with HTTP Basic (and Basic is then sent up front to that host); after a `401` with a `Bearer` challenge the token is requested from the realm with HTTP Basic — but only when the realm is the registry host itself or a host of the same registrable domain by the public suffix list (`auth.docker.io` for `registry-1.docker.io`, `gitlab.com` for `registry.gitlab.com`). A realm on any other host is refused with `… a different site: refusing to send the registry credentials there`. Redirects (blob storage included) never carry an `Authorization` header. Tokens obtained this way are cached in memory per registry, scope and credential (keyed by a hash) for their `expires_in` — 60 s when absent, at most 5 minutes. A wrong password fails with `rejected the registry credentials: check the username and password`.

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
| GET | `/api/v1/security/policy-coverage?namespace=...` | Kyverno/Gatekeeper PolicyReport rollup by `policy` (+ `rule`): pass/fail/warn/error counts and `failRate` |
| GET | `/api/v1/security/reports/{namespace}/{kind}/{name}` | Drill-down for one workload: `vulnerabilities[]` (sorted by severity, de-duplicated, capped at 300), `misconfigurations[]`, `policyResults[]` (failing/warning only), `images` |

Read-only aggregation of Trivy Operator `VulnerabilityReport` / `ConfigAuditReport` (`aquasecurity.github.io`) and `wgpolicyk8s.io` `PolicyReport` / `ClusterPolicyReport` (Kyverno, Gatekeeper-compatible) objects through the dynamic client. Axiom runs no scanner. Missing CRDs are not an error: `available` is `false`, each source says why (`CRD not installed`, or an RBAC hint on `Forbidden`) and `installHint` explains what to install. Trivy reports a Deployment's scan against its current ReplicaSet; the pod-template-hash suffix is stripped so the row shows the Deployment. Scoped like other namespaced lists: namespace-restricted accounts must pass `?namespace=` for a namespace they hold (cluster-scoped `ClusterPolicyReport` results appear only in the all-namespaces view). Reports are paged (200 per page, 10,000 per source).

## Agent runs and platform intents

Axiom ships a built-in ProposeOnly planner (BYO LLM or deterministic). Runs persist in the `platform.example.com` AgentRun CRD when installed, otherwise in ConfigMap `zyvor-axiom-agent-runs`. Approve of an Axiom-owned run executes allowlisted actions; approve/deny refuse when the run is not pending.

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/v1/agent-runs?namespace=...` | List agent runs |
| POST | `/api/v1/agent-runs` | Create ProposeOnly agent run (body: `goal`, optional `namespace`, `name`, `target`, `dryRun`, `source`, `extraContext`) then plan |
| GET | `/api/v1/agent-runs/{namespace}/{name}` | Agent run detail |
| POST | `/api/v1/agent-runs/{namespace}/{name}/approve` | Approve (and execute if Axiom-owned) |
| POST | `/api/v1/agent-runs/{namespace}/{name}/deny` | Deny run |
| GET | `/api/v1/platform-intents?namespace=...` | Platform intent CRs |
| GET | `/api/v1/platform-intents/drift` | Compare place-intent annotations vs Platform Intent CRs vs ready/desired (drift kinds: `annotation-only`, `missing-annotation`, `replicas`) |
| GET | `/api/v1/mcp` | MCP server info |
| GET | `/api/v1/mcp/tools` | MCP tool list |
| POST | `/api/v1/mcp` | MCP JSON-RPC 2.0 (`initialize`, `tools/list`, `tools/call`) |
| GET | `/api/v1/metrics/history` | Prometheus CPU/memory history (`?namespace=&name=&kind=pod\|node\|workload&window=1h\|6h\|24h`) |
| GET | `/api/v1/alertmanager/status` | Whether Alertmanager is configured |
| GET | `/api/v1/alertmanager/alerts` | Firing Alertmanager alerts |
| GET | `/api/v1/alertmanager/silences` | List silences |
| POST | `/api/v1/alertmanager/silences` | Create a silence |
| DELETE | `/api/v1/alertmanager/silences/{id}` | Expire a silence |
| GET | `/api/v1/upgrade-risk` | Lite cluster upgrade readiness report |
| POST | `/api/v1/upgrade-risk/agent-run` | Create a ProposeOnly agent run from the upgrade-risk report |

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
| POST | `/api/v1/integrations/billing/test` | **Experimental.** Read the invoice totals once with the saved billing settings, bypassing (and then refreshing) the cache → `{ok, service:"billing", latencyMs, detail:{provider, months:[{month,total,currency}]}, error, hint}` (admin only; audited as `integrations.billing_test`) |

`PUT /api/v1/integrations` also accepts `slackWebhookUrl`, `alertWebhookUrl`, and `notifyOnWarnings` (bool). When `notifyOnWarnings` is true and at least one webhook is set, a background dispatcher polls `GET /api/v1/alerts`-equivalent Warning events every 60s and forwards new ones to both configured channels.

**Notifications.** The same channels and switch carry Axiom's own notifications: a cluster becoming unreachable or degraded and recovering (from the fleet prober), and a cost budget reaching `warning`, `over` or back to `ok` (from the budget watcher). Slack receives three lines — `:warning: *Axiom: cluster eu-west-1 is unreachable*`, the one-line detail, and ``In Axiom: `/fleet` ``; the generic webhook receives `{"source":"zyvor-axiom","kind":"fleet.cluster_unreachable","severity":"warning","title","detail","path","time"}` where `kind` is the matching audit action and `path` is an Axiom-relative UI path (`/fleet`, `/ops/cost`). URLs inside a detail are cut down to scheme and host. A cluster or namespace is not re-announced in the same state within 10 minutes; a recovery / back-to-ok message is sent whenever the problem was announced; conditions present when Axiom starts are not announced. Delivery is best effort: one attempt per channel, failures are logged with the webhook URL redacted, and each replica sends its own.

`PUT /api/v1/integrations` also accepts the AI assistant fields `aiEnabled` (bool), `aiProvider` (`openai-compatible` | `anthropic` | `ollama`), `aiBaseUrl` (http/https only, no embedded credentials, no query), `aiModel`, `aiRedact` (bool, default `true`), `aiApiKey` (write-only) and `aiApiKeyClear`. `GET` returns `aiApiKeySet` instead of the key. Invalid scheme/provider is a `400`.

`PUT /api/v1/integrations` also accepts the **experimental** billing reconciliation fields: `billingProvider` (`aws` | `azure` | `gcp`, empty = off), `billingTagKey` / `billingTagValue` (optional; at most 128 / 256 characters, no quotes or control characters), `billingAwsRegion` (`us-east-1` or `cn-northwest-1`, the two Cost Explorer endpoints), `billingAzureTenantId` (GUID or domain), `billingAzureClientId` and `billingAzureSubscriptionId` (GUIDs), `billingGcpProjectId` (optional query project, defaults to the table's) and `billingGcpTable` (`project.dataset.table`: a lowercase project ID, then letters, digits and underscores only — it is the one identifier in the SQL and cannot be a parameter). Credentials are write-only: `billingAwsAccessKeyId`, `billingAwsSecretAccessKey`, optional `billingAwsSessionToken` (STS), `billingAzureClientSecret` and `billingGcpServiceAccountKey` (the service-account key JSON) are stored in the `zyvor-axiom-integrations` Secret, a blank value keeps the stored one, `…Clear: true` removes it, and `GET` only returns `billingAwsAccessKeyIdSet`, `billingAwsSecretAccessKeySet`, `billingAwsSessionTokenSet`, `billingAzureClientSecretSet`, `billingGcpServiceAccountKeySet` and `billingConfigured`. Malformed values are `400` before anything is stored; error messages never echo a credential. There is no endpoint field: each provider's API host is fixed. Live check: `e2e/scripts/validate-billing-live.sh`.

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
| DELETE | `/api/v1/users/{username}/mfa` | Admin only. Remove a person's second factor (and any pending enrolment) and revoke their sessions — for someone who has lost their device and recovery codes. A local account of that name is reset first; otherwise the directory (LDAP) account's record is removed (audit detail `ldap`). `404` only when neither exists. Audited as `user.mfa_reset` |
| GET | `/api/v1/users/external-mfa` | Admin only (operators and viewers get `403`). Directory (LDAP) accounts with a second factor or an enrolment in progress: `[{"username","enabled","updatedAt"}]`, sorted by username. Never any secret material |
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
