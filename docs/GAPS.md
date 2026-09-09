# Feature gaps (living)

Closed or substantially narrowed in the current tree (see CHANGELOG):

- Real SAML ACS sessions (`POST/GET /api/v1/auth/saml/acs`) with IdP cert pin or lab unsigned mode
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

Still thinner than a full private-cloud suite (intentionally or next):

- Full XML-DSig C14N for every IdP (cert pin is the shipped check)
- Embedded noVNC UI (proxy path is ready; virtctl remains supported)
- GitOps Application create / Helm chart catalog browse
- Historical Prometheus TSDB / alert silence
- ClusterRole write UI (deliberate security boundary)
- SOC2 / pen-test packaging
- On-demand Velero backup (`POST /api/v1/velero/backups`) is unit-tested against a fake clientset only — no cluster in this session's environment had the Velero CRDs installed to validate it against a real one
