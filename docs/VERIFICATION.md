# Real-world verification checklist

Features below are covered by unit, fake-cluster and demo e2e tests, but have **not** been
run against the real systems they talk to. Work through this list on a staging cluster
before relying on them. Tick each box, note the version you tested, and report anything
that differs from "Expected".

Conventions: `$AX` is your Axiom URL, `$AUTH` an admin credential (`-u user:pass` or a
bearer token). Never paste real credentials into tickets or logs.

## Already verified on a real cluster

Run on the dev k3s cluster (v1.35, Axiom 0.13.1, 2026-11-04); the test operators and registry were removed afterwards.

- **VPA and KEDA edit (section 5):** edit view, one-change preview, save with annotations preserved, stale write -> 409, a `kubectl`-made VPA with an unsupported field refused, KEDA trigger/max-replica edit applied and honoured by KEDA. Not yet checked: restricted-operator namespace limits against a real cluster.
- **Private OCI registry over plain http (section 3):** no-login message, write-only login, repo with login, chart values, install and upgrade through the Job (pull with the stored login), in-use delete refused, audit entries. Not yet checked: a registry whose token realm is on another domain, and cloud registries (GHCR, ECR, ACR, Harbor).
- **Durable fleet history (section 6):** ConfigMap created, `durable: true`, `restoredFrom` set after a restart. Not yet checked: multi-replica merge and many clusters.

- **Pod proxy in host mode with two replicas and no session affinity (section 4):** per-session hostnames; a link minted on one replica redeems on the other and the cookie works on both; a replayed ticket is refused on both; another session's cookie, a tampered cookie and an unknown host are refused (403/404); the in-console route returns 410; a deleted operator's session is refused on both replicas within about 18 s (refresh 15 s); the redeemed-ticket ConfigMap stayed at 236 bytes; no errors in the logs. Run over SSH tunnels straight to each replica with `https://{id}.pods.verify.test:30591` hostnames, so real DNS/wildcard certificates, browsers, WebSocket apps and absolute-path apps (Grafana, Jupyter) are still unchecked, and so is multi-cluster pinning (dev has one cluster).

- **GitOps with real Argo CD v2.13.3 and Flux v2.4.0 (section 7):** Argo CD create (preview then apply; synced and healthy), edit (one-change preview, stale write -> 409, live `targetRevision` updated), sync, repository credentials (write-only, labelled Secret), delete. Flux Kustomization create (GitRepository + Kustomization Ready, workload deployed), stale edit -> 409, sync, credentials (write-only), cascading delete refused without `confirm=<name>` and then removing the workload. Not yet checked: Flux HelmRelease create/edit, a non-no-op Flux edit, SSH-key credentials, private Git repositories, and the UI dialogs in a browser.

Finding (historical): Helm installs used to fail when the chart created a ServiceAccount. The ClusterRole now grants ServiceAccounts, PDBs, Leases, EndpointSlices and ControllerRevisions — re-verify with podinfo (or similar) after applying the updated RBAC. Charts that mint CRDs or ClusterRoles still need a wider Role or an out-of-band install.

## 0. Live harnesses (shipped with the docs)

- [ ] `e2e/scripts/validate-velero.sh` against a cluster with `backups.velero.io` installed.
  Expected: create Backup + Schedule via the API, `kubectl get` succeeds, cleanup deletes them.
- [ ] `e2e/scripts/validate-billing-live.sh` with Settings configured, **or**
  `AXIOM_BILLING_LIVE=1 AWS_ACCESS_KEY_ID=… AWS_SECRET_ACCESS_KEY=…` (optional `AWS_SESSION_TOKEN`).
  Expected: Cost Explorer returns at least one month; errors never echo the secret.
- [ ] Helm install of an ordinary chart (e.g. podinfo) through the console after the
  extended ClusterRole is applied. Expected: release appears; Job Succeeded.

## 1. Cloud invoice reconciliation (experimental, 0.13.0+)

Covered by `httptest` fixtures (incl. NextPageToken + STS). Use a **read-only** billing identity; prefer the live harness above.

For each provider you use (Settings → Integrations → Billing):

- [ ] **AWS**: IAM user (or STS) with `ce:GetCostAndUsage` only. Region `us-east-1`. Optional session token field. Press *Test*.
  Expected: success with a currency and a non-error latency. A wrong secret gives an error
  that does **not** contain the key.
- [ ] **Azure**: app registration with *Cost Management Reader* on the subscription.
  Expected: *Test* succeeds; the invoice card lists the current and previous month.
