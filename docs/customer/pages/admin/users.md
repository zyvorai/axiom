# Users

## Purpose

Local user accounts with admin or viewer roles.

## When to use it

- Operate **Users** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/users`
- Nav: **Sidebar → Users**

## Operate from the console (UX)

1. Open `/users` (admin).
2. Create users with admin/viewer; viewers read-only on mutating APIs.
3. **Empty / fail:** Persistence needs `zyvor-axiom-users` Secret in production.
4. **Success:** New user can sign in with the assigned role.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Settings](settings.md)
- [Login](../auth/login.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
