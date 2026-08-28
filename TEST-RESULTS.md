# Zyvor Axiom Validation Results

## 2026-08-28 — React frontend rewrite + live remote validation

The embedded UI was replaced entirely: the hand-written `web/app.js` vanilla-JS SPA is gone, replaced by `frontend/` (React 19 + CRACO + Tailwind + shadcn/ui), ported directly from the kube-cockpit reference and wired to Axiom's real API.

```text
go test ./cmd/... ./internal/...                                    PASS
go vet ./cmd/... ./internal/...                                     PASS
go build ./cmd/zyvor-axiom                                          PASS (demo build)
go build -modfile=go.production.mod -tags kubernetes ./cmd/zyvor-axiom  PASS (kubernetes build)
npx craco build (frontend/)                                         PASS
```

Verified in a real Chrome browser (not just curl) against `make demo` locally, covering every page reachable from the sidebar, plus:

- Login (local Basic-auth form + SSO buttons), workload launch wizard (Form/Intent/Validate-YAML), node cordon/drain, VM start/stop/migrate, Helm rollback/uninstall, agent-run approve/deny, backup create/restore, secret create, namespace create/delete, pod exec (one-shot, demo mode), Atlas page, PacketWolf panel (not-configured state).

### Live remote deployment (real k3s cluster + real Atlas)

Deployed to a real remote host with `./scripts/deploy-remote.sh <host> <user> --with-atlas` (Axiom + a separate Atlas storage-control-plane service, auto-wired together). Found and fixed a real bug in the process: an unrelated pre-existing Service was already holding the default NodePort, which `kubectl apply` silently rejected — the script now checks the apply's exit status and the live NodePort, failing loudly with a clear "pick a different `AXIOM_NODE_PORT`" message instead of limping on.

Verified end-to-end **in an actual Chrome window** against the live cluster (not a demo):

- Real interactive Pod terminal: typed `echo HELLO_FROM_REAL_CHROME_TERMINAL` on the keyboard into the xterm.js UI → WebSocket → real bash PTY in a real KubeVirt pod → output rendered back correctly
- Atlas page showing real Ceph capacity (204TB raw / 90TB used) from actual backends/pools/volumes
- Workloads page listing all 65 real Deployments on the cluster
- Pod detail page showing real owner references, container images, resource requests from an actual pod's live manifest
- Getting Started onboarding checklist correctly detecting real unconfigured-SSO state

Also verified directly over a raw WebSocket client (bypassing the browser, to isolate protocol correctness): the same terminal endpoint with `?access_token=` query-param auth, confirming the `__resize__:WxH` control message and PTY exec both work independent of any frontend code.

## 2026-08-27 — prior validation (superseded)

```text
go test ./...          PASS
go vet ./...           PASS
go build               PASS
node --check web/app.js PASS   (web/ no longer exists — see above)
source identity scan   PASS
HTTP smoke test        PASS
```

At the time of this entry, the kubernetes-tagged build could not be validated in the sandbox (no network access to `proxy.golang.org` to download `client-go`/`gorilla/websocket`). This is resolved as of 2026-08-28 above — the kubernetes build has since been compiled and deployed successfully.