- [ ] **GCP**: service account with BigQuery Data Viewer + Job User on the billing export
  table. Table name must be `project.dataset.table`. Expected: *Test* succeeds.
- [ ] Compare `GET $AX/api/v1/cost/invoice` month totals with the provider's own console
  (same currency, same month). Expected: within rounding; if not, note the difference.
- [ ] After saving, `GET /api/v1/integrations` shows `...Set: true` flags and **no**
  secret values; `kubectl -n axiom-system get secret` holds them, the ConfigMap does not.
- [ ] Egress: with a NetworkPolicy allowing only the provider's hosts, the card still
  loads (AWS `ce.us-east-1.amazonaws.com`; Azure `login.microsoftonline.com`,
  `management.azure.com`; GCP `oauth2.googleapis.com`, `bigquery.googleapis.com`).
- [ ] Known approximation: "allocated" is today's 30-day run-rate prorated per month.
  Confirm this is acceptable or file a request for allocation history.

## 2. LDAP two-factor sign-in (0.10.0)

Needs a real directory (AD or OpenLDAP) with `AXIOM_LDAP_*` configured.

- [ ] Sign in with a directory account (no MFA). Expected: normal session.
- [ ] Enrol TOTP from the user menu (password re-confirmed). Sign out, sign in again.
  Expected: password step then code step.
- [ ] Wrong codes repeatedly. Expected: audit shows `auth.mfa_failed`, then throttling (429) after the configured failure limit.
- [ ] Use a recovery code once. Expected: works once, second use fails.
- [ ] `AXIOM_REQUIRE_MFA=all`: an unenrolled directory user is forced to enrol before
  using the console; the mapped role (admin/viewer) decides coverage.
- [ ] As admin, `DELETE /api/v1/users/{username}/mfa` for that directory user. Expected:
  second factor reset and the user's sessions revoked.
- [ ] Disable the account in the directory. Expected: next sign-in fails even with a
  valid code.

## 3. Private OCI Helm registries (0.11.0)

Use a real private registry (GHCR, Harbor, ECR with a token, ACR…).

- [ ] Helm → Catalog → *OCI registry* → add the repo **without** a login. Expected: the
  message "this registry needs credentials: add them under Helm → Registries".
- [ ] *Registry logins* → add name, registry host, username, token. Expected: listed
  without the password; viewers get 403 on `GET /api/v1/helm/registry-credentials`.
- [ ] Add the repo again selecting that login. Expected: "private" badge; chart list and
  default values load.
