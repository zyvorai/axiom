# Platform Intents

## Purpose

Intent-driven placement / platform requests surfaced in the console.

## When to use it

- Operate **Platform Intents** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/platform-intents`
- Nav: **Sidebar → Platform Intents**

## Operate from the console (UX)

1. Open `/platform-intents`.
2. Review intent cards/status; create via Workloads → Run → Intent when offered.
3. **Empty / fail:** No intents → submit one from the wizard.
4. **Success:** Intent progresses to a concrete workload.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Workloads](workloads.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
