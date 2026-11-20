# Pod Logs

## Purpose

Stream and inspect pod/container logs from the console.

## When to use it

- Operate **Pod Logs** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/logs`
- Nav: **Sidebar → Pod Logs**

## Operate from the console (UX)

1. Open `/logs`.
2. Select namespace/pod/container; stream or fetch recent lines.
3. Correlate with Events and Pod detail when diagnosing CrashLoop.
4. **Empty / fail:** No lines → wrong container / pod gone; 403 → RBAC logs.
5. **Success:** Live or recent log text for the selected container.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Pods](../workloads/pods.md)
- [Events](events.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
