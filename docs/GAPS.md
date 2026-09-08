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

Still thinner than a full private-cloud suite (intentionally or next):

- Full XML-DSig C14N for every IdP (cert pin is the shipped check)
- Embedded noVNC UI (proxy path is ready; virtctl remains supported)
- GitOps Application create / Helm chart catalog browse
- Historical Prometheus TSDB / alert silence
- ClusterRole write UI (deliberate security boundary)
- SOC2 / pen-test packaging
