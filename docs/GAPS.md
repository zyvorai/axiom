# Feature gaps (living)

Closed or substantially narrowed in the current tree (see CHANGELOG):

- SAML ACS sessions (`POST /api/v1/auth/saml/acs`) — the response's enveloped XML-DSig signature is verified against the pinned `AXIOM_SAML_IDP_CERT` (only the signed content is trusted, which defeats signature wrapping), and the audience (`AXIOM_SAML_ENTITY_ID`) and `NotBefore`/`NotOnOrAfter` window are enforced. Not yet checked: `InResponseTo` (replay within the validity window is possible) and `SubjectConfirmationData`
- LDAP simple-bind via the local username/password form (`AXIOM_LDAP_*`)
- Operator role + safer OIDC default role env (`AXIOM_OIDC_DEFAULT_ROLE`)
- Fleet cluster register/unregister from kubeconfig (Settings → Clusters)
- HA: Helm fails when `replicaCount > 1` without `auth.sessionSecret`
- Audit ring default raised to 5000 (`AXIOM_AUDIT_RING_SIZE`)
- ConfigMap/Secret update; Velero on-demand backup; VM create/delete; PVC/Service/Ingress/Quota writes; Helm upgrade
- VM console returns proxied `wsPath` for browser VNC clients
- OIDC login validated end-to-end against a real Keycloak IdP (public client + PKCE, no client secret needed) — see `e2e/scripts/verify-sso.js`
- Enterprise write endpoints validated against a real cluster, not just the demo provider/fake clientsets — see `e2e/scripts/validate-enterprise-writes.sh`. Found and fixed one real RBAC gap: `DELETE /api/v1/vms/{ns}/{name}` was 403ing because the `zyvor-axiom` ClusterRole never granted `delete` on `kubevirt.io` VMs
- LDAP group-based role mapping (`AXIOM_LDAP_ADMIN_GROUP`/`AXIOM_LDAP_VIEWER_GROUP`/`AXIOM_LDAP_GROUP_ATTR`) — only for the `AXIOM_LDAP_BASE_DN`-constructed bind form; AD-style NetBIOS/UPN binds still always get the `operator` default

Other known gaps found by the functional test pass (see `e2e/README.md`):

- Registered clusters are always reported `Healthy` (`internal/platform/switchable_provider.go`) — there is no probe
- `POST /api/v1/users` maps an unknown role to `admin` (`NormalizeRole`, documented as intentional; consider fail-closed)
- A saved Slack / webhook URL cannot be cleared from the UI (the API's `*Clear` flags have no control), and the Atlas token has no UI field
- `/api/v1/session` reports `role: "Platform Admin"` for operators and `auth: "oidc"` for SAML sessions
- `DELETE /api/v1/clusters/{unknown}` returns 400 rather than 404

Still thinner than a full private-cloud suite (intentionally or next):

- SAML `InResponseTo` request tracking (needs a server-side store of outstanding AuthnRequests)
- Embedded noVNC UI (proxy path is ready; virtctl remains supported)
- GitOps Application create / Helm chart catalog browse
- Historical Prometheus TSDB / alert silence
- ClusterRole write UI (deliberate security boundary)
- SOC2 / pen-test packaging
- On-demand Velero backup (`POST /api/v1/velero/backups`) is unit-tested against a fake clientset only — no cluster in this session's environment had the Velero CRDs installed to validate it against a real one
