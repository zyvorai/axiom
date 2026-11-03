# Deploying Zyvor Axiom on k3s / Kubernetes

For a first install, see [QUICKSTART.md](QUICKSTART.md) — it uses the Helm
chart at `deploy/helm/axiom` and only assumes `helm`, `kubectl`, and a
browser. The rest of this document covers the underlying raw-manifest
deploy (what the Helm chart templates) and remains useful for the lab-SSH
workflow, understanding what the chart does under the hood, or building
from source.

## 1. Build the production binary

```bash
go mod download -modfile=go.production.mod
go build -modfile=go.production.mod -tags kubernetes -o bin/zyvor-axiom-kubernetes ./cmd/zyvor-axiom
```

## 2. Build the container

The Dockerfile builds the production Kubernetes binary (`-tags kubernetes`, `go.production.mod`).

Recommended image identity for registry deploys:

```text
ghcr.io/zyvorai/axiom:<version>
```

For k3s lab hosts, use `./scripts/deploy-remote.sh` to build locally with podman and import into k3s (see below).

## 3. Configure credentials and TLS

The Deployment expects **HTTPS only** via `AXIOM_TLS_CERT` / `AXIOM_TLS_KEY` mounted from Secret `zyvor-axiom-tls`.

```bash
# Auth
kubectl -n axiom-system create secret generic zyvor-axiom-auth \
  --from-literal=username=admin \
  --from-literal=password='replace-with-strong-secret' \
  --dry-run=client -o yaml | kubectl apply -f -

# Self-signed TLS (lab) — or use your CA-issued cert
openssl req -x509 -newkey rsa:2048 -sha256 -days 3650 -nodes \
  -keyout tls.key -out tls.crt \
  -subj "/CN=axiom.local/O=Zyvor Axiom" \
  -addext "subjectAltName=DNS:axiom.local,IP:127.0.0.1"
kubectl -n axiom-system create secret tls zyvor-axiom-tls \
  --cert=tls.crt --key=tls.key \
  --dry-run=client -o yaml | kubectl apply -f -
```

## 4. Apply RBAC and deployment

```bash
kubectl apply -f deploy/k8s/rbac.yaml
kubectl apply -f deploy/k8s/deployment.yaml
kubectl apply -f deploy/k8s/nodeport.yaml   # lab NodePort 30590
```

`deploy/k8s/rbac.yaml` grants: core resources (Pods, Services, ConfigMaps, Secrets, ResourceQuotas, LimitRanges, ...), `apps` Deployments/StatefulSets, `batch` Jobs/CronJobs, `networking.k8s.io` NetworkPolicies/Ingresses, plus the KubeVirt/Helm/Velero/GitOps/Haven CRD groups Axiom optionally reads. If you narrow this ClusterRole, keep `secrets` write access in Axiom's own namespace — both the Integrations hub and local Users store (`zyvor-axiom-users` Secret) depend on it.

For least privilege, replace the ClusterRole with namespace-scoped Roles where your operating model permits it.

