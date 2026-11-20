# Workload detail

## Purpose

Single controller — owned pods, events, labels, rollout, raw YAML.

## When to use it

- Operate **Workload detail** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/workloads/:namespace/:kind/:name`
- Nav: **Workloads → row**

## Operate from the console (UX)

1. Open a workload from the list.
2. Inspect owned Pods and Events; use lifecycle actions from the page.
3. Edit YAML carefully — Save uses server-side apply where offered.
4. **Empty / fail:** Missing pods → check selectors / Events.
5. **Success:** Desired replicas match Ready; rollout history coherent.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Workloads](workloads.md)
- [Pods](pods.md)
- [Events](../observe/events.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
