# Pods

## Purpose

Pod inventory across namespaces — phase, restarts, entry into pod detail.

## When to use it

- Operate **Pods** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/pods`
- Nav: **Sidebar → Pods**

## Operate from the console (UX)

1. Open `/pods`; filter by namespace/status.
2. Open `/pods/:ns/:name` for specs, metrics, terminal, YAML.
3. Use scheduling-diagnosis banner when not Running.
4. **Empty / fail:** Empty → broaden namespace; CrashLoop → Logs + Events.
5. **Success:** Running pods listed; detail terminal connects (HTTPS / token).

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Pod detail](pods-namespace-name.md)
- [Pod Logs](../observe/logs.md)
- [Events](../observe/events.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
