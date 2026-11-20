# Compliance and security posture

An honest current-state statement for prospects and customers evaluating
Axiom, kept up to date as the program matures — not a claim of
certifications Zyvor doesn't yet hold.

## Current status

- **SOC 2 / ISO 27001:** Not yet certified. [DATE OR TARGET, IF ANY —
  business to confirm whether/when this is planned.]
- **Third-party penetration test:** Not yet performed. [Note here once
  one is commissioned, including scope and remediation status.]
- **Vulnerability scanning:** Automated in CI as of `v0.1.0` — `govulncheck`
  against the Go module (both build configurations), `npm audit` against
  the frontend, and a Trivy scan of the published container image (see
  `.github/workflows/ci.yml` and `.github/workflows/release.yml`).
  Findings are currently visible but do not block merges or releases —
  see [SECURITY.md](../SECURITY.md).
- **Vulnerability disclosure:** A private reporting channel and process
  exist — see [SECURITY.md](../SECURITY.md).
- **Data residency:** Axiom is self-hosted — it runs entirely inside the
  customer's own Kubernetes cluster. Zyvor does not host, receive, or
  store customer cluster data or workloads. See `legal/DPA-DRAFT.md` for
  the data-processing implications of this model.
- **Supply chain:** Container images are built from a `distroless`,
  non-root base image (see `Dockerfile`). Image signing (cosign) and an
  SBOM (syft) are produced starting with `v0.1.0`'s release pipeline —
  see the release workflow's `sbom` and `sign` steps.

## Security architecture references

For reviewers who want the technical detail behind this posture
statement rather than just the summary above:

- [README.md#security](../README.md#security) — auth model, RBAC
  allowlist design, local account roles
- [docs/DEPLOYMENT.md#production-hardening-checklist](DEPLOYMENT.md#production-hardening-checklist)
  — what we recommend customers configure before going to production
- [SECURITY.md](../SECURITY.md) — vulnerability disclosure policy and scope

## Roadmap

This section should be kept current as the program develops — remove
completed items and add real dates once they're committed to, rather
than leaving aspirational language in place indefinitely.

- [ ] Decide on and schedule a third-party penetration test
- [ ] Decide whether SOC 2 Type I/II is a near-term goal (commonly driven
      by specific customer procurement requirements — revisit once one
      comes up)
- [ ] Start enforcing (not just surfacing) CI vulnerability scan results
      once the current findings backlog is triaged
