# Feature gaps (living)

Closed or substantially narrowed in the current tree (see CHANGELOG):

- SAML ACS sessions (`POST /api/v1/auth/saml/acs`) — the response's enveloped XML-DSig signature is verified against the pinned `AXIOM_SAML_IDP_CERT` (only the signed content is trusted, which defeats signature wrapping), and the audience (`AXIOM_SAML_ENTITY_ID`) and `NotBefore`/`NotOnOrAfter` window are enforced
- SAML `InResponseTo` and `SubjectConfirmationData` — `GET /api/v1/auth/saml/login` sends a real AuthnRequest with a random ID, recorded in a signed 5-minute `axiom_saml_req` cookie and in a single-use in-memory set; the ACS requires the signed `InResponseTo` to match both and consumes it, so a captured response cannot be replayed (not even in the same browser). Unsolicited (IdP-initiated) responses are rejected unless `AXIOM_SAML_ALLOW_IDP_INITIATED=true`. A `SubjectConfirmation` must use the bearer method, and its `SubjectConfirmationData` `Recipient` must equal this ACS URL and `NotOnOrAfter` must not have passed
- LDAP simple-bind via the local username/password form (`AXIOM_LDAP_*`)
- Operator role + safer OIDC default role env (`AXIOM_OIDC_DEFAULT_ROLE`)
- Fleet cluster register/unregister from kubeconfig (Settings → Clusters)
- HA: Helm fails when `replicaCount > 1` without `auth.sessionSecret`
- Audit ring default raised to 5000 (`AXIOM_AUDIT_RING_SIZE`)
- ConfigMap/Secret update; Velero on-demand backup; VM create/delete; PVC/Service/Ingress/Quota writes; Helm upgrade
- VM console returns proxied `wsPath` for browser VNC clients
- OIDC login validated end-to-end against a real Keycloak IdP (public client + PKCE, no client secret needed) — see `e2e/scripts/verify-sso.js`
- Enterprise write endpoints validated against a real cluster, not just the demo provider/fake clientsets — see `e2e/scripts/validate-enterprise-writes.sh`. Found and fixed one real RBAC gap: `DELETE /api/v1/vms/{ns}/{name}` was 403ing because the `zyvor-axiom` ClusterRole never granted `delete` on `kubevirt.io` VMs
- Login throttling is per client IP **and** per attempted account name (`internal/auth/ratelimit.go`); a valid login from another account can no longer reset the counter of an account being guessed. Trade-off: anyone who knows a username can lock that account for up to 5 minutes by failing logins
- OIDC ID tokens are verified against the provider's JWKS (algorithm allow-list, `kid`, signature, `iss`/`aud`/`azp`/`exp`/`sub`) and bound to the sign-in by a `nonce` cookie (`internal/api/oidc_jwks.go`)
- Browser WebSocket/EventSource auth uses single-use, 30-second tickets (`POST /api/v1/ws-ticket`) instead of credentials in the URL; the legacy `?access_token=`/`?axiom_session=`/`?api_key=` forms still work but are deprecated
- Audit log export as JSON as well as CSV
- TOTP two-factor sign-in for local accounts and the built-in admin (`POST /api/v1/auth/login`, `AXIOM_TOTP_SECRET`, enrolment and recovery codes from the user menu), with replay protection and throttling shared with passwords
- Server-side session revocation: logout ends that session, "sign out everywhere", automatic revocation on role/namespace/password changes and deletion, admin "sign out everywhere", and a configurable `AXIOM_SESSION_TTL`
- Read-only RBAC viewer, NetworkPolicy analysis and security-report (Trivy/PolicyReport) views
- LDAP group-based role mapping (`AXIOM_LDAP_ADMIN_GROUP`/`AXIOM_LDAP_VIEWER_GROUP`/`AXIOM_LDAP_GROUP_ATTR`) — only for the `AXIOM_LDAP_BASE_DN`-constructed bind form; AD-style NetBIOS/UPN binds still always get the `operator` default

Known security limitations (reviewed, not yet fixed):

