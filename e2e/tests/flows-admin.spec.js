// @ts-check
// Functional specs for the admin surfaces: Users and Settings (integrations,
// connection tests, cluster registry). Same contract as flows-core.spec.js:
// drive the real UI, assert the mutating request returned 2xx, assert the UI
// reflects it, assert an independent API read agrees, and assert no unexpected
// failed mutations / console errors via track().
//
// Local test doubles (plain Node http/net servers in this process) stand in for
// Slack / webhook / Atlas / OIDC discovery / probe targets / Postgres so both a
// success and a failure result of every "Test" button can be exercised.
// http://127.0.0.1:9 (discard port, nothing listens) is the "unreachable" target.
//
// Known product bugs are declared with `fixme(...)` below; they run (and are
// expected to fail) only when AXIOM_E2E_RUN_FIXME=1, so the suite stays green
// while the bug is open but the correct-behaviour assertion is preserved.
const { test, expect } = require("@playwright/test");
const http = require("http");
const net = require("net");
const path = require("path");
const { spawn, spawnSync } = require("child_process");
const { signIn, track, apiGet, apiSend, waitMutation, confirmDialog, toastText } = require("./helpers");

const uid = () => Math.random().toString(36).slice(2, 8);
const fixme = process.env.AXIOM_E2E_RUN_FIXME ? test : test.fixme;
const UNREACHABLE = "http://127.0.0.1:9";

const basic = (u, p) => ({ Authorization: `Basic ${Buffer.from(`${u}:${p}`).toString("base64")}` });

/** API call authenticated as an arbitrary user (not the admin the helpers use). */
async function apiAs(page, user, pass, method, p, data) {
  const res = await page.request.fetch(`/api/v1${p}`, { method, headers: basic(user, pass), data });
  let body = null;
  try { body = await res.json(); } catch { /* non-JSON error body */ }
  return { status: res.status(), data: body };
}

/** Signs a *different* user in through the real login page in a fresh browser context. */
async function loginAs(browser, baseURL, username, password) {
  const ctx = await browser.newContext({ baseURL, ignoreHTTPSErrors: true });
  const page = await ctx.newPage();
  await page.goto("/");
  await page.getByPlaceholder("Username").fill(username);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.getByPlaceholder("Password").fill(password);
  await page.getByRole("button", { name: "Sign in to Axiom" }).click();
  return { ctx, page };
}

/**
 * The login rate limiter locks a client IP out (429, exponential backoff) after a few failed auth
 * attempts and any later success resets it. Tests that deliberately fail a login call this so the
 * next test does not start inside a lockout.
 */
async function recover(page) {
  for (let i = 0; i < 40; i++) {
    const res = await page.request.get("/api/v1/session", { headers: basic(process.env.AXIOM_E2E_USERNAME || "demo", process.env.AXIOM_E2E_PASSWORD || "demo") });
    if (res.status() !== 429) return;
    await new Promise((r) => setTimeout(r, Math.max(500, Number(res.headers()["retry-after"] || 1) * 1000)));
  }
}

async function startHttpMock(handler) {
  const requests = [];
  const server = http.createServer((req, res) => {
    const chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      const rec = { method: req.method, url: req.url, headers: req.headers, body: Buffer.concat(chunks).toString() };
      requests.push(rec);
      handler(req, res, rec);
    });
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", () => r(undefined)));
  const { port } = /** @type {import("net").AddressInfo} */ (server.address());
  return {
    url: `http://127.0.0.1:${port}`,
    port,
    requests,
    close: () => new Promise((r) => { server.closeAllConnections?.(); server.close(() => r(undefined)); }),
  };
}

/** A TCP listener that is *not* a Postgres server: accepts, then answers with garbage and hangs up. */
async function startTcpMock() {
  const conns = [];
  const socks = new Set();
  const server = net.createServer((sock) => { conns.push(1); socks.add(sock); sock.on("close", () => socks.delete(sock)); sock.on("error", () => {}); sock.end("not-postgres\n"); });
  await new Promise((r) => server.listen(0, "127.0.0.1", () => r(undefined)));
  const { port } = /** @type {import("net").AddressInfo} */ (server.address());
  return { port, conns, close: () => new Promise((r) => { socks.forEach((x) => x.destroy()); server.close(() => r(undefined)); }) };
}

/** Puts every integration field back to "not configured" (admin API), regardless of what a test did. */
async function resetIntegrations(page) {
  await apiSend(page, "PUT", "/integrations", {
    atlasUrl: "", atlasConsoleUrl: "", atlasTokenClear: true,
    oidcIssuer: "", oidcLoginUrl: "", oidcClientId: "",
    postgresHost: "", postgresPort: 5432, postgresDatabase: "", postgresUser: "", postgresPasswordClear: true, postgresSslMode: "disable",
    probePodNamespace: "", probePodName: "", probePodContainer: "", probeUrl: "",
    slackWebhookUrlClear: true, alertWebhookUrlClear: true, notifyOnWarnings: false,
  });
}

const card = (page, heading) =>
  page.locator("div.card").filter({ has: page.getByRole("heading", { name: heading, exact: true }) });

