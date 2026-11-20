# Security Policy

## Supported versions

Zyvor Axiom is pre-1.0. Security fixes are made against the latest tagged
release; there is no long-term-support branch yet. See [CHANGELOG.md](CHANGELOG.md)
for released versions.

## Reporting a vulnerability

Please report suspected security vulnerabilities privately — do not open a
public GitHub issue.

Email **security@zyvor.dev** with:

- A description of the vulnerability and its potential impact
- Steps to reproduce (a minimal repro is very helpful)
- The version/commit you tested against, and your deployment mode (demo or
  Kubernetes build)

We aim to acknowledge reports within 3 business days. Once a fix is
available, we'll credit reporters (unless you'd prefer to stay anonymous)
in the CHANGELOG entry for the fix.

## Scope

In scope: the Axiom server (`internal/`), the embedded frontend
(`frontend/`), the Kubernetes manifests and Helm chart under `deploy/`, and
the deployment scripts under `scripts/`.

Out of scope: vulnerabilities in third-party dependencies without a
demonstrated Axiom-specific impact (report those upstream instead), and
issues that require an attacker to already hold cluster-admin or an
Axiom `admin` account (see `docs/DEPLOYMENT.md`'s production hardening
checklist for how to scope those credentials down).

## Design notes for security reviewers

- `deploy/k8s/rbac.yaml` / `deploy/helm/axiom` grant Axiom's ServiceAccount
  a deliberate per-API-group allowlist, not a wildcard `ClusterRole`.
- Axiom's own RBAC UI (Ops -> Access) can create/delete namespaced
  `Role`/`RoleBinding` objects but cannot touch `ClusterRole`/`ClusterRoleBinding`.
- Local accounts are bcrypt-hashed; a rate limiter
  (`internal/auth/ratelimit.go`) applies exponential backoff per client IP
  after repeated failed logins.
- See [README.md#security](README.md#security) and
  [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md#production-hardening-checklist)
  for the full security model and hardening checklist.
