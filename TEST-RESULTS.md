# Zyvor Axiom Validation Results

Validation date: 2026-08-27

## Passed in this execution environment

```text
go test ./...          PASS
go vet ./...           PASS
go build               PASS
node --check web/app.js PASS
source identity scan   PASS
HTTP smoke test        PASS
```

Built binary:

```text
bin/zyvor-axiom
```

Smoke validation covered:

- unauthenticated `GET /api/v1/health`
- authenticated `GET /api/v1/overview`
- embedded UI serving with `Zyvor Axiom` branding
- authenticated workload launch
- workload appears in subsequent listing
- demo cluster identity is `axiom-demo`

## Source cleanup validation

Runtime source and manifests were scanned for stale `Aether` and `Metal3` identifiers. None remain. Historical rename terminology appears only in documentation that intentionally explains the migration.

## Production Kubernetes tagged build

The real Kubernetes provider is included under the `kubernetes` build tag using `client-go` and `gorilla/websocket`.

This runner cannot resolve `proxy.golang.org`, so it cannot download those external modules or create their `go.sum` entries. The attempted command was:

```bash
go mod download -modfile=go.production.mod
```

and failed at DNS/network access to `proxy.golang.org`.

On a connected Go build machine run:

```bash
go mod download -modfile=go.production.mod
go build -modfile=go.production.mod -tags kubernetes \
  -o bin/zyvor-axiom-kubernetes ./cmd/zyvor-axiom
```