- [ ] Install a chart by `oci://` reference. Expected: the install Job pulls with the
  login (check the Job's pod logs; `HELM_REGISTRY_CONFIG` is mounted from a Secret) and
  the release appears. The registry Secret for the Job is deleted afterwards.
- [ ] Upgrade the release. Expected: audit entry records the chart (`ociRef`) and
  `registry-credential=<name>`.
- [ ] Try deleting the login while a repo uses it. Expected: refused (in use).
- [ ] Token realm on another domain (e.g. a registry that redirects auth to a different
  host): confirm the login is sent only to the same site/eTLD+1; otherwise expect a
  clear error rather than a leak.
- [ ] Plain-http registry (lab only): requires `AXIOM_HELM_ALLOW_INSECURE_REPOS=true`;
  install works with Helm ≥ 3.13 (`--plain-http`). Credentials travel unencrypted.

## 4. Pod proxy: separate origin, per-session hosts, replicas (0.10.0 – 0.12.1)

Needs real DNS/TLS or `*.localhost` for a lab.

- [ ] **Fixed origin**: set `AXIOM_POD_PROXY_ADDR` + `AXIOM_POD_PROXY_ORIGIN`. Open a pod
  port from the UI. Expected: opens on the pod origin, the console cookie is not sent
  there, `GET /api/v1/pods/.../window/<port>/` on the console returns 410.
- [ ] **Host mode**: `AXIOM_POD_PROXY_ORIGIN=https://{id}.pods.example.com`, wildcard DNS
  and wildcard certificate (or `podProxy.tlsSecret`). Open the same port twice.
  Expected: two different hostnames, each with its own cookie; a cookie/ticket from one
  host is refused on the other; unknown host → empty 404.
- [ ] Absolute-path web apps (Grafana, Jupyter, Prometheus UI) work through host mode.
- [ ] WebSocket-based UIs through the proxy (note any that fail).
- [ ] Ticket is single use: reloading the redeem URL a second time fails.
- [ ] **Multiple replicas** (`replicaCount: 2`, `auth.sessionSecret` set, **no** session
  affinity): open a pod window repeatedly. Expected: always works regardless of which
  replica serves each request. Check the ConfigMap `zyvor-axiom-pod-proxy-redeemed`
  exists and stays small.
- [ ] Revoke the user (sign out everywhere / disable). Expected: pod windows return 403
  on both replicas within `AXIOM_REVOCATION_REFRESH` (15 s default).
- [ ] **Multi-cluster**: open a pod window on cluster A, then switch the console tab to
  cluster B. Expected: the open window still shows A's pod. Remove cluster A from the
  registry: the window returns 410.
- [ ] After upgrading from 0.12.0, previously issued pod links return 410 (expected,
  open the port again).

## 5. VPA and KEDA edit in place (0.11.0)

Install the operators on a test cluster first:
VPA (from kubernetes/autoscaler) and KEDA (Helm chart `kedacore/keda`).

- [ ] Create a VPA (UI) targeting a Deployment; open *Edit*, change update mode to
  `Initial`, *Preview* shows exactly that one change, *Save*. Expected:
  `kubectl get vpa -o yaml` shows the change, other fields (annotations, status) intact.
- [ ] Edit a VPA that was created by `kubectl` with an unsupported field (e.g. container
  policy `mode`). Expected: shown as not editable with reasons, no write possible.
- [ ] Create a ScaledObject with a cron trigger, edit min/max replicas and a trigger
  setting, preview, save. Expected: change applied; KEDA keeps scaling.
- [ ] Concurrent edit: change the object with `kubectl` between Preview and Save.
  Expected: 409 with the "changed since you opened it" message.
- [ ] RBAC: the ClusterRole has `patch` on `verticalpodautoscalers` and `scaledobjects`
  (re-apply `deploy/k8s/rbac.yaml` or upgrade the chart if you installed before 0.11.0).
- [ ] A namespace-restricted operator can edit only their own namespace.

## 6. Durable fleet history (0.11.0)

Needs two or more registered clusters and ideally two replicas.

- [ ] Wait more than 5 minutes after start. Expected: ConfigMap `zyvor-axiom-fleet-history`
  exists; `GET /api/v1/fleet/history` has `durable: true`.
- [ ] Restart the pod. Expected: charts keep earlier history, the restart gap is drawn
  as a gap, and no downtime or incident is counted for the restart itself.
- [ ] Two replicas: each writes its own key; after a restart of one, history from both
  is merged without duplicate points.
- [ ] Remove a cluster from the registry. Expected: its history disappears on the next
  save/restore; the ConfigMap stays under 900 KiB with many clusters.

## 7. GitOps (Argo CD / Flux) create, edit, credentials

- [ ] Install Argo CD; create an Application from the UI, edit its revision (preview
  shows the exact change), delete it with and without cascade.
- [ ] Install Flux; same for Kustomization/GitRepository.
- [ ] Repository credentials: add one; confirm the Secret carries the Argo CD/Flux label,
  the UI never shows the password, and removing it works.

## 8. Cost with OpenCost

- [ ] Install OpenCost, set `cost.opencostUrl`. Expected: the Cost page switches from
  "estimate" to actual allocation with an idle share and a time window selector.
- [ ] Budgets still evaluate (warning/over notifications) using the actual numbers.

## 9. Other environment-dependent paths

- [ ] **SSO**: OIDC with JWKS signature and nonce checks against your IdP; SAML sign-in,
  and IdP-initiated only if `AXIOM_SAML_ALLOW_IDP_INITIATED` is set.
- [ ] **Node shell** (off by default): enable on a throwaway cluster, confirm it is
  admin-only, audited, and the privileged pod is removed on disconnect.
- [ ] **Security reports**: install Trivy-operator / Kyverno and confirm the `/security`
  page lists real reports (otherwise it shows the install hint).
- [ ] **AI assistant** against your real provider: check the payload preview contains no
  Secret/ConfigMap values or env var values before sending.

## 10. Translations

Machine-translated (de, es, fr, pt, it, ja, zh); none reviewed by a native speaker.

- [ ] Ask a native speaker per language to review `frontend/src/locales/<lang>/*.json`
  against `frontend/src/locales/GLOSSARY.md`; send fixes as a PR.
- [ ] Spot-check long strings for layout overflow (buttons, table headers) in German and
  Portuguese, and CJK line breaking in 日本語 and 简体中文.

## Reporting

For each failed box, record: Axiom version, Kubernetes version, what you did, what you
expected, what happened, and relevant `kubectl logs` lines (redact secrets). Open an
issue at https://github.com/zyvorai/axiom/issues.
