# Ops hub

## Purpose

Day-2 ops tabs — GitOps/Helm/RBAC/Identity and related operator tools.

## When to use it

- Operate **Ops hub** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/ops`
- Nav: **Sidebar → Ops (`/ops` or `/ops/:tab`)**

## Operate from the console (UX)

1. Open `/ops` and switch tabs as labeled in the UI.
2. RBAC: **New Role** / **New RoleBinding**; Sync where offered.
3. Identity tab: Haven/Keycloak health when installed.
4. **Empty / fail:** Tab empty → optional component not installed.
5. **Success:** Sync/create toasts succeed; RBAC objects listed.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Users](users.md)
- [Settings](settings.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
