# Pod detail

## Purpose

Container specs, metrics, multi-tab PTY terminal, manifest Save, port Open.

## When to use it

- Operate **Pod detail** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/pods/:namespace/:name`
- Nav: **Pods → row**

## Operate from the console (UX)

1. Open a pod from `/pods`.
2. Read resources/env/probes; Metrics tab for CPU/memory sparklines.
3. Terminal: open sessions (multi-tab WebSocket); demo falls back to one-shot exec.
4. YAML tab → **Save** (server-side apply); Open buttons for container ports.
5. **Empty / fail:** Terminal blocked → auth/token on WS; Save 403 → viewer role.
6. **Success:** Shell prompt or metrics stream; Save toast OK.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Pods](pods.md)
- [Pod Logs](../observe/logs.md)
- [Workloads](workloads.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