Optionally set `AXIOM_CLUSTER_NAME` on the Deployment to label the single connected cluster shown in the sidebar switcher and Settings → Clusters (defaults to `in-cluster` — see [ARCHITECTURE.md](ARCHITECTURE.md#multi-cluster-switchableprovider)).

To map OIDC group membership to Axiom roles, also set `AXIOM_OIDC_ADMIN_GROUP` and/or `AXIOM_OIDC_VIEWER_GROUP` (and `AXIOM_OIDC_GROUPS_CLAIM` if your IdP doesn't use the claim name `groups`) — leave both unset and every OIDC login stays admin, unchanged from the default.

## 5. Expose the UI

For a lab NodePort (self-signed HTTPS):

```text
https://<node-ip>:30590
```

If port `30590` is already allocated (e.g. by another NodePort service), override with `AXIOM_NODE_PORT=30591` when deploying.

Accept the browser certificate warning once. Probe with:

```bash
curl -k https://<node-ip>:30590/api/v1/health
```

Port-forward (HTTPS):

```bash
kubectl -n axiom-system port-forward svc/zyvor-axiom 5090:443
curl -k https://localhost:5090/api/v1/health
```

Plain HTTP is not supported for cluster deploys. For production, place Axiom behind your Gateway/Ingress with a real certificate and enterprise authentication.

## 6. k3s notes

Axiom uses the Kubernetes API and does not depend on a specific distro. In a k3s cluster the same ServiceAccount, workload, Pod log/exec, eviction and node APIs are used.

## Remote lab deploy (k3s)

One command deploys Axiom to a remote k3s node (**HTTPS only**, self-signed TLS):

```bash
./scripts/deploy-remote.sh <ephemeral-ip> zyvorai
```

Flow: rsync → generate/reuse `.run/tls` cert → Secret `zyvor-axiom-tls` → podman build → k3s import → kubectl apply → `curl -k` health.

Fast redeploy (skip image rebuild):

```bash
./scripts/deploy-remote.sh <ephemeral-ip> zyvorai --quick
```

Also deploy [Atlas](../../atlas) (the separate storage control-plane service) to the same host and wire it into Axiom automatically — mints an admin token from Atlas's freshly-created `atlas-gateway-auth` secret and `PUT`s it into Axiom's own `/api/v1/integrations`, the same call Settings → Integrations makes:

```bash
./scripts/deploy-remote.sh <ephemeral-ip> zyvorai --with-atlas
```

Requires a sibling `../atlas` checkout with its own `scripts/deploy-remote.sh` (override the path with `ATLAS_REPO=/path/to/atlas`, and its NodePort with `ATLAS_NODE_PORT` if not `30510`). If the wiring step fails (e.g. Axiom's own health check didn't pass yet), it prints the Atlas URL so you can finish it by hand in Settings → Integrations.

**`--with-atlas` deploys Atlas's own fake/demo image** (namespace `zyvor-system`, port `30510`).
A *real*-Ceph Atlas gateway — the one `../hypercluster`'s `storage apply` deploys via
`../atlas/scripts/deploy-ceph-gateway-remote.sh` for a production Ceph cluster — lives in
namespace `rook-ceph` on port `30511` instead, and `--with-atlas` doesn't know about it. Every
deploy (no flag needed) now auto-detects and wires that path too if present and not already
configured, plus auto-detects and wires a PacketWolf deployment (`AXIOM_PACKETWOLF_URL`/
`AXIOM_PACKETWOLF_API_KEY`, read live from env — no Settings → Integrations persistence for
PacketWolf, so unlike Atlas this re-wires on *every* deploy, not just once, since a bare
`kubectl apply` would otherwise silently drop it on the next redeploy). Found live (2026-08-31):
a cluster with a fully healthy real-Ceph Atlas gateway and PacketWolf deploy still showed neither
wired in Axiom's own UI, because the only wiring path assumed a namespace/port this deployment
mode never used.

Check last target and health:

```bash
./scripts/deploy status
```

Or via Make:

```bash
make deploy-remote H=<ephemeral-ip> U=zyvorai
```

### Prerequisites (remote host)

- SSH access (key-based recommended): `ssh zyvorai@<host>` — some lab hosts use a different user (e.g. `operator@<ephemeral-ip>`); pass it as the second argument
- k3s with `kubectl` configured for the deploy user (`~/.kube/config`)
- `podman` or `docker` for image builds
- `openssl` for self-signed cert generation

### Defaults

| Setting | Default |
|---------|---------|
| Remote SSH user | `zyvorai` (override: 2nd arg or `DEPLOY_USER`) |
| Remote checkout | `~/.deployment/axiom` |
| Image | `ghcr.io/zyvorai/axiom:<version>` (published; `--build` uses a local `zyvor-axiom:dev` import) |
| NodePort | `30590` (HTTPS; use `30591` if `30590` is taken) |
| Lab login | existing `zyvor-axiom-auth` Secret if present, else `demo` / `demo` (shown automatically when SSO is off) |
| TLS | Self-signed, reused under `.run/tls/` |

Override with environment variables: `AXIOM_IMAGE_TAG`, `AXIOM_NODE_PORT`, `AXIOM_USERNAME`, `AXIOM_PASSWORD`.

### After deploy

- UI: `https://<ephemeral-ip>:30591` (accept self-signed warning; port may differ if `30590` is in use)
- Health: `curl -k https://<ephemeral-ip>:30591/api/v1/health`
- **Lab sign-in:** username `demo`, password `demo` — the login form appears automatically when SSO is not configured

### Troubleshooting

```bash
ssh zyvorai@<ephemeral-ip> kubectl -n axiom-system get pods,svc
ssh zyvorai@<ephemeral-ip> kubectl -n axiom-system logs -l app=zyvor-axiom --tail=50
ssh zyvorai@<ephemeral-ip> 'sudo ufw status; sudo firewall-cmd --list-ports 2>/dev/null'
```

If the NodePort is blocked, open TCP `30590` (or your `AXIOM_NODE_PORT`) on the host firewall (the deploy script attempts this automatically).

If deploy fails with `nodePort ... already allocated to another Service`, another Service on that cluster already holds the port (shared lab hosts often have unrelated workloads squatting on NodePorts). Find it and redeploy on a free one:

```bash
ssh zyvorai@<ephemeral-ip> 'kubectl get svc -A -o wide | grep 30590'
AXIOM_NODE_PORT=30592 ./scripts/deploy-remote.sh <ephemeral-ip> zyvorai --quick
```

Uninstall:

```bash
./scripts/deploy-remote.sh <ephemeral-ip> zyvorai --uninstall
```

## Production hardening checklist

- Real TLS certificate (replace self-signed lab cert)
- OIDC or an identity-aware proxy instead of reference Basic Auth
- If using OIDC, set `AXIOM_SESSION_SECRET` (Helm: `auth.sessionSecret`) to a stable random value — left unset, Axiom generates a safe random one per process automatically, but sessions won't survive a restart
- If Axiom sits behind a reverse proxy or identity-aware proxy, set `AXIOM_TRUSTED_PROXY_HEADER` (Helm: `auth.trustedProxyHeader`, e.g. `X-Forwarded-For`) so the login rate limiter keys on the real client IP — otherwise every client behind that proxy shares one rate-limit bucket
- Local `viewer`-role accounts (Settings → Users) for anyone who doesn't need write access, instead of sharing the admin credential — restrict them to specific namespaces when a team should only see their own workloads
- namespace-scoped RBAC where possible, both at the Kubernetes ServiceAccount level (`deploy/k8s/rbac.yaml`) and at Axiom's own viewer-account level (Settings → Users)
- network policy protecting the Axiom namespace
- external secret management
- forward the audit log (`GET /api/v1/audit`, persisted in `zyvor-axiom-audit`) to your real log/SIEM pipeline — Axiom keeps only the most recent 200 entries
- immutable image tags/digests
- enforce the PodSecurity `restricted` profile on the Axiom namespace (`kubectl label ns axiom-system pod-security.kubernetes.io/enforce=restricted`). The Axiom pod already complies: the Helm chart (`podSecurityContext`/`securityContext`) and `deploy/k8s/deployment.yaml` run it as non-root (65532) with a read-only root filesystem, all capabilities dropped, no privilege escalation and the `RuntimeDefault` seccomp profile
- resource quotas and LimitRanges (viewable live on Axiom's own Quotas & Limits page)
- backups for cluster state and Axiom configuration
- configure Settings → Notifications (Slack/webhook) so cluster Warning events actually reach someone
