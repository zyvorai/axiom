# Networking · Cilium

## Purpose

Service graph, NetworkPolicies, and live PacketWolf flows when configured.

## When to use it

- Operate **Networking · Cilium** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/networking`
- Nav: **Sidebar → Networking · Cilium**

## Operate from the console (UX)

1. Open `/networking`.
2. Inspect service graph / Cilium summary.
3. NetworkPolicies: **Create via YAML** (ApplyManifest) or Delete rows.
4. When `AXIOM_PACKETWOLF_URL` is set, read the live eBPF flow panel.
5. **Empty / fail:** No flows → PacketWolf unreachable; policies empty → create first.
6. **Success:** Graph + policies match cluster; seal on a workload shows isolation.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Services](services.md)
- [Workloads](../workloads/workloads.md)
- [Settings](../admin/settings.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