- **Two-factor sign-in covers local accounts and the built-in admin only.** SSO accounts rely on the IdP's MFA, and LDAP directory accounts have none (the bind password alone signs in). Enrolment shows the setup key as text (no QR code), there is no "require MFA for everyone" switch, and a person who loses both their device and their recovery codes needs an administrator to reset it
- **Outstanding SAML AuthnRequests are in-memory per replica** (capped at 4096, oldest evicted first). With `replicaCount>1` the `/saml/login` redirect and the IdP's POST to `/saml/acs` must reach the same replica (session affinity), otherwise the sign-in fails with "unknown, expired or already used". The ACS URL is derived from the request (`X-Forwarded-Proto`/`X-Forwarded-Host`), so the proxy must pass the public scheme and host for the `Recipient` check to match. With `AXIOM_SAML_ALLOW_IDP_INITIATED=true`, unsolicited responses are bound only by their validity window and Recipient, not to a browser, and can be replayed within that window
- **Stream tickets are in-memory per replica.** With `replicaCount>1` a ticket is only redeemable on the replica that issued it, so WebSocket/SSE connections need session affinity (or a single replica); the login throttle counters are likewise per replica
- **The pod HTTP proxy (`/pods/{ns}/{name}/window/{port}/`) still accepts `?access_token=` for plain browser navigations**: it loads many sub-resources, so a single-use ticket cannot cover it
- **Session revocation is per replica.** Logout, sign-out-everywhere and account changes are stored in a ConfigMap and applied at once on the replica that handled them; other replicas load them at start-up, so with `replicaCount>1` a revoked session can stay valid there until it expires (use a short `AXIOM_SESSION_TTL`). A role change at an SSO identity provider is still only picked up at the next sign-in (an administrator can force that with "Sign out everywhere")
- **The pod "window" proxy serves pod content from Axiom's own origin.** It is now sandboxed (CSP), strips cookies and is operator-only, but a separate origin would be the real isolation.
- **LDAP over `ldap://` is plaintext** (no StartTLS); use `ldaps://`. `AXIOM_LDAP_BIND_DN`/`AXIOM_LDAP_BIND_PASSWORD` are read but unused.
- **Default credentials**: the Helm chart and manifest default to `demo`/`demo`. Axiom logs a warning outside demo mode, and no longer advertises them on the login page, but does not refuse to start.

Known platform limitations (reviewed, not yet fixed):

- **Audit persistence with several replicas**: each replica keeps its own ring and `Save` is an unconditional update, so the last writer wins. (A single replica is fine; the persisted ring is now trimmed to stay under the ConfigMap size limit.)
- **Generic manifest apply uses forced server-side apply**, which still takes field ownership from Argo CD/Flux/Helm — but the apply dialog now shows a diff first and warns when another manager owns the fields. "Validate" and the real apply can still disagree for Roles/RoleBindings/ServiceAccounts because of RBAC verbs.
- **Alert dispatch has no leader election**: with `replicaCount>1` every replica posts the same Slack alert, and a flapping pod re-fires every minute.
- **Blast-radius preview and resource detail match pods by name substring** (`web` also matches `web-api-*`), and the preview's Atlas volume list can never populate (`WorkloadVolumes` is not in the Provider interface).
- **`ListSecrets` still reads every Secret with its data**, and no list call is paginated (`Limit`/`Continue`), so very large clusters are slow.
- **Interactive terminals have no idle timeout or ping/pong**; a client that vanishes without closing leaves its PTY until the pod exits. (Concurrent sessions are now capped at 64 and frames at 1 MiB.)
- **Seal NetworkPolicy names omit the workload kind**, so a Deployment and StatefulSet with the same name collide; `Approve/DenyAgentRun` do not check the current state; `GetCRDInstance`/`DeleteCRDInstance` accept built-in groups (apps, batch, rbac).

Other known gaps found by the functional test pass (see `e2e/README.md`):

- Registered clusters are always reported `Healthy` (`internal/platform/switchable_provider.go`) — there is no probe
- `POST /api/v1/users` maps an unknown role to `admin` (`NormalizeRole`, documented as intentional; consider fail-closed)
- A saved Slack / webhook URL cannot be cleared from the UI (the API's `*Clear` flags have no control), and the Atlas token has no UI field
- `/api/v1/session` reports `role: "Platform Admin"` for operators and `auth: "oidc"` for SAML sessions (sessions from the local sign-in now correctly report `local`)
- `DELETE /api/v1/clusters/{unknown}` returns 400 rather than 404

Still thinner than a full private-cloud suite (intentionally or next):

- MFA for LDAP accounts, QR-code enrolment and a require-MFA policy
- Embedded noVNC UI (proxy path is ready; virtctl remains supported)
- GitOps Application create / Helm chart catalog browse
- Historical Prometheus TSDB / alert silence
- ClusterRole write UI (deliberate security boundary)
- SOC2 / pen-test packaging
- On-demand Velero backup (`POST /api/v1/velero/backups`) is unit-tested against a fake clientset only — no cluster in this session's environment had the Velero CRDs installed to validate it against a real one
