# Login

## Purpose

Sign in to the Axiom console — local Basic auth, SSO (OIDC/SAML), or lab demo helper.

## When to use it

- Operate **Login** when your job matches this page
- Prefer **Overview** after login if you are unsure where to start
- Confirm auth and activated cluster if inventories look empty

## How to get there

- Route: `/login`
- Nav: **Shown when unauthenticated → `/login`**

## Operate from the console (UX)

1. Open `https://<host>:30590/` and accept the self-signed certificate once if needed.
2. Sign-in is two steps: enter a username and **Continue**, then enter the password and **Sign in**. Use **Not you?** on the password screen to go back and change the username.
3. When SSO is off, click **Lab mode — fill demo** to fill both fields and jump straight to the password screen (lab default `demo` / `demo`).
4. Or enter operator credentials from Secret `zyvor-axiom-auth` / `AXIOM_USERNAME`+`AXIOM_PASSWORD`.
5. When SSO is configured, use the OIDC/SAML buttons (public client + PKCE) on the first screen instead.
6. **Empty / fail:** Sign in failed toast → check credentials / SSO redirect URI `/api/v1/auth/oidc/callback`; health: `curl -k https://<host>:30590/api/v1/health`.
7. **Success:** Redirect to **Overview**; cluster switcher and sidebar appear.

Console: `https://<host>:30590/` (NodePort HTTPS; override with `AXIOM_NODE_PORT`). Local demo: `http://<host>:5090` when `AXIOM_MODE=demo`. Never publish lab IPs — use `<host>`.

## Related pages

- [Overview](../console/overview.md)
- [Getting Started](../console/getting-started.md)
- [Getting Started](../../getting-started.md)
- [Page index](../../PAGE_INDEX.md)
