# Workloads

## Purpose

Deployments, StatefulSets, DaemonSets — scale, restart, seal, rollout, and Run workload wizard.

## When to use it

- Operate **Workloads** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/workloads`
- Nav: **Sidebar → Workloads**

## Operate from the console (UX)

1. Open `/workloads`.
2. **Run workload** — Form / Intent / YAML tabs → submit.
3. Row menu: Start / Stop / Restart / Scale / Seal / Unseal / Delete / Rollout (pause, resume, undo).
4. Open detail `/workloads/:ns/:kind/:name` for pods, events, YAML.
5. **Empty / fail:** Viewer gets 403 on mutate; empty list → wrong namespace filter or fresh cluster.
6. **Success:** New Deployment appears; scale/restart toasts succeed.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Workload detail](workloads-namespace-kind-name.md)
- [Pods](pods.md)
- [Platform Intents](platform-intents.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
