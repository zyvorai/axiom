# Deploying Zyvor Axiom on k3s / Kubernetes

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

For least privilege, replace the ClusterRole with namespace-scoped Roles where your operating model permits it.

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
./scripts/deploy-remote.sh 80.79.5.173 zyvorai
```

Flow: rsync → generate/reuse `.run/tls` cert → Secret `zyvor-axiom-tls` → podman build → k3s import → kubectl apply → `curl -k` health.

Fast redeploy (skip image rebuild):

```bash
./scripts/deploy-remote.sh 80.79.5.173 zyvorai --quick
```

Also deploy [Atlas](../../atlas) (the separate storage control-plane service) to the same host and wire it into Axiom automatically — mints an admin token from Atlas's freshly-created `atlas-gateway-auth` secret and `PUT`s it into Axiom's own `/api/v1/integrations`, the same call Settings → Integrations makes:

```bash
./scripts/deploy-remote.sh 80.79.5.173 zyvorai --with-atlas
```

Requires a sibling `../atlas` checkout with its own `scripts/deploy-remote.sh` (override the path with `ATLAS_REPO=/path/to/atlas`, and its NodePort with `ATLAS_NODE_PORT` if not `30510`). If the wiring step fails (e.g. Axiom's own health check didn't pass yet), it prints the Atlas URL so you can finish it by hand in Settings → Integrations.

Check last target and health:

```bash
./scripts/deploy status
```

Or via Make:

```bash
make deploy-remote H=80.79.5.173 U=zyvorai
```

### Prerequisites (remote host)

- SSH access (key-based recommended): `ssh zyvorai@<host>` — some lab hosts use a different user (e.g. `sus@80.79.5.173`); pass it as the second argument
- k3s with `kubectl` configured for the deploy user (`~/.kube/config`)
- `podman` or `docker` for image builds
- `openssl` for self-signed cert generation

### Defaults

| Setting | Default |
|---------|---------|
| Remote SSH user | `zyvorai` (override: 2nd arg or `DEPLOY_USER`) |
| Remote checkout | `~/.deployment/axiom` |
| Image | `zyvor-axiom:dev` (local k3s import) |
| NodePort | `30590` (HTTPS; use `30591` if `30590` is taken) |
| Lab login | `demo` / `demo` (shown automatically when SSO is off) |
| TLS | Self-signed, reused under `.run/tls/` |

Override with environment variables: `AXIOM_IMAGE_TAG`, `AXIOM_NODE_PORT`, `AXIOM_USERNAME`, `AXIOM_PASSWORD`.

### After deploy

- UI: `https://80.79.5.173:30591` (accept self-signed warning; port may differ if `30590` is in use)
- Health: `curl -k https://80.79.5.173:30591/api/v1/health`
- **Lab sign-in:** username `demo`, password `demo` — the login form appears automatically when SSO is not configured

### Troubleshooting

```bash
ssh zyvorai@80.79.5.173 kubectl -n axiom-system get pods,svc
ssh zyvorai@80.79.5.173 kubectl -n axiom-system logs -l app=zyvor-axiom --tail=50
ssh zyvorai@80.79.5.173 'sudo ufw status; sudo firewall-cmd --list-ports 2>/dev/null'
```

If the NodePort is blocked, open TCP `30590` (or your `AXIOM_NODE_PORT`) on the host firewall (the deploy script attempts this automatically).

If deploy fails with `nodePort ... already allocated to another Service`, another Service on that cluster already holds the port (shared lab hosts often have unrelated workloads squatting on NodePorts). Find it and redeploy on a free one:

```bash
ssh zyvorai@80.79.5.173 'kubectl get svc -A -o wide | grep 30590'
AXIOM_NODE_PORT=30592 ./scripts/deploy-remote.sh 80.79.5.173 zyvorai --quick
```

Uninstall:

```bash
./scripts/deploy-remote.sh 80.79.5.173 zyvorai --uninstall
```

## Production hardening checklist

- Real TLS certificate (replace self-signed lab cert)
- OIDC/SAML or identity-aware proxy instead of reference Basic Auth
- namespace-scoped RBAC where possible
- network policy protecting the Axiom namespace
- external secret management
- audit log forwarding
- immutable image tags/digests
- PodSecurity admission profile
- resource quotas and LimitRanges
- backups for cluster state and Axiom configuration
