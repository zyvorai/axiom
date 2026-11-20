# Jobs & CronJobs

## Purpose

Batch Jobs and CronJobs — trigger runs, inspect status, delete.

## When to use it

- Operate **Jobs & CronJobs** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/jobs`
- Nav: **Sidebar → Jobs & CronJobs**

## Operate from the console (UX)

1. Open `/jobs`.
2. On CronJobs: **Run now** to trigger; delete retired jobs/cronjobs from row actions.
3. Confirm Job pods under Pods / Logs.
4. **Empty / fail:** No CronJobs → create via Apply YAML or workload wizard.
5. **Success:** Triggered Job appears with completions.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Pods](pods.md)
- [Pod Logs](../observe/logs.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
