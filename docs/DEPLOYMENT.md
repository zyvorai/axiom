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
./scripts/deploy-remote.sh 212.8.248.187 sus
```

Flow: rsync → generate/reuse `.run/tls` cert → Secret `zyvor-axiom-tls` → podman build → k3s import → kubectl apply → `curl -k` health.

Fast redeploy (skip image rebuild):

```bash
./scripts/deploy-remote.sh 212.8.248.187 sus --quick
```

Check last target and health:

```bash
./scripts/deploy status
```

Or via Make:

```bash
make deploy-remote H=212.8.248.187 U=sus
```

### Prerequisites (remote host)

- SSH access (key-based recommended): `ssh sus@212.8.248.187`
- k3s with `kubectl` configured for the deploy user (`~/.kube/config`)
- `podman` or `docker` for image builds
- `openssl` for self-signed cert generation

### Defaults

| Setting | Default |
|---------|---------|
| Remote checkout | `~/.deployment/axiom` |
| Image | `zyvor-axiom:dev` (local k3s import) |
| NodePort | `30590` (HTTPS) |
| Login | `demo` / `demo` (or `demo@zyvor.dev` / `demo`) |
| TLS | Self-signed, reused under `.run/tls/` |

Override with environment variables: `AXIOM_IMAGE_TAG`, `AXIOM_NODE_PORT`, `AXIOM_USERNAME`, `AXIOM_PASSWORD`.

### After deploy

- UI: `https://212.8.248.187:30590` (accept self-signed warning)
- Health: `curl -k https://212.8.248.187:30590/api/v1/health`

### Troubleshooting

```bash
ssh sus@212.8.248.187 kubectl -n axiom-system get pods,svc
ssh sus@212.8.248.187 kubectl -n axiom-system logs -l app=zyvor-axiom --tail=50
ssh sus@212.8.248.187 'sudo ufw status; sudo firewall-cmd --list-ports 2>/dev/null'
```

If the NodePort is blocked, open TCP `30590` on the host firewall (the deploy script attempts this automatically).

Uninstall:

```bash
./scripts/deploy-remote.sh 212.8.248.187 sus --uninstall
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