/** Clicks a card's test button and returns the parsed ConnectionTestResult the UI received. */
async function clickTest(page, scope, buttonName, service) {
  const resp = waitMutation(page, "POST", new RegExp(`/integrations/${service}/test$`));
  await scope.getByRole("button", { name: buttonName }).click();
  const r = await resp;
  expect(r.status()).toBe(200);
  return r.json();
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------
test.describe("users", () => {
  test.beforeEach(async ({ page }) => {
    await recover(page);
    await signIn(page);
    await page.goto("/users");
    await expect(page.getByRole("heading", { name: "Users", exact: true }).first()).toBeVisible();
  });

  async function openCreate(page) {
    await page.getByRole("button", { name: "Add user" }).first().click();
    const dlg = page.getByRole("dialog");
    await expect(dlg).toBeVisible();
    return dlg;
  }
  async function pickRole(page, dlg, selector, label) {
    await dlg.locator(selector).click();
    await page.getByRole("option", { name: new RegExp(`^${label}`) }).click();
  }
  // locator("tr") rather than getByRole("row"): rows are aria-hidden while a modal dialog is open.
  const rowOf = (page, name) => page.locator("tbody tr").filter({ hasText: name });

  test("create viewer -> restricted API access -> promote to admin -> delete -> sign-in rejected", async ({ page, browser, baseURL }) => {
    const t = track(page);
    const name = `e2e-u-${uid()}`;
    const pw = "Passw0rd-one";

    // --- create (UI) ---
    const dlg = await openCreate(page);
    await dlg.locator("#users-username").fill(name);
    await dlg.locator("#users-password").fill(pw);
    await dlg.locator("#users-restrict-to-namespaces").fill("default");
    const posted = waitMutation(page, "POST", /\/users$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    const postRes = await posted;
    expect(postRes.status()).toBe(201);
    await toastText(page, new RegExp(`User ${name} created`));
    await expect(dlg).toBeHidden();
    const row = rowOf(page, name);
    await expect(row).toContainText("viewer");
    await expect(row).toContainText("default");

    // --- API agrees, and the hash/password is never returned ---
    const list = await apiGet(page, "/users");
    const mine = list.data.find((u) => u.username === name);
    expect(mine).toMatchObject({ username: name, role: "viewer", namespaces: ["default"] });
    expect(JSON.stringify(list.data)).not.toMatch(/passwordHash|\$2[aby]\$|Passw0rd/);

    // --- the new account can really sign in through the login page ---
    const viewer = await loginAs(browser, baseURL, name, pw);
    try {
      await expect(viewer.page.getByTestId("sidebar-nav")).toBeVisible();
      await viewer.page.getByTestId("user-menu-trigger").click();
      await expect(viewer.page.getByText(name, { exact: true }).last()).toBeVisible();
      await expect(viewer.page.getByRole("menu").getByText("Viewer", { exact: true })).toBeVisible();
    } finally {
      await viewer.ctx.close();
    }

    // --- viewer is read-only and namespace-scoped (server-side enforcement) ---
    const ro = await apiAs(page, name, pw, "POST", "/namespaces", { name: `e2e-nope-${uid()}` });
    expect(ro.status).toBe(403);
    expect(ro.data.error).toMatch(/read-only/);
    expect((await apiAs(page, name, pw, "GET", "/pods?namespace=default")).status).toBe(200);
    const wide = await apiAs(page, name, pw, "GET", "/pods");
    expect(wide.status).toBe(403);
    expect(wide.data.error).toMatch(/restricted to specific namespaces/);

    // --- edit: viewer -> admin (UI) ---
    await page.getByRole("button", { name: `Edit user ${name}` }).click();
    const edit = page.getByRole("dialog");
    await expect(edit.getByRole("heading", { name: `Edit ${name}` })).toBeVisible();
    await pickRole(page, edit, "#users-role-2", "Admin");
    await expect(edit.locator("#users-restrict-to-namespaces-2")).toHaveCount(0); // admins are never namespace-scoped
    const patched = waitMutation(page, "PATCH", new RegExp(`/users/${name}$`));
    await edit.getByRole("button", { name: "Save" }).click();
    expect((await patched).status()).toBe(200);
    await toastText(page, new RegExp(`Updated ${name}`));
    await expect(row).toContainText("admin");
    await expect(row).toContainText("All namespaces");
    const after = (await apiGet(page, "/users")).data.find((u) => u.username === name);
    expect(after.role).toBe("admin");
    expect(after.namespaces ?? []).toEqual([]);
    // The promoted account now passes the admin-only guard: an empty create is a 400 (validation), not a 403.
    const adminProbe = await apiAs(page, name, pw, "POST", "/users", {});
    expect(adminProbe.status).toBe(400);

    // --- delete (UI) ---
    const del = waitMutation(page, "DELETE", new RegExp(`/users/${name}$`));
    await page.getByRole("button", { name: `Delete user ${name}` }).click();
    await confirmDialog(page);
    expect((await del).status()).toBeLessThan(300);
    await toastText(page, new RegExp(`Deleted ${name}`));
    await expect(row).toHaveCount(0);
    expect((await apiGet(page, "/users")).data.map((u) => u.username)).not.toContain(name);

    // --- the deleted account can no longer sign in (UI and API) ---
    const gone = await loginAs(browser, baseURL, name, pw);
    try {
      await expect(gone.page.getByText("Check your username and password.")).toBeVisible();
      await expect(gone.page.getByTestId("sidebar-nav")).toHaveCount(0);
    } finally {
      await gone.ctx.close();
    }
    expect((await apiAs(page, name, pw, "GET", "/session")).status).toBe(401);
    await recover(page);

    t.allow(/^POST \/api\/v1\/users$/); // none expected; kept explicit for the negative path below
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("operator role: create with namespaces, day-2 writes allowed, user/integration admin forbidden", async ({ page }) => {
    const t = track(page);
    const name = `e2e-op-${uid()}`;
    const pw = "Passw0rd-op1";
    const dlg = await openCreate(page);
    await dlg.locator("#users-username").fill(name);
    await dlg.locator("#users-password").fill(pw);
    await pickRole(page, dlg, "#users-role", "Operator");
    await dlg.locator("#users-restrict-to-namespaces").fill("team-a, team-b , team-a");
    const posted = waitMutation(page, "POST", /\/users$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await posted).status()).toBe(201);
    try {
      const mine = (await apiGet(page, "/users")).data.find((u) => u.username === name);
      // trimmed + de-duplicated by the server
      expect(mine).toMatchObject({ role: "operator", namespaces: ["team-a", "team-b"] });
      await expect(rowOf(page, name)).toContainText("operator");

      const noUsers = await apiAs(page, name, pw, "POST", "/users", { username: "x", password: "password1" });
      expect(noUsers.status).toBe(403);
      expect(noUsers.data.error).toMatch(/operator role cannot manage users, integrations, or cluster registry/);
      const noSettings = await apiAs(page, name, pw, "PUT", "/integrations", { atlasUrl: "http://x" });
      expect(noSettings.status).toBe(403);
      // Reads are fine.
      expect((await apiAs(page, name, pw, "GET", "/session")).status).toBe(200);
    } finally {
      await apiSend(page, "DELETE", `/users/${name}`);
    }
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("operator: edit dialog persists a namespace-restriction change (Users.jsx:128 only sends namespaces for role === 'viewer')", async ({ page }) => {
    const name = `e2e-opns-${uid()}`;
    const made = await apiSend(page, "POST", "/users", { username: name, password: "Passw0rd-op2", role: "operator", namespaces: ["team-a"] });
    expect(made.status).toBe(201);
    try {
      await page.goto("/users");
      await page.getByRole("button", { name: `Edit user ${name}` }).click();
      const edit = page.getByRole("dialog");
      // the dialog offers the field for operators ...
      const field = edit.locator("#users-restrict-to-namespaces-2");
      await expect(field).toBeVisible();
      await field.fill("team-b");
      // ... so saving must actually send it.
      const patched = page.waitForResponse((r) => r.request().method() === "PATCH" && r.url().endsWith(`/users/${name}`), { timeout: 4000 });
      await edit.getByRole("button", { name: "Save" }).click();
      // Actual: no request is sent at all (patch is empty because namespaces are skipped for operators) and the dialog just closes.
      expect((await patched).status()).toBe(200);
      const mine = (await apiGet(page, "/users")).data.find((u) => u.username === name);
      expect(mine.namespaces).toEqual(["team-b"]);
    } finally {
      await apiSend(page, "DELETE", `/users/${name}`);
    }
  });

  test("negative: duplicate name (case-insensitive), weak/empty input, blank username", async ({ page }) => {
    const t = track(page);
    t.allow(/^POST \/api\/v1\/users$/);
    const name = `e2e-dup-${uid()}`;
    const seed = await apiSend(page, "POST", "/users", { username: name, password: "Passw0rd-dup1", role: "viewer" });
    expect(seed.status).toBe(201);
    try {
      await page.reload();
      const dlg = await openCreate(page);
      const create = dlg.getByRole("button", { name: "Create", exact: true });

      // client-side gating: empty username / 7-char password keep Create disabled
      await expect(create).toBeDisabled();
      await dlg.locator("#users-username").fill(name.toUpperCase());
      await dlg.locator("#users-password").fill("1234567");
      await expect(create).toBeDisabled();
      await dlg.locator("#users-password").fill("12345678");
      await expect(create).toBeEnabled();

      // duplicate (different case) -> 409 and a visible error, dialog stays open, no second row
      const posted = waitMutation(page, "POST", /\/users$/);
      await create.click();
      const res = await posted;
      expect(res.status()).toBe(409);
      expect((await res.json()).error).toMatch(/already exists/);
      await toastText(page, /Create failed/);
      await expect(dlg).toBeVisible();
      await expect(rowOf(page, name)).toHaveCount(1);
      expect((await apiGet(page, "/users")).data.filter((u) => u.username.toLowerCase() === name).length).toBe(1);

      // whitespace-only username passes the button gate but is rejected by the server with a clean 400
      await dlg.locator("#users-username").fill("   ");
      const posted2 = waitMutation(page, "POST", /\/users$/);
      await create.click();
      const res2 = await posted2;
      expect(res2.status()).toBe(400);
      expect((await res2.json()).error).toMatch(/username is required/);
      await toastText(page, /Create failed/);
    } finally {
      await apiSend(page, "DELETE", `/users/${name}`);
    }
    // server-side validation independent of the UI
    const weak = await apiSend(page, "POST", "/users", { username: `e2e-weak-${uid()}`, password: "short", role: "viewer" });
    expect(weak.status).toBe(400);
    expect(weak.data.error).toMatch(/at least 8 characters/);
    expect((await apiSend(page, "DELETE", "/users/does-not-exist")).status).toBe(404);
    expect((await apiSend(page, "PATCH", "/users/does-not-exist", { role: "viewer" })).status).toBe(404);
    expect((await apiSend(page, "PATCH", `/users/${name}`, {})).status).toBe(400);
    expect(t.errors).toEqual([]);
  });

  test("password reset via edit dialog: weak rejected client-side, old password stops working, new one signs in", async ({ page, browser, baseURL }) => {
    const t = track(page);
    const name = `e2e-pw-${uid()}`;
    const oldPw = "OldPassw0rd!";
    const newPw = "NewPassw0rd!";
    expect((await apiSend(page, "POST", "/users", { username: name, password: oldPw, role: "viewer" })).status).toBe(201);
    try {
      await page.reload();
      await page.getByRole("button", { name: `Edit user ${name}` }).click();
      const edit = page.getByRole("dialog");
      const save = edit.getByRole("button", { name: "Save" });
      await edit.locator("#users-new-password").fill("short");
      await expect(save).toBeDisabled();
      await edit.locator("#users-new-password").fill(newPw);
      await expect(save).toBeEnabled();
      const patched = waitMutation(page, "PATCH", new RegExp(`/users/${name}$`));
      await save.click();
      expect((await patched).status()).toBe(200);
      await toastText(page, new RegExp(`Updated ${name}`));

      // new password works through the real login page; the old one does not
      const ok = await loginAs(browser, baseURL, name, newPw);
      try {
        await expect(ok.page.getByTestId("sidebar-nav")).toBeVisible();
      } finally {
        await ok.ctx.close();
      }
      expect((await apiAs(page, name, oldPw, "GET", "/session")).status).toBe(401);
      expect((await apiAs(page, name, newPw, "GET", "/session")).status).toBe(200);
      await recover(page);
    } finally {
      await apiSend(page, "DELETE", `/users/${name}`);
    }
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("delete of an already-removed user surfaces a clean error (stale row)", async ({ page }) => {
    const t = track(page);
    t.allow(/^DELETE \/api\/v1\/users\//);
    const name = `e2e-stale-${uid()}`;
    expect((await apiSend(page, "POST", "/users", { username: name, password: "Passw0rd-stale", role: "viewer" })).status).toBe(201);
    await page.reload();
    await expect(rowOf(page, name)).toBeVisible();
    expect((await apiSend(page, "DELETE", `/users/${name}`)).status).toBe(200); // removed behind the UI's back
    const del = waitMutation(page, "DELETE", new RegExp(`/users/${name}$`));
    await page.getByRole("button", { name: `Delete user ${name}` }).click();
    await confirmDialog(page);
    expect((await del).status()).toBe(404);
    await toastText(page, /Delete failed/);
    expect(t.errors).toEqual([]);
  });

  fixme("an unknown/typo'd role is rejected instead of silently becoming admin (platform.NormalizeRole)", async ({ page }) => {
    const name = `e2e-role-${uid()}`;
    const res = await apiSend(page, "POST", "/users", { username: name, password: "Passw0rd-role", role: "administrator-typo" });
    try {
      // Correct: 400 (or at worst the least-privileged role). Actual: 201 with role "admin".
      expect(res.status).toBe(400);
    } finally {
      if (res.status === 201) await apiSend(page, "DELETE", `/users/${name}`);
    }
  });

  test("usernames that can never authenticate are rejected (a ':' breaks HTTP Basic credential parsing)", async ({ page }) => {
    const name = `e2e:colon-${uid()}`;
    const res = await apiSend(page, "POST", "/users", { username: name, password: "Passw0rd-colon", role: "viewer" });
    try {
      // Correct: 400. Actual: 201, and the account can never sign in (Authorization: Basic splits on the first ':').
      expect(res.status).toBe(400);
    } finally {
      if (res.status === 201) await apiSend(page, "DELETE", `/users/${encodeURIComponent(name)}`);
    }
  });
});

// ---------------------------------------------------------------------------
// Settings: integrations + connection tests
// ---------------------------------------------------------------------------
test.describe("settings integrations", () => {
  test.beforeEach(async ({ page }) => {
    await recover(page);
    await signIn(page);
    await resetIntegrations(page);
    await page.goto("/settings");
    await expect(card(page, "Atlas")).toBeVisible();
  });
  test.afterEach(async ({ page }) => {
    await resetIntegrations(page);
  });

  test("OIDC: save persists (normalized), survives reload, test success + failure + bad discovery", async ({ page, browser, baseURL }) => {
    const t = track(page);
    let mode = "ok";
    const idp = await startHttpMock((req, res) => {
      if (mode === "404") { res.writeHead(404).end("nope"); return; }
      if (mode === "html") { res.writeHead(200, { "Content-Type": "text/html" }).end("<html>spa</html>"); return; }
      res.writeHead(200, { "Content-Type": "application/json" }).end(JSON.stringify({
        issuer: `${idp.url}/realms/e2e`,
        authorization_endpoint: `${idp.url}/realms/e2e/protocol/openid-connect/auth`,
        token_endpoint: `${idp.url}/realms/e2e/protocol/openid-connect/token`,
      }));
    });
    try {
      const oidc = card(page, "OIDC single sign-on");
      await expect(oidc).toContainText("not configured");
      // untested-yet: Test with nothing saved -> failure with a hint, not a crash
      const none = await clickTest(page, oidc, "Test connection", "oidc");
      expect(none.ok).toBe(false);
      expect(none.error).toMatch(/not configured/);
      await toastText(page, /OIDC test failed/);

      await oidc.getByLabel("Issuer", { exact: true }).fill(`${idp.url}/realms/e2e/`); // trailing slash is trimmed server-side
      await oidc.getByLabel("Login URL", { exact: true }).fill(`${idp.url}/realms/e2e/protocol/openid-connect/auth`);
      await oidc.getByLabel("Client ID", { exact: true }).fill("axiom-e2e");
      const put = waitMutation(page, "PUT", /\/integrations$/);
      await oidc.getByRole("button", { name: "Save" }).click();
      expect((await put).status()).toBe(200);
      await toastText(page, /OIDC settings saved/);
      await expect(oidc).toContainText("configured");
      await expect(oidc.getByLabel("Issuer", { exact: true })).toHaveValue(`${idp.url}/realms/e2e`);

      const view = (await apiGet(page, "/integrations")).data;
      expect(view).toMatchObject({ oidcIssuer: `${idp.url}/realms/e2e`, oidcClientId: "axiom-e2e", oidcConfigured: true });

      await page.reload();
      const again = card(page, "OIDC single sign-on");
      await expect(again.getByLabel("Issuer", { exact: true })).toHaveValue(`${idp.url}/realms/e2e`);
      await expect(again.getByLabel("Client ID", { exact: true })).toHaveValue("axiom-e2e");

      // success
      const okRes = await clickTest(page, again, "Test connection", "oidc");
      expect(okRes.ok).toBe(true);
      expect(okRes.detail.authorization_endpoint).toContain("/protocol/openid-connect/auth");
      await toastText(page, /OIDC reachable/);
      expect(idp.requests.some((r) => r.url === "/realms/e2e/.well-known/openid-configuration")).toBe(true);

      // failure modes: HTTP 404, non-discovery body, unreachable
      mode = "404";
      let r = await clickTest(page, again, "Test connection", "oidc");
      expect(r).toMatchObject({ ok: false });
      expect(r.error).toMatch(/HTTP 404/);
      await toastText(page, /OIDC test failed/);
      mode = "html";
      r = await clickTest(page, again, "Test connection", "oidc");
      expect(r.ok).toBe(false);
      expect(r.error).toMatch(/OpenID discovery JSON/);
      await again.getByLabel("Issuer", { exact: true }).fill(`${UNREACHABLE}/realms/e2e`);
      await again.getByRole("button", { name: "Save" }).click();
      await toastText(page, /OIDC settings saved/);
      r = await clickTest(page, again, "Test connection", "oidc");
      expect(r.ok).toBe(false);
      expect(r.error).toMatch(/connect|refused/i);

      // Saved OIDC config is what the login page + /auth/login redirect use.
      const anon = await browser.newContext({ baseURL, ignoreHTTPSErrors: true });
      try {
        const providers = await (await anon.request.get("/api/v1/auth/providers")).json();
        expect(providers.oidc.enabled).toBe(true);
        expect(providers.oidc.client_id).toBe("axiom-e2e");
        const p = await anon.newPage();
        await p.goto("/");
        await p.getByPlaceholder("Username").fill("someone");
        await p.getByRole("button", { name: "Continue", exact: true }).click();
        await expect(p.getByRole("link", { name: "Sign in with SSO" })).toHaveAttribute("href", "/api/v1/auth/oidc/login");
        await expect(p.getByText("Lab mode — no SSO configured")).toHaveCount(0);
      } finally {
        await anon.close();
      }
    } finally {
      await idp.close();
    }
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("Atlas: save normalizes URLs, persists across reload, test success + non-JSON + unreachable; token is API-only", async ({ page }) => {
    const t = track(page);
    let mode = "ok";
    const atlas = await startHttpMock((req, res) => {
      if (req.url === "/version" && mode === "ok") { res.writeHead(200, { "Content-Type": "application/json" }).end(JSON.stringify({ version: "9.9.9-e2e" })); return; }
      if (mode === "spa") { res.writeHead(200, { "Content-Type": "text/html" }).end("<html>SPA fallback</html>"); return; }
      res.writeHead(500).end("boom");
    });
    try {
      const c = card(page, "Atlas");
      await c.getByLabel("Atlas URL", { exact: true }).fill(`${atlas.url}/`);
      await c.getByLabel("Console URL", { exact: true }).fill("https://atlas.example.test/");
      const put = waitMutation(page, "PUT", /\/integrations$/);
      await c.getByRole("button", { name: "Save" }).click();
      expect((await put).status()).toBe(200);
      await toastText(page, /Atlas settings saved/);
      await expect(c.getByLabel("Atlas URL", { exact: true })).toHaveValue(atlas.url);
      const view = (await apiGet(page, "/integrations")).data;
      expect(view).toMatchObject({ atlasUrl: atlas.url, atlasConsoleUrl: "https://atlas.example.test", atlasConfigured: true, atlasTokenSet: false });
      expect(view).not.toHaveProperty("atlasToken");

      await page.reload();
      await expect(card(page, "Atlas").getByLabel("Atlas URL", { exact: true })).toHaveValue(atlas.url);
      await expect(card(page, "Atlas")).toContainText("configured");

      let r = await clickTest(page, card(page, "Atlas"), "Test connection", "atlas");
      expect(r.ok).toBe(true);
      expect(r.detail.version).toEqual({ version: "9.9.9-e2e" });
      await toastText(page, /Atlas reachable/);

      mode = "spa";
      r = await clickTest(page, card(page, "Atlas"), "Test connection", "atlas");
      expect(r.ok).toBe(false);
      expect(r.error).toMatch(/non-JSON/);
      await toastText(page, /Atlas test failed/);
      mode = "500";
      r = await clickTest(page, card(page, "Atlas"), "Test connection", "atlas");
      expect(r.ok).toBe(false);
      expect(r.error).toMatch(/500/);

      await card(page, "Atlas").getByLabel("Atlas URL", { exact: true }).fill(UNREACHABLE);
      await card(page, "Atlas").getByRole("button", { name: "Save" }).click();
      await toastText(page, /Atlas settings saved/);
      r = await clickTest(page, card(page, "Atlas"), "Test connection", "atlas");
      expect(r.ok).toBe(false);
      expect(r.error).toMatch(/connect|refused/i);

      // Token: not editable from the UI (no field), but stored + masked when set through the API.
      await expect(card(page, "Atlas").getByLabel(/token/i)).toHaveCount(0);
      const withTok = await apiSend(page, "PUT", "/integrations", { atlasToken: "tok-SECRET-123" });
      expect(withTok.status).toBe(200);
      expect(withTok.data.atlasTokenSet).toBe(true);
      expect(JSON.stringify((await apiGet(page, "/integrations")).data)).not.toContain("tok-SECRET-123");
    } finally {
      await atlas.close();
    }
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("Postgres: save persists, password masked + kept when left blank, test success (TCP) + auth failure + unreachable", async ({ page }) => {
    const t = track(page);
    const pg = await startTcpMock();
    const SECRET = "pg-SECRET-pass-42";
    try {
      const c = card(page, "Postgres");
      await c.getByLabel("Host", { exact: true }).fill("127.0.0.1");
      await c.getByLabel("Port", { exact: true }).fill(String(pg.port));
      await c.getByLabel("Database", { exact: true }).fill("platform");
      await c.getByLabel("SSL mode", { exact: true }).fill("disable");
      await c.getByLabel("Password", { exact: true }).fill(SECRET);
      const put = waitMutation(page, "PUT", /\/integrations$/);
      await c.getByRole("button", { name: "Save" }).click();
      const putRes = await put;
      expect(putRes.status()).toBe(200);
      await toastText(page, /Postgres settings saved/);
      // the PUT response and GET never echo the password; UI clears the field and says it is set
      expect(JSON.stringify(await putRes.json())).not.toContain(SECRET);
      const view = (await apiGet(page, "/integrations")).data;
      expect(view).toMatchObject({ postgresHost: "127.0.0.1", postgresPort: pg.port, postgresDatabase: "platform", postgresPasswordSet: true, postgresConfigured: true });
      expect(JSON.stringify(view)).not.toContain(SECRET);
      await expect(c.getByLabel(/^Password \(set/)).toHaveValue("");

      // Save again with the password blank: existing secret is kept (not wiped)
      await c.getByLabel("Database", { exact: true }).fill("platform2");
      await c.getByRole("button", { name: "Save" }).click();
      await toastText(page, /Postgres settings saved/);
      const kept = (await apiGet(page, "/integrations")).data;
      expect(kept).toMatchObject({ postgresDatabase: "platform2", postgresPasswordSet: true });

      await page.reload();
      const c2 = card(page, "Postgres");
      await expect(c2.getByLabel("Host", { exact: true })).toHaveValue("127.0.0.1");
      await expect(c2.getByLabel("Port", { exact: true })).toHaveValue(String(pg.port));
      await expect(c2.getByLabel("Database", { exact: true })).toHaveValue("platform2");
      await expect(c2.getByLabel(/^Password \(set/)).toHaveValue("");

      // With a user set the server attempts a real login against a listener that is not Postgres -> clean failure, no secret leaked
      await c2.getByLabel("User", { exact: true }).fill("axiom");
      await c2.getByRole("button", { name: "Save" }).click();
      await toastText(page, /Postgres settings saved/);
      let r = await clickTest(page, c2, "Test connection", "postgres");
      expect(r.ok).toBe(false);
      expect(r.error).toBeTruthy();
      expect(JSON.stringify(r)).not.toContain(SECRET);
      expect(pg.conns.length).toBeGreaterThan(0);
      await toastText(page, /Postgres test failed/);

      // No user -> TCP-only check succeeds against the listener
      await c2.getByLabel("User", { exact: true }).fill("");
      await c2.getByRole("button", { name: "Save" }).click();
      await toastText(page, /Postgres settings saved/);
      r = await clickTest(page, c2, "Test connection", "postgres");
      expect(r.ok).toBe(true);
      expect(r.detail.auth).toMatch(/skipped/);
      await toastText(page, /Postgres reachable/);

      // Unreachable port
      await c2.getByLabel("Port", { exact: true }).fill("9");
      await c2.getByRole("button", { name: "Save" }).click();
      await toastText(page, /Postgres settings saved/);
      r = await clickTest(page, c2, "Test connection", "postgres");
      expect(r.ok).toBe(false);
      expect(r.error).toMatch(/connect|refused/i);
    } finally {
      await pg.close();
    }
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("Probe: save persists, HTTP probe success / non-2xx / unreachable; pod-exec path in demo", async ({ page }) => {
    const t = track(page);
    let status = 200;
    const target = await startHttpMock((req, res) => { res.writeHead(status, { "Content-Type": "text/plain" }).end(status === 200 ? "healthy-e2e" : "broken"); });
    try {
      const c = card(page, "Probe");
      await c.getByLabel("Probe URL", { exact: true }).fill(`${target.url}/healthz`);
      const put = waitMutation(page, "PUT", /\/integrations$/);
      await c.getByRole("button", { name: "Save" }).click();
      expect((await put).status()).toBe(200);
      await toastText(page, /Probe settings saved/);
      expect((await apiGet(page, "/integrations")).data).toMatchObject({ probeUrl: `${target.url}/healthz`, probeConfigured: true });

      await page.reload();
      await expect(card(page, "Probe").getByLabel("Probe URL", { exact: true })).toHaveValue(`${target.url}/healthz`);

      let r = await clickTest(page, card(page, "Probe"), "Test connection", "probe");
      expect(r.ok).toBe(true);
      expect(r.detail).toMatchObject({ status: 200, preview: "healthy-e2e" });
      await toastText(page, /Probe reachable/);
      expect(target.requests.at(-1).url).toBe("/healthz");

      status = 503;
      r = await clickTest(page, card(page, "Probe"), "Test connection", "probe");
      expect(r.ok).toBe(false);
      expect(r.error).toBe("HTTP 503");
      await toastText(page, /Probe test failed/);

      await card(page, "Probe").getByLabel("Probe URL", { exact: true }).fill(`${UNREACHABLE}/healthz`);
      await card(page, "Probe").getByRole("button", { name: "Save" }).click();
      await toastText(page, /Probe settings saved/);
      r = await clickTest(page, card(page, "Probe"), "Test connection", "probe");
      expect(r.ok).toBe(false);
      expect(r.error).toMatch(/connect|refused/i);

      // Pod-exec path: in demo mode the provider fabricates exec output, so this only proves the wiring
      // (pod fields saved + routed to ExecCommand), NOT real reachability — see the report.
      const c2 = card(page, "Probe");
      await c2.getByLabel("Pod namespace", { exact: true }).fill("default");
      await c2.getByLabel("Pod name", { exact: true }).fill("web-1");
      await c2.getByRole("button", { name: "Save" }).click();
      await toastText(page, /Probe settings saved/);
      r = await clickTest(page, c2, "Test connection", "probe");
      expect(r.detail.from).toBe("default/web-1");
    } finally {
      await target.close();
    }
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("Notifications: secrets masked + kept, toggle persists, Test Slack / Test webhook success, non-2xx and unreachable", async ({ page }) => {
    const t = track(page);
    let code = 200;
    const slack = await startHttpMock((req, res) => { res.writeHead(code).end(code === 200 ? "ok" : "no_service"); });
    const hook = await startHttpMock((req, res) => { res.writeHead(code === 200 ? 204 : 502).end(); });
    const SLACK = `${slack.url}/services/T000/B000/SLACK-SECRET`;
    const HOOK = `${hook.url}/hooks/axiom-HOOK-SECRET`;
    try {
      const c = card(page, "Notifications");
      // nothing configured yet -> both tests fail cleanly
      let r = await clickTest(page, c, "Test Slack", "slack");
      expect(r).toMatchObject({ ok: false });
      expect(r.error).toMatch(/not configured/);
      await toastText(page, /Slack test failed/);
      r = await clickTest(page, c, "Test webhook", "webhook");
      expect(r.ok).toBe(false);
      await toastText(page, /Webhook test failed/);

      await c.getByLabel("Slack webhook URL", { exact: true }).fill(SLACK);
      await c.getByLabel("Alert webhook URL", { exact: true }).fill(HOOK);
      await c.getByRole("switch", { name: "Forward Warning events automatically" }).click();
      const put = waitMutation(page, "PUT", /\/integrations$/);
      await c.getByRole("button", { name: "Save" }).click();
      const putRes = await put;
      expect(putRes.status()).toBe(200);
      await toastText(page, /Notification settings saved/);

      // masked in the PUT response and in GET; only "...Set" flags come back
      for (const body of [JSON.stringify(await putRes.json()), JSON.stringify((await apiGet(page, "/integrations")).data)]) {
        expect(body).not.toContain("SLACK-SECRET");
        expect(body).not.toContain("HOOK-SECRET");
      }
      expect((await apiGet(page, "/integrations")).data).toMatchObject({ slackWebhookUrlSet: true, alertWebhookUrlSet: true, notifyOnWarnings: true, notifyConfigured: true });

      await page.reload();
      const c2 = card(page, "Notifications");
      await expect(c2.getByLabel(/^Slack webhook URL \(set/)).toHaveValue("");
      await expect(c2.getByLabel(/^Alert webhook URL \(set/)).toHaveValue("");
      await expect(c2.getByRole("switch", { name: "Forward Warning events automatically" })).toBeChecked();

      // saving again with blank secret fields keeps them
      await c2.getByRole("button", { name: "Save" }).click();
      await toastText(page, /Notification settings saved/);
      expect((await apiGet(page, "/integrations")).data).toMatchObject({ slackWebhookUrlSet: true, alertWebhookUrlSet: true });

      // success: the receiver actually got the payloads
      r = await clickTest(page, c2, "Test Slack", "slack");
      expect(r.ok).toBe(true);
      await toastText(page, /Slack message sent/);
      const s = slack.requests.at(-1);
      expect(s.method).toBe("POST");
      expect(s.url).toBe("/services/T000/B000/SLACK-SECRET");
      expect(JSON.parse(s.body).text).toMatch(/Axiom test notification/);
      r = await clickTest(page, c2, "Test webhook", "webhook");
      expect(r.ok).toBe(true);
      await toastText(page, /Webhook delivered/);
      expect(JSON.parse(hook.requests.at(-1).body)).toMatchObject({ source: "zyvor-axiom" });

      // receiver rejects -> failure surfaced, and the error text does not leak the secret path beyond the host
      code = 500;
      r = await clickTest(page, c2, "Test Slack", "slack");
      expect(r.ok).toBe(false);
      expect(r.error).toMatch(/HTTP 500/);
      await toastText(page, /Slack test failed/);
      r = await clickTest(page, c2, "Test webhook", "webhook");
      expect(r.ok).toBe(false);
      expect(r.error).toMatch(/HTTP 502/);

      // unreachable receiver
      await c2.getByLabel(/^Slack webhook URL/).fill(`${UNREACHABLE}/hook`);
      await c2.getByLabel(/^Alert webhook URL/).fill(`${UNREACHABLE}/hook`);
      await c2.getByRole("button", { name: "Save" }).click();
      await toastText(page, /Notification settings saved/);
      r = await clickTest(page, c2, "Test Slack", "slack");
      expect(r.ok).toBe(false);
      expect(r.error).toMatch(/connect|refused/i);
      r = await clickTest(page, c2, "Test webhook", "webhook");
      expect(r.ok).toBe(false);

      // turning the switch off persists
      await c2.getByRole("switch", { name: "Forward Warning events automatically" }).click();
      await c2.getByRole("button", { name: "Save" }).click();
      await toastText(page, /Notification settings saved/);
      expect((await apiGet(page, "/integrations")).data.notifyOnWarnings).toBe(false);
    } finally {
      await slack.close();
      await hook.close();
    }
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  fixme("Notifications: a saved Slack/webhook URL can be removed from the UI (Settings.jsx only sends non-empty secrets; the API's *Clear flags have no UI)", async ({ page }) => {
    expect((await apiSend(page, "PUT", "/integrations", { slackWebhookUrl: `${UNREACHABLE}/x` })).status).toBe(200);
    await page.reload();
    const c = card(page, "Notifications");
    await expect(c.getByLabel(/^Slack webhook URL \(set/)).toBeVisible();
    // Correct behaviour: some control (clear button / clearing the field + Save) unsets it.
    await c.getByLabel(/^Slack webhook URL/).fill("");
    await c.getByRole("button", { name: "Save" }).click();
    await expect.poll(async () => (await apiGet(page, "/integrations")).data.slackWebhookUrlSet).toBe(false);
  });

  async function withViewer(page, browser, baseURL, fn) {
    const name = `e2e-vs-${uid()}`;
    const pw = "Passw0rd-vs1";
    expect((await apiSend(page, "POST", "/users", { username: name, password: pw, role: "viewer" })).status).toBe(201);
    const v = await loginAs(browser, baseURL, name, pw);
    try {
      await expect(v.page.getByTestId("sidebar-nav")).toBeVisible();
      await v.page.goto("/settings");
      await expect(card(v.page, "Atlas")).toBeVisible();
      await fn(v.page);
    } finally {
      await v.ctx.close();
      await apiSend(page, "DELETE", `/users/${name}`);
    }
  }

  test("viewer: Save on Settings is refused (403) and reported", async ({ page, browser, baseURL }) => {
    await withViewer(page, browser, baseURL, async (vp) => {
      const atlas = card(vp, "Atlas");
      await atlas.getByLabel("Atlas URL", { exact: true }).fill("http://127.0.0.1:1");
      const saved = vp.waitForResponse((r) => r.request().method() === "PUT" && r.url().includes("/integrations"));
      await atlas.getByRole("button", { name: "Save" }).click();
      expect((await saved).status()).toBe(403);
      await expect(vp.locator("[data-sonner-toast]").filter({ hasText: /Save failed/ }).first()).toBeVisible();
    });
    expect((await apiGet(page, "/integrations")).data.atlasUrl).toBe("");
  });

  test("viewer: pressing Test connection shows feedback instead of failing silently (Settings.jsx test handlers have no catch)", async ({ page, browser, baseURL }) => {
    await withViewer(page, browser, baseURL, async (vp) => {
      const errors = [];
      vp.on("pageerror", (e) => errors.push(e.message));
      const atlas = card(vp, "Atlas");
      const tested = vp.waitForResponse((r) => r.request().method() === "POST" && r.url().includes("/integrations/atlas/test"));
      await atlas.getByRole("button", { name: "Test connection" }).click();
      expect((await tested).status()).toBe(403); // POSTs are blocked for read-only users, tests included
      // Correct: a toast (or inline message) explains it. Actual: nothing visible, and an unhandled promise rejection.
      await expect(vp.locator("[data-sonner-toast]").first()).toBeVisible({ timeout: 3000 });
      expect(errors).toEqual([]);
    });
  });
});

// ---------------------------------------------------------------------------
// Settings -> Clusters
// ---------------------------------------------------------------------------
const VALID_KUBECONFIG = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://cluster.example.invalid:6443
    insecure-skip-tls-verify: true
  name: e2e-cluster
contexts:
- context:
    cluster: e2e-cluster
    user: e2e-user
  name: e2e-context
current-context: e2e-context
users:
- name: e2e-user
  user:
    token: fake-token
`;

test.describe("settings clusters (demo build)", () => {
  test.beforeEach(async ({ page }) => {
    await recover(page);
    await signIn(page);
    await page.goto("/settings#clusters");
    await expect(page.getByRole("button", { name: "Add cluster" })).toBeVisible();
  });

  async function register(page, name, label, kubeconfig) {
    await page.getByRole("button", { name: "Add cluster" }).click();
    const dlg = page.getByRole("dialog");
    await dlg.getByLabel("Name", { exact: true }).fill(name);
    if (label) await dlg.getByLabel("Label", { exact: true }).fill(label);
    if (kubeconfig !== undefined) await dlg.getByLabel("Kubeconfig YAML").fill(kubeconfig);
    return dlg;
  }

  test("register a valid-looking kubeconfig: row appears, API agrees, can be activated; API validation for name/kubeconfig", async ({ page }) => {
    const t = track(page);
    const name = `e2e-c-${uid()}`;
    const dlg = await register(page, name, `E2E ${name}`, VALID_KUBECONFIG);
    const posted = waitMutation(page, "POST", /\/clusters$/);
    await dlg.getByRole("button", { name: "Register", exact: true }).click();
    const res = await posted;
    try {
      expect(res.status()).toBe(201);
      const body = await res.json();
      expect(body).toMatchObject({ ok: true, name, label: `E2E ${name}` });
      await toastText(page, new RegExp(`Cluster ${name} registered`));
      const row = page.getByTestId(`cluster-option-${name}`);
      await expect(row).toBeVisible();
      await expect(row).toContainText(`E2E ${name}`);
      const list = await apiGet(page, "/clusters");
      expect(list.data.map((c) => c.name)).toContain(name);
      expect(JSON.stringify(list.data)).not.toContain("fake-token"); // kubeconfig contents never echoed

      // activate through the UI
      const act = waitMutation(page, "POST", new RegExp(`/clusters/${name}/activate$`));
      await row.getByRole("button", { name: "Activate" }).click();
      expect((await act).status()).toBeLessThan(300);
      await expect(row).toContainText("active");

      // re-activate the original so the shared server is left as found
      const orig = page.getByTestId("cluster-option-eu-west-1");
      await orig.getByRole("button", { name: "Activate" }).click();
      await expect(orig).toContainText("active");
    } finally {
      const del = await apiSend(page, "DELETE", `/clusters/${name}`);
      expect(del.status).toBe(200);
    }
    expect((await apiGet(page, "/clusters")).data.map((c) => c.name)).not.toContain(name);
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("register form gating + server-side validation (empty name / kubeconfig)", async ({ page }) => {
    const t = track(page);
    t.allow(/^POST \/api\/v1\/clusters$/);
    const dlg = await register(page, "", "", undefined);
    const submit = dlg.getByRole("button", { name: "Register", exact: true });
    await expect(submit).toBeDisabled(); // no name, no kubeconfig
    await dlg.getByLabel("Name", { exact: true }).fill("only-name");
    await expect(submit).toBeDisabled(); // kubeconfig still empty
    await dlg.getByLabel("Kubeconfig YAML").fill("x");
    await expect(submit).toBeEnabled();

    // API contract, independent of the UI
    const noCfg = await apiSend(page, "POST", "/clusters", { name: "no-cfg", kubeconfig: "" });
    expect(noCfg.status).toBe(400);
    expect(noCfg.data.error).toMatch(/kubeconfig is required/);
    const noName = await apiSend(page, "POST", "/clusters", { name: " ", kubeconfig: "x" });
    expect(noName.status).toBe(400);
    expect(noName.data.error).toMatch(/name is required/);
    const ghost = await apiSend(page, "DELETE", "/clusters/never-registered");
    expect(ghost.status).toBeGreaterThanOrEqual(400);
    test.info().annotations.push({ type: "behaviour", description: `DELETE unknown cluster -> ${ghost.status} ${JSON.stringify(ghost.data)}` });
    expect(t.errors).toEqual([]);
  });

  test("register a clearly invalid kubeconfig (documents actual demo-build behaviour)", async ({ page }) => {
    const t = track(page);
    t.allow(/^POST \/api\/v1\/clusters$/);
    const name = `e2e-bad-${uid()}`;
    const dlg = await register(page, name, "", "this is: [not a kubeconfig");
    const posted = waitMutation(page, "POST", /\/clusters$/);
    await dlg.getByRole("button", { name: "Register", exact: true }).click();
    const res = await posted;
    try {
      if (res.status() >= 400) {
        // real (kubernetes-tagged) validation: clean 400 + visible error, dialog stays open, nothing registered
        expect(res.status()).toBe(400);
        expect((await res.json()).error).toMatch(/invalid kubeconfig/);
        await toastText(page, /Register failed/);
        await expect(dlg).toBeVisible();
        expect((await apiGet(page, "/clusters")).data.map((c) => c.name)).not.toContain(name);
      } else {
        // The non-kubernetes (demo) stub, remote_clusters_stub.go, never parses kubeconfig content and
        // accepts any non-empty text. Not a UI bug; the real validation is covered against a
        // kubernetes-tagged binary by the next test.
        test.info().annotations.push({ type: "behaviour", description: "demo build accepted an unparsable kubeconfig (stub does not validate)" });
        expect(res.status()).toBe(201);
      }
    } finally {
      await apiSend(page, "DELETE", `/clusters/${name}`);
    }
    expect(t.errors).toEqual([]);
  });
});

// The kubernetes-tagged binary validates kubeconfig content for real (demo data, real registry code).
test.describe("settings clusters (kubernetes build, real kubeconfig validation)", () => {
  test.setTimeout(240_000);
  /** @type {{url:string, stop:()=>void}|null} */
  let srv = null;

  async function freePort() {
    return new Promise((resolve) => { const s = net.createServer(); s.listen(0, "127.0.0.1", () => { const p = /** @type {any} */ (s.address()).port; s.close(() => resolve(p)); }); });
  }

  test.beforeAll(async () => {
    const root = path.resolve(__dirname, "../..");
    let bin = process.env.AXIOM_E2E_K8S_BIN || "";
    if (!bin) {
      bin = path.join(require("os").tmpdir(), `axiom-e2e-k8s-${process.pid}`);
      const b = spawnSync("go", ["build", "-tags", "kubernetes", "-o", bin, "./cmd/zyvor-axiom"], { cwd: root, encoding: "utf8", timeout: 200_000 });
      if (b.status !== 0) { bin = ""; test.info().annotations.push({ type: "skip-reason", description: `go build -tags kubernetes failed/unavailable: ${(b.stderr || b.error || "").toString().slice(0, 200)}` }); }
    }
    if (!bin) return;
    const port = await freePort();
    const child = spawn(bin, [], { env: { ...process.env, AXIOM_MODE: "demo", AXIOM_USERNAME: "demo", AXIOM_PASSWORD: "demo", AXIOM_ADDR: `127.0.0.1:${port}` }, stdio: "ignore" });
    const url = `http://127.0.0.1:${port}`;
    for (let i = 0; i < 60; i++) {
      try { if ((await fetch(`${url}/api/v1/health`)).ok) break; } catch { /* not up yet */ }
      await new Promise((r) => setTimeout(r, 250));
    }
    srv = { url, stop: () => { child.kill("SIGTERM"); } };
  });
  test.afterAll(() => { srv?.stop(); });

  test("invalid kubeconfig -> clean 400 + visible error; a syntactically valid one registers", async ({ browser }) => {
    test.skip(!srv, "kubernetes-tagged binary unavailable (need `go` or AXIOM_E2E_K8S_BIN)");
    const ctx = await browser.newContext({ baseURL: srv.url, ignoreHTTPSErrors: true });
    const page = await ctx.newPage();
    try {
      const t = track(page);
      t.allow(/^POST \/api\/v1\/clusters$/);
      await signIn(page);
      await page.goto("/settings#clusters");
      const before = (await apiGet(page, "/clusters")).data.map((c) => c.name);

      await page.getByRole("button", { name: "Add cluster" }).click();
      const dlg = page.getByRole("dialog");
      await dlg.getByLabel("Name", { exact: true }).fill("bad-cfg");
      await dlg.getByLabel("Kubeconfig YAML").fill("definitely: [not a kubeconfig");
      const bad = waitMutation(page, "POST", /\/clusters$/);
      await dlg.getByRole("button", { name: "Register", exact: true }).click();
      const badRes = await bad;
      expect(badRes.status()).toBe(400);
      expect((await badRes.json()).error).toMatch(/invalid kubeconfig/);
      await toastText(page, /Register failed/);
      await expect(dlg).toBeVisible(); // form kept so the user can fix it
      expect((await apiGet(page, "/clusters")).data.map((c) => c.name)).toEqual(before);

      // plain text that parses as YAML but is not a kubeconfig is also rejected cleanly
      await dlg.getByLabel("Kubeconfig YAML").fill("hello");
      const bad2 = waitMutation(page, "POST", /\/clusters$/);
      await dlg.getByRole("button", { name: "Register", exact: true }).click();
      expect((await bad2).status()).toBe(400);

      // valid-looking kubeconfig (building a client never dials the API server) -> registered
      await dlg.getByLabel("Name", { exact: true }).fill("e2e-real");
      await dlg.getByLabel("Kubeconfig YAML").fill(VALID_KUBECONFIG);
      const good = waitMutation(page, "POST", /\/clusters$/);
      await dlg.getByRole("button", { name: "Register", exact: true }).click();
      const goodRes = await good;
      expect(goodRes.status()).toBe(201);
      expect((await goodRes.json()).server).toBe("https://cluster.example.invalid:6443");
      const row = page.getByTestId("cluster-option-e2e-real");
      await expect(row).toBeVisible();
      await expect(row).toContainText("https://cluster.example.invalid:6443");
      expect((await apiSend(page, "DELETE", "/clusters/e2e-real")).status).toBe(200);
      expect(t.errors).toEqual([]);
    } finally {
      await ctx.close();
    }
  });
});

// Health of a registered cluster is never probed: SwitchableProvider.Clusters() reports
// `Healthy: e.provider != nil` (internal/platform/switchable_provider.go:103), so a cluster whose API
// server does not exist is shown with a green "Healthy" badge in Settings -> Clusters.
test.describe("settings clusters health (kubernetes build)", () => {
  test.setTimeout(240_000);
  fixme("an unreachable registered cluster is not reported Healthy", async ({ browser }) => {
    const root = path.resolve(__dirname, "../..");
    const bin = process.env.AXIOM_E2E_K8S_BIN || path.join(require("os").tmpdir(), `axiom-e2e-k8s-h-${process.pid}`);
    if (!process.env.AXIOM_E2E_K8S_BIN) spawnSync("go", ["build", "-tags", "kubernetes", "-o", bin, "./cmd/zyvor-axiom"], { cwd: root, timeout: 200_000 });
    const port = await new Promise((resolve) => { const s = net.createServer(); s.listen(0, "127.0.0.1", () => { const p = /** @type {any} */ (s.address()).port; s.close(() => resolve(p)); }); });
    const child = spawn(bin, [], { env: { ...process.env, AXIOM_MODE: "demo", AXIOM_USERNAME: "demo", AXIOM_PASSWORD: "demo", AXIOM_ADDR: `127.0.0.1:${port}` }, stdio: "ignore" });
    const url = `http://127.0.0.1:${port}`;
    for (let i = 0; i < 60; i++) { try { if ((await fetch(`${url}/api/v1/health`)).ok) break; } catch { /* starting */ } await new Promise((r) => setTimeout(r, 250)); }
    const ctx = await browser.newContext({ baseURL: url, ignoreHTTPSErrors: true });
    const page = await ctx.newPage();
    try {
      await signIn(page);
      expect((await apiSend(page, "POST", "/clusters", { name: "e2e-dead", kubeconfig: VALID_KUBECONFIG })).status).toBe(201);
      await page.goto("/settings#clusters");
      const row = page.getByTestId("cluster-option-e2e-dead");
      await expect(row).toBeVisible();
      await expect(row.getByText("Healthy", { exact: true })).toHaveCount(0);
    } finally {
      await ctx.close();
      child.kill("SIGTERM");
    }
  });
});
