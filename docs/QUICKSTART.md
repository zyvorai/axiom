# Quickstart

Get Zyvor Axiom running against your own Kubernetes or k3s cluster in a
few minutes. This guide only assumes `helm`, `kubectl`, and a browser —
no Go toolchain, no Node, no building from source.

## Prerequisites

- A Kubernetes or k3s cluster, with `kubectl` already configured against it
- [Helm](https://helm.sh/docs/intro/install/) 3.x
- Cluster-admin (or equivalent) access, to create a namespace and a
  `ClusterRole`/`ClusterRoleBinding` for Axiom's ServiceAccount

## 1. Install

```bash
helm install axiom deploy/helm/axiom \
  --set auth.username=admin \
  --set auth.password='replace-with-a-strong-password'
```

This creates namespace `axiom-system` (configurable via `--set namespace=...`;
pass `--set createNamespace=false` if it already exists),
Axiom's ServiceAccount and RBAC, a self-signed TLS certificate, and exposes
the UI on NodePort `30590` by default.

Watch it come up:

```bash
kubectl -n axiom-system get pods -w
```

## 2. Open the UI

```text
https://<any-node-ip>:30590
```

Your browser will warn about the self-signed certificate — accept it once.
Sign in with the `auth.username` / `auth.password` you set above.

Verify from the command line first, if you prefer:

```bash
curl -k https://<any-node-ip>:30590/api/v1/health
```

## 3. Common install variations

**Behind an Ingress with a real certificate**, instead of NodePort:

```bash
helm install axiom deploy/helm/axiom \
  --set expose.type=Ingress \
  --set expose.ingress.className=nginx \
  --set expose.ingress.host=axiom.yourcompany.com \
  --set tls.existingSecret=your-ca-cert-secret \
  --set auth.username=admin --set auth.password='...'
```

`your-ca-cert-secret` must already exist as a `kubernetes.io/tls` Secret in
the `axiom-system` namespace (or whatever you set `namespace` to).

**Naming the cluster** shown in Axiom's sidebar switcher:

```bash
--set clusterName=production-east
```

**Wiring SSO at install time** (or do this later from Settings ->
Integrations in the UI):

```bash
--set oidc.issuer=https://your-idp/.well-known \
--set oidc.loginURL=https://your-idp/authorize \
--set oidc.clientID=... \
--set oidc.adminGroup=axiom-admins \
--set oidc.viewerGroup=axiom-viewers
```

If you use OIDC, also set a stable `auth.sessionSecret` (a long random
string) so sessions survive a pod restart — left blank, Axiom generates a
random one per process, which is a safe default but logs everyone out on
every restart/rollout:

```bash
--set auth.sessionSecret=$(openssl rand -base64 32)
```

**Behind a reverse proxy or identity-aware proxy**: also set
`auth.trustedProxyHeader` (e.g. `X-Forwarded-For`) — otherwise every
client behind that proxy shares one rate-limit bucket, and one user's
failed logins can lock out everyone else behind it. Only set this when
the proxy itself sets/overwrites the header; never when it's reachable
directly, since the header is otherwise spoofable by the client:

```bash
--set auth.trustedProxyHeader=X-Forwarded-For
```

## 4. Next steps

- Read the [production hardening checklist](DEPLOYMENT.md#production-hardening-checklist)
  before pointing this at anything but a throwaway evaluation cluster —
  it covers replacing the self-signed certificate, restricting RBAC
  further, and forwarding the audit log.
- Create `viewer`-role accounts for your team from Settings -> Users
  instead of sharing the admin credential.
- See [API.md](API.md) for the full HTTP API reference and
  [ARCHITECTURE.md](ARCHITECTURE.md) for how Axiom's provider model works.

## Upgrading

```bash
helm upgrade axiom deploy/helm/axiom --set image.tag=<new-version> ...
```

Re-pass every `--set` flag you used at install time (or keep them in a
`values.yaml` override file and pass `-f your-values.yaml` instead — the
same file works for both `install` and `upgrade`).

## Uninstalling

```bash
helm uninstall axiom -n axiom-system
```

This removes Axiom's own resources but leaves the `axiom-system` namespace
and any workloads Axiom doesn't own untouched.
