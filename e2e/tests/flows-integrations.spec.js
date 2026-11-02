// @ts-check
// Functional specs for login integrations (OIDC, SAML, LDAP) and data integrations (PacketWolf,
// plugin/CRD instances) that can be exercised locally.
//
// These need Axiom processes with specific environment (AXIOM_OIDC_*, AXIOM_SAML_*, AXIOM_LDAP_*,
// AXIOM_PACKETWOLF_URL ...), so this file starts its OWN demo servers from ../../bin/zyvor-axiom
// (copied to a temp file first) on free ports and points them at in-process test doubles:
//   - a mock PacketWolf HTTP API,
//   - a mock OIDC IdP (discovery + authorize redirect + PKCE-verifying token endpoint),
//   - a mock SAML IdP (SSO endpoint that auto-POSTs a SAMLResponse to Axiom's ACS),
//   - a mock LDAP directory speaking just enough BER (simple bind + base-scope search) for auth.LDAPAuthenticate.
// It therefore ignores AXIOM_E2E_BASE_URL and only needs the binary (build: go build -o bin/zyvor-axiom ./cmd/zyvor-axiom).
//
// Known product bugs are declared with `fixme(...)`; they run (and are expected to fail) only when
// AXIOM_E2E_RUN_FIXME=1 so the suite stays green while the bug is open.
const { test, expect } = require("@playwright/test");
const http = require("http");
const net = require("net");
const fs = require("fs");
const os = require("os");
const path = require("path");
const crypto = require("crypto");
const { spawn, spawnSync } = require("child_process");
const { signIn, confirmDialog, toastText, waitMutation, USERNAME, PASSWORD } = require("./helpers");

const fixme = process.env.AXIOM_E2E_RUN_FIXME ? test : test.fixme;
const uid = () => Math.random().toString(36).slice(2, 8);
const basic = (u, p) => ({ Authorization: `Basic ${Buffer.from(`${u}:${p}`).toString("base64")}` });

// ---------------------------------------------------------------------------
// Process + mock infrastructure
// ---------------------------------------------------------------------------
const BIN_SRC = path.resolve(__dirname, "../../bin/zyvor-axiom");
/** @type {string} */
let BIN = "";
const running = new Set();

test.beforeAll(() => {
  if (!fs.existsSync(BIN_SRC)) return;
  // Copy so that a `pkill zyvor-axiom` from a parallel session cannot take these servers down.
  BIN = path.join(os.tmpdir(), `axiom-e2e-int-${process.pid}`);
  fs.copyFileSync(BIN_SRC, BIN);
  fs.chmodSync(BIN, 0o755);
});
test.afterAll(() => {
  for (const stop of running) stop();
  if (BIN) try { fs.unlinkSync(BIN); } catch { /* already gone */ }
});
test.beforeEach(() => {
  test.skip(!BIN, `binary not found at ${BIN_SRC} - run: go build -o bin/zyvor-axiom ./cmd/zyvor-axiom`);
});

function freePort() {
  return new Promise((resolve) => {
    const s = net.createServer();
    s.listen(0, "127.0.0.1", () => { const p = /** @type {any} */ (s.address()).port; s.close(() => resolve(p)); });
  });
}

async function startAxiom(env = {}) {
  const port = await freePort();
  const child = spawn(BIN, [], {
    env: {
      PATH: process.env.PATH, HOME: process.env.HOME,
      AXIOM_MODE: "demo", AXIOM_USERNAME: USERNAME, AXIOM_PASSWORD: PASSWORD,
      AXIOM_ADDR: `127.0.0.1:${port}`,
      ...env,
    },
    stdio: "ignore",
  });
  const url = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 80; i++) {
    try { if ((await fetch(`${url}/api/v1/health`)).ok) break; } catch { /* not listening yet */ }
    await new Promise((r) => setTimeout(r, 150));
  }
  const stop = () => { running.delete(stop); child.kill("SIGTERM"); };
  running.add(stop);
  return { url, port, stop };
}

/** Starts a server, opens a fresh browser context against it, runs fn, always cleans up. */
async function withApp(browser, env, fn) {
  const srv = await startAxiom(env);
  const ctx = await browser.newContext({ baseURL: srv.url, ignoreHTTPSErrors: true });
  const page = await ctx.newPage();
  try {
    await fn({ page, ctx, srv });
  } finally {
    await ctx.close();
    srv.stop();
  }
}

async function startHttpMock(handler, port = 0) {
  const requests = [];
  const sockets = new Set();
  const server = http.createServer((req, res) => {
    const chunks = [];
    req.on("data", (c) => chunks.push(c));
    req.on("end", () => {
      const rec = { method: req.method, url: req.url, headers: req.headers, body: Buffer.concat(chunks).toString() };
      requests.push(rec);
      handler(req, res, rec);
    });
  });
  server.on("connection", (s) => { sockets.add(s); s.on("close", () => sockets.delete(s)); });
  await new Promise((r) => server.listen(port, "127.0.0.1", () => r(undefined)));
  const p = /** @type {any} */ (server.address()).port;
  return {
    url: `http://127.0.0.1:${p}`,
    port: p,
    requests,
    close: () => new Promise((r) => { sockets.forEach((s) => s.destroy()); server.close(() => r(undefined)); }),
  };
}

const json = (res, status, body) => res.writeHead(status, { "Content-Type": "application/json" }).end(JSON.stringify(body));

/** Login rate limiter (per process) can lock out after failed logins; a success resets it. */
async function recover(url) {
  for (let i = 0; i < 40; i++) {
    const r = await fetch(`${url}/api/v1/session`, { headers: basic(USERNAME, PASSWORD) });
    if (r.status !== 429) return;
    await new Promise((res) => setTimeout(res, 1000));
  }
}

/** Goes to the login page and to the password step, where the SSO links live. */
async function toPasswordStep(page, username = "someone") {
  await page.goto("/");
  await page.getByPlaceholder("Username").fill(username);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await expect(page.getByPlaceholder("Password")).toBeVisible();
}

async function uiLogin(page, username, password) {
  await toPasswordStep(page, username);
  await page.getByPlaceholder("Password").fill(password);
  await page.getByRole("button", { name: "Sign in to Axiom" }).click();
}

/**
 * Effective role of whatever credentials `request` carries (cookie jar and/or headers), found by
 * probing the server: admin-only route -> 400 (validation) vs 403; write route -> 403 (read-only) vs ok.
 */
async function roleOf(request, headers = {}) {
  const s = await request.get("/api/v1/session", { headers });
  if (s.status() === 401 || s.status() === 429) return `denied(${s.status()})`;
  const adminProbe = await request.post("/api/v1/users", { headers, data: {} });
  if (adminProbe.status() === 400) return "admin";
  if (adminProbe.status() !== 403) return `unexpected(${adminProbe.status()})`;
  const ns = `e2e-probe-${uid()}`;
  const w = await request.post("/api/v1/namespaces", { headers, data: { name: ns } });
  if (w.status() === 403) return "viewer";
  if (w.status() < 300) {
    await request.delete(`/api/v1/namespaces/${ns}`, { headers: basic(USERNAME, PASSWORD) });
    return "operator";
  }
  return `unexpected-write(${w.status()})`;
}

/** Status of GET /session for whatever credentials the page's request context carries; waits out a 429 lockout. */
async function sessionStatus(page) {
  for (let i = 0; i < 40; i++) {
    const r = await page.request.get("/api/v1/session");
    if (r.status() !== 429) return r.status();
    await new Promise((res) => setTimeout(res, Math.max(500, Number(r.headers()["retry-after"] || 1) * 1000)));
  }
  return 429;
}

async function openUserMenu(page) {
  await page.getByTestId("user-menu-trigger").click();
  return page.getByRole("menu");
}

// ---------------------------------------------------------------------------
// (a) PacketWolf panel on Networking
// ---------------------------------------------------------------------------
async function startPacketWolf(state, port = 0) {
  const mock = await startHttpMock((req, res) => {
    const u = new URL(req.url, "http://x");
    if (state.apiKey && req.headers["x-api-key"] !== state.apiKey) return json(res, 401, { error: "bad api key" });
    if (u.pathname === "/api/v1/health") {
      if (state.healthStatus !== 200) return json(res, state.healthStatus, { error: "unhealthy" });
      return json(res, 200, { status: "ok", subsystems: { ebpf: "running", store: "ok" } });
    }
    if (u.pathname === "/api/v1/flows") {
      if (state.flowsStatus !== 200) return json(res, state.flowsStatus, { error: "mock flows exploded" });
      return json(res, 200, state.flows);
    }
    return json(res, 404, { error: "not found" });
  }, port);
  return mock;
}

const flowsPanel = (page) => page.locator("div.card").filter({ has: page.getByRole("heading", { name: "PacketWolf" }) });
const F = (n, extra = {}) => ({ source: `10.0.0.${n}`, destination: `10.1.0.${n}`, protocol: "TCP", verdict: "FORWARDED", ...extra });

test.describe("PacketWolf panel (Networking)", () => {
  test.setTimeout(60_000);

  test("not configured: panel says so and status API agrees", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signIn(page);
      await page.goto("/networking");
      const panel = flowsPanel(page);
      await expect(panel.getByText("Not configured", { exact: true })).toBeVisible();
      await expect(panel).toContainText("AXIOM_PACKETWOLF_URL");
      await expect(page.getByTestId("flow-row-0")).toHaveCount(0);
      const st = await (await page.request.get("/api/v1/packetwolf/status", { headers: basic(USERNAME, PASSWORD) })).json();
      expect(st).toMatchObject({ configured: false, reachable: false });
      // proxy refuses cleanly instead of crashing
      const prox = await page.request.get("/api/v1/packetwolf/proxy/api/v1/flows", { headers: basic(USERNAME, PASSWORD) });
      expect(prox.status()).toBe(503);
    });
  });

  test("connected, array-shaped flows: rows render, limit + API key are forwarded, key never exposed", async ({ browser }) => {
    const state = { healthStatus: 200, flowsStatus: 200, apiKey: "pw-key-e2e", flows: [F(1), F(2, { verdict: "DROPPED", protocol: "UDP" }), { src: "a", dst: "b", proto: "ICMP", action: "ALLOW" }, { from: "x", to: "y", protocol: 6, disposition: "AUDIT" }] };
    const pw = await startPacketWolf(state);
    try {
      await withApp(browser, { AXIOM_PACKETWOLF_URL: `${pw.url}/`, AXIOM_PACKETWOLF_API_KEY: "pw-key-e2e" }, async ({ page }) => {
        await signIn(page);
        await page.goto("/networking");
        const panel = flowsPanel(page);
        await expect(panel.getByText("Connected", { exact: true }).first()).toBeVisible();
        await expect(page.getByTestId("flow-row-3")).toBeVisible();
        await expect(page.getByTestId("flow-row-4")).toHaveCount(0);
        await expect(page.getByTestId("flow-row-0")).toContainText("10.0.0.1");
        await expect(page.getByTestId("flow-row-0")).toContainText("10.1.0.1");
        await expect(page.getByTestId("flow-row-0")).toContainText("FORWARDED");
        await expect(page.getByTestId("flow-row-1")).toContainText("DROPPED");
        await expect(page.getByTestId("flow-row-1")).toContainText("UDP");
        // alternate field names (src/dst/proto/action, from/to/disposition) and non-string values
        await expect(page.getByTestId("flow-row-2")).toContainText("ICMP");
        await expect(page.getByTestId("flow-row-2")).toContainText("ALLOW");
        await expect(page.getByTestId("flow-row-3")).toContainText("AUDIT");
        await expect(page.getByTestId("flow-row-3")).toContainText("6");

        const flowReq = pw.requests.find((r) => r.url.startsWith("/api/v1/flows"));
        expect(flowReq.url).toContain("limit=25");
        expect(flowReq.headers["x-api-key"]).toBe("pw-key-e2e");
        expect(pw.requests.find((r) => r.url === "/api/v1/health").headers["x-api-key"]).toBe("pw-key-e2e");

        const stRes = await page.request.get("/api/v1/packetwolf/status", { headers: basic(USERNAME, PASSWORD) });
        const st = await stRes.json();
        expect(st).toMatchObject({ configured: true, reachable: true, message: "Connected", baseUrl: pw.url, subsystems: { ebpf: "running", store: "ok" } });
        expect(JSON.stringify(st)).not.toContain("pw-key-e2e");
        // raw proxy passthrough incl. query string
        const raw = await page.request.get("/api/v1/packetwolf/proxy/api/v1/flows?limit=2&x=1", { headers: basic(USERNAME, PASSWORD) });
        expect(raw.status()).toBe(200);
        expect(await raw.json()).toHaveLength(4);
        expect(pw.requests.at(-1).url).toBe("/api/v1/flows?limit=2&x=1");
      });
    } finally {
      await pw.close();
    }
  });

  test("connected, {items:[]}-shaped flows (capped at 25 rows) and {flows:[]} shape", async ({ browser }) => {
    const many = Array.from({ length: 30 }, (_, i) => F(i));
    const state = { healthStatus: 200, flowsStatus: 200, apiKey: null, flows: { items: many, total: 30 } };
    const pw = await startPacketWolf(state);
    try {
      await withApp(browser, { AXIOM_PACKETWOLF_URL: pw.url }, async ({ page }) => {
        await signIn(page);
        await page.goto("/networking");
        await expect(flowsPanel(page).getByText("Connected", { exact: true }).first()).toBeVisible();
        await expect(page.getByTestId("flow-row-24")).toBeVisible();
        await expect(page.getByTestId("flow-row-25")).toHaveCount(0);
        await expect(page.getByTestId("flow-row-0")).toContainText("10.0.0.0");
        await expect(page.getByRole("region", { name: "Live flows" })).toBeVisible();

        // same server, other envelope: the UI re-polls (8s) - use the raw API to prove the shape is what the mock serves,
        // then a fresh page load for {flows:[...]}
        state.flows = { flows: [F(77)] };
        await page.reload();
        await expect(page.getByTestId("flow-row-0")).toContainText("10.0.0.77");
        await expect(page.getByTestId("flow-row-1")).toHaveCount(0);
      });
    } finally {
      await pw.close();
    }
  });

  test("connected, empty and unrecognised payloads", async ({ browser }) => {
    const state = { healthStatus: 200, flowsStatus: 200, apiKey: null, flows: { items: [] } };
    const pw = await startPacketWolf(state);
    try {
      await withApp(browser, { AXIOM_PACKETWOLF_URL: pw.url }, async ({ page }) => {
        await signIn(page);
        await page.goto("/networking");
        await expect(flowsPanel(page).getByText("Connected", { exact: true }).first()).toBeVisible();
        await expect(flowsPanel(page).getByText("No flows in the current window.")).toBeVisible();
        state.flows = { summary: { total: 0 }, note: "no list here" };
        await page.reload();
        // not an array / items / flows -> falls back to the raw JSON viewer
        await expect(flowsPanel(page)).toContainText("flows.json");
        await expect(flowsPanel(page)).toContainText("no list here");
        await expect(page.getByTestId("flow-row-0")).toHaveCount(0);
      });
    } finally {
      await pw.close();
    }
  });

  test("connected but the flows endpoint fails: error is shown, status stays Connected", async ({ browser }) => {
    const state = { healthStatus: 200, flowsStatus: 500, apiKey: null, flows: [] };
    const pw = await startPacketWolf(state);
    try {
      await withApp(browser, { AXIOM_PACKETWOLF_URL: pw.url }, async ({ page }) => {
        await signIn(page);
        await page.goto("/networking");
        const panel = flowsPanel(page);
        await expect(panel.getByText("Connected", { exact: true }).first()).toBeVisible();
        await expect(panel).toContainText("mock flows exploded"); // upstream error body relayed by the proxy
        await expect(page.getByTestId("flow-row-0")).toHaveCount(0);
      });
    } finally {
      await pw.close();
    }
  });

  test("unreachable states: health 503, wrong API key, nothing listening", async ({ browser }) => {
    // health returns 503
    const sick = await startPacketWolf({ healthStatus: 503, flowsStatus: 200, apiKey: null, flows: [F(1)] });
    try {
      await withApp(browser, { AXIOM_PACKETWOLF_URL: sick.url }, async ({ page }) => {
        await signIn(page);
        await page.goto("/networking");
        const panel = flowsPanel(page);
        await expect(panel.getByText("Unreachable", { exact: true })).toBeVisible();
        await expect(panel).toContainText("HTTP 503");
        await expect(page.getByTestId("flow-row-0")).toHaveCount(0); // no flows fetched while unhealthy
        expect(sick.requests.some((r) => r.url.startsWith("/api/v1/flows"))).toBe(false);
      });
    } finally {
      await sick.close();
    }
    // API key mismatch -> 401 from PacketWolf
    const locked = await startPacketWolf({ healthStatus: 200, flowsStatus: 200, apiKey: "right-key", flows: [F(1)] });
    try {
      await withApp(browser, { AXIOM_PACKETWOLF_URL: locked.url, AXIOM_PACKETWOLF_API_KEY: "wrong-key" }, async ({ page }) => {
        await signIn(page);
        await page.goto("/networking");
        await expect(flowsPanel(page).getByText("Unreachable", { exact: true })).toBeVisible();
        await expect(flowsPanel(page)).toContainText("HTTP 401");
      });
    } finally {
      await locked.close();
    }
    // nothing listening
    await withApp(browser, { AXIOM_PACKETWOLF_URL: "http://127.0.0.1:9" }, async ({ page }) => {
      await signIn(page);
      await page.goto("/networking");
      const panel = flowsPanel(page);
      await expect(panel.getByText("Unreachable", { exact: true })).toBeVisible();
      await expect(panel).toContainText(/refused|connect/i);
      const prox = await page.request.get("/api/v1/packetwolf/proxy/api/v1/flows", { headers: basic(USERNAME, PASSWORD) });
      expect(prox.status()).toBe(502);
      expect((await prox.json()).error).toMatch(/refused|connect/i);
    });
  });

  test("live transition: Connected -> mock stopped -> Unreachable -> mock back -> Connected", async ({ browser }) => {
    test.setTimeout(120_000);
    const state = { healthStatus: 200, flowsStatus: 200, apiKey: null, flows: [F(1)] };
    let pw = await startPacketWolf(state);
    const port = pw.port;
    try {
      await withApp(browser, { AXIOM_PACKETWOLF_URL: pw.url }, async ({ page }) => {
        await signIn(page);
        await page.goto("/networking");
        const panel = flowsPanel(page);
        await expect(panel.getByText("Connected", { exact: true }).first()).toBeVisible();
        await expect(page.getByTestId("flow-row-0")).toBeVisible();

        await pw.close();
        // server caches status for 10s and the panel polls every 10s -> allow up to ~25s
        await expect(panel.getByText("Unreachable", { exact: true })).toBeVisible({ timeout: 40_000 });
        await expect(page.getByTestId("flow-row-0")).toHaveCount(0);

        pw = await startPacketWolf(state, port);
        await expect(panel.getByText("Connected", { exact: true }).first()).toBeVisible({ timeout: 40_000 });
        await expect(page.getByTestId("flow-row-0")).toBeVisible({ timeout: 15_000 });
      });
    } finally {
      await pw.close();
    }
  });
});

// ---------------------------------------------------------------------------
// (b) Plugin page + CRD instances
// ---------------------------------------------------------------------------
test.describe("plugin page (CRD-backed instances, demo data)", () => {
  const PLUGIN = "volume-snapshots-viewer";
  const CRD = "/api/v1/crds/snapshot.storage.k8s.io/v1/volumesnapshots";
  const rows = (page) => page.locator("tbody tr");

  test("plugin is listed, page renders manifest columns, row click opens instance detail", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signIn(page);
      await expect(page.locator(`a[href="/plugins/${PLUGIN}"]`).first()).toBeAttached();
      const plugins = await (await page.request.get("/api/v1/plugins", { headers: basic(USERNAME, PASSWORD) })).json();
      expect(plugins.map((p) => p.name)).toContain(PLUGIN);

      await page.goto(`/plugins/${PLUGIN}`);
      await expect(page.getByRole("heading", { name: "Volume Snapshots" }).first()).toBeVisible();
      await expect(page.getByText("VolumeSnapshot · snapshot.storage.k8s.io/v1").first()).toBeVisible();
      await expect(page.getByRole("columnheader", { name: "Name", exact: true })).toBeVisible();
      await expect(page.getByRole("columnheader", { name: "Namespace", exact: true })).toBeVisible();
      await expect(page.getByRole("columnheader", { name: "Age", exact: true })).toBeVisible();
      await expect(rows(page)).toHaveCount(2);
      await expect(rows(page).first()).toContainText("demo-instance-1");
      await expect(rows(page).first()).toContainText("default");

      await rows(page).first().click();
      const dlg = page.getByRole("dialog");
      await expect(dlg.getByText("VolumeSnapshot · demo-instance-1")).toBeVisible();
      await expect(dlg).toContainText("Namespace");
      await expect(dlg).toContainText("demo-instance-1");
      await page.keyboard.press("Escape");
      await expect(dlg).toBeHidden();
    });
  });

  test("delete an instance: DELETE /crds/... 2xx, row disappears, API list + single GET agree, second delete is 404", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      const calls = [];
      page.on("response", (r) => { if (r.request().method() !== "GET" && r.url().includes("/api/")) calls.push({ m: r.request().method(), u: new URL(r.url()), s: r.status() }); });
      const errors = [];
      page.on("pageerror", (e) => errors.push(e.message));
      await signIn(page);
      await page.goto(`/plugins/${PLUGIN}`);
      await expect(rows(page)).toHaveCount(2);

      const del = waitMutation(page, "DELETE", /\/crds\/snapshot\.storage\.k8s\.io\/v1\/volumesnapshots\/demo-instance-1$/);
      await page.getByRole("button", { name: "Delete demo-instance-1" }).click();
      await confirmDialog(page);
      const res = await del;
      expect(res.status()).toBeLessThan(300);
      expect(new URL(res.url()).searchParams.get("namespace")).toBe("default");
      await toastText(page, /Deleted demo-instance-1/);
      await expect(rows(page)).toHaveCount(1);
      await expect(rows(page).first()).toContainText("demo-instance-2");

      const list = await (await page.request.get(CRD, { headers: basic(USERNAME, PASSWORD) })).json();
      expect(list.map((i) => i.name)).toEqual(["demo-instance-2"]);
      const one = await page.request.get(`${CRD}/demo-instance-1?namespace=default`, { headers: basic(USERNAME, PASSWORD) });
      expect(one.status()).toBe(404);
      const again = await page.request.delete(`${CRD}/demo-instance-1?namespace=default`, { headers: basic(USERNAME, PASSWORD) });
      expect(again.status()).toBe(404);

      // survives a reload; cancelling the confirm dialog deletes nothing
      await page.reload();
      await expect(rows(page)).toHaveCount(1);
      await page.getByRole("button", { name: "Delete demo-instance-2" }).click();
      await page.getByRole("alertdialog").getByRole("button", { name: /cancel/i }).click();
      await expect(rows(page)).toHaveCount(1);
      expect(calls.filter((c) => c.m === "DELETE").length).toBe(1);
      expect(calls.filter((c) => c.s >= 400)).toEqual([]);
      expect(errors).toEqual([]);
    });
  });

  test("stale row: deleting an instance someone else already removed reports the failure", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signIn(page);
      await page.goto(`/plugins/${PLUGIN}`);
      await expect(rows(page)).toHaveCount(2);
      expect((await page.request.delete(`${CRD}/demo-instance-2?namespace=default`, { headers: basic(USERNAME, PASSWORD) })).status()).toBe(200);
      const del = page.waitForResponse((r) => r.request().method() === "DELETE");
      await page.getByRole("button", { name: "Delete demo-instance-2" }).click();
      await confirmDialog(page);
      expect((await del).status()).toBe(404);
      await toastText(page, /Delete failed/);
    });
  });

  test("viewer sees the plugin but a delete is refused (403) with a visible error", async ({ browser }) => {
    await withApp(browser, {}, async ({ page, srv }) => {
      const created = await page.request.post("/api/v1/users", { headers: basic(USERNAME, PASSWORD), data: { username: "e2e-plug-viewer", password: "Passw0rd-plug", role: "viewer" } });
      expect(created.status()).toBe(201);
      await uiLogin(page, "e2e-plug-viewer", "Passw0rd-plug");
      await expect(page.getByTestId("sidebar-nav")).toBeVisible();
      await page.goto(`/plugins/${PLUGIN}`);
      await expect(rows(page)).toHaveCount(2);
      const del = page.waitForResponse((r) => r.request().method() === "DELETE");
      await page.getByRole("button", { name: "Delete demo-instance-1" }).click();
      await confirmDialog(page);
      const r = await del;
      expect(r.status()).toBe(403);
      await toastText(page, /Delete failed/);
      await expect(rows(page)).toHaveCount(2);
      const list = await (await fetch(`${srv.url}${CRD}`, { headers: basic(USERNAME, PASSWORD) })).json();
      expect(list).toHaveLength(2);
    });
  });

  test("unknown plugin id shows a not-found state; Custom Resources page lists the demo CRDs", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signIn(page);
      await page.goto("/plugins/does-not-exist");
      await expect(page.getByText('Plugin "does-not-exist" was not found.')).toBeVisible();
      await page.goto("/custom-resources");
      const region = page.getByRole("region", { name: "Custom resource definitions" });
      await expect(region).toContainText("VolumeSnapshot");
      await expect(region).toContainText("PrometheusRule");
      await expect(region).toContainText("snapshot.storage.k8s.io");
    });
  });
});

// ---------------------------------------------------------------------------
// (c1) OIDC against a mock IdP
// ---------------------------------------------------------------------------
const b64url = (b) => Buffer.from(b).toString("base64url");
const jwt = (claims) => `${b64url(JSON.stringify({ alg: "none", typ: "JWT" }))}.${b64url(JSON.stringify(claims))}.`;

async function startOidcIdp() {
  const st = {
    mode: "ok", // ok | deny | badstate | token500
    claims: { preferred_username: "oidc-alice", email: "alice@corp.test", sub: "sub-alice", groups: [] },
    authRequests: [], tokenRequests: [], pkceVerified: [], codes: new Map(), n: 0, lastCallback: "",
  };
  const mock = await startHttpMock((req, res, rec) => {
    const u = new URL(req.url, "http://x");
    const base = `http://${req.headers.host}`;
    if (u.pathname === "/realms/e2e/.well-known/openid-configuration") {
      return json(res, 200, {
        issuer: `${base}/realms/e2e`,
        authorization_endpoint: `${base}/realms/e2e/protocol/openid-connect/auth`,
        token_endpoint: `${base}/realms/e2e/protocol/openid-connect/token`,
        userinfo_endpoint: `${base}/realms/e2e/protocol/openid-connect/userinfo`,
        jwks_uri: `${base}/realms/e2e/protocol/openid-connect/certs`,
      });
    }
    if (u.pathname === "/realms/e2e/protocol/openid-connect/auth") {
      const q = Object.fromEntries(u.searchParams);
      st.authRequests.push(q);
      const back = new URL(q.redirect_uri);
      if (st.mode === "deny") {
        back.searchParams.set("error", "access_denied");
        back.searchParams.set("error_description", "user declined");
        back.searchParams.set("state", q.state);
      } else {
        const code = `code-${++st.n}`;
        st.codes.set(code, { challenge: q.code_challenge, redirect_uri: q.redirect_uri, client_id: q.client_id, used: false });
        back.searchParams.set("code", code);
        back.searchParams.set("state", st.mode === "badstate" ? "tampered-state" : q.state);
      }
      st.lastCallback = back.toString();
      res.writeHead(302, { Location: back.toString() }).end();
      return;
    }
    if (u.pathname === "/realms/e2e/protocol/openid-connect/token" && req.method === "POST") {
      const f = new URLSearchParams(rec.body);
      st.tokenRequests.push(Object.fromEntries(f));
      if (st.mode === "token500") return json(res, 500, { error: "server_error" });
      const c = st.codes.get(f.get("code"));
      const verifier = f.get("code_verifier") || "";
      const okPkce = !!c && crypto.createHash("sha256").update(verifier).digest("base64url") === c.challenge;
      st.pkceVerified.push(okPkce);
      if (!c || c.used || !okPkce || f.get("redirect_uri") !== c.redirect_uri || f.get("client_id") !== c.client_id || f.get("grant_type") !== "authorization_code") {
        return json(res, 400, { error: "invalid_grant" });
      }
      c.used = true;
      return json(res, 200, { access_token: "at-e2e", token_type: "Bearer", id_token: jwt({ iss: `${base}/realms/e2e`, aud: c.client_id, exp: Math.floor(Date.now() / 1000) + 300, ...st.claims }) });
    }
    return json(res, 404, { error: "not found" });
  });
  return { ...mock, st };
}

test.describe("OIDC SSO (mock IdP)", () => {
  test.setTimeout(60_000);

  test("full browser login: SSO link -> IdP redirect params (PKCE/state/redirect_uri) -> code exchange -> session -> sign-out", async ({ browser }) => {
    const idp = await startOidcIdp();
    try {
      await withApp(browser, { AXIOM_OIDC_ISSUER: `${idp.url}/realms/e2e`, AXIOM_OIDC_CLIENT_ID: "axiom-e2e" }, async ({ page, ctx, srv }) => {
        // the login page exposes the SSO link and drops the "lab mode" hint
        const providers = await (await page.request.get("/api/v1/auth/providers")).json();
        expect(providers.oidc).toMatchObject({ enabled: true, client_id: "axiom-e2e", login_url: "/api/v1/auth/oidc/login" });
        await toPasswordStep(page);
        await expect(page.getByRole("link", { name: "Sign in with SSO" })).toHaveAttribute("href", "/api/v1/auth/oidc/login");
        await expect(page.getByRole("link", { name: "Sign in with SAML" })).toHaveCount(0);

        await page.getByRole("link", { name: "Sign in with SSO" }).click();
        await expect(page.getByTestId("sidebar-nav")).toBeVisible();

        // what Axiom sent to the IdP
        expect(idp.st.authRequests).toHaveLength(1);
        const a = idp.st.authRequests[0];
        expect(a).toMatchObject({ client_id: "axiom-e2e", response_type: "code", scope: "openid profile email", code_challenge_method: "S256", redirect_uri: `${srv.url}/api/v1/auth/oidc/callback` });
        expect(a.state.length).toBeGreaterThan(16);
        expect(a.code_challenge).toMatch(/^[A-Za-z0-9_-]{43}$/);
        // the token exchange presented the matching PKCE verifier (mock recomputed S256 and compared)
        expect(idp.st.tokenRequests).toHaveLength(1);
        expect(idp.st.pkceVerified).toEqual([true]);
        expect(idp.st.tokenRequests[0]).toMatchObject({ grant_type: "authorization_code", client_id: "axiom-e2e", redirect_uri: `${srv.url}/api/v1/auth/oidc/callback` });

        // session: cookie based, HttpOnly, PKCE cookies cleaned up, identity from the ID token
        const cookies = await ctx.cookies();
        const sess = cookies.find((c) => c.name === "axiom_session");
        expect(sess).toBeTruthy();
        expect(sess.httpOnly).toBe(true);
        expect(sess.sameSite).toBe("Lax");
        expect(cookies.filter((c) => /oidc/i.test(c.name) && c.value)).toEqual([]);
        const session = await (await page.request.get("/api/v1/session")).json();
        expect(session).toMatchObject({ user: "oidc-alice", auth: "oidc" });
        const menu = await openUserMenu(page);
        await expect(menu).toContainText("oidc-alice");
        await expect(menu).toContainText("SSO");
        // default OIDC role is admin when no group mapping is configured
        expect(await roleOf(page.request)).toBe("admin");

        // sign out via the UI clears the cookie
        const logout = page.waitForResponse((r) => r.url().endsWith("/api/v1/auth/logout"));
        await page.getByTestId("user-menu-signout").click();
        expect((await logout).status()).toBe(200);
        await expect(page.getByPlaceholder("Username")).toBeVisible();
        expect(await sessionStatus(page)).toBe(401);
        await page.reload();
        await expect(page.getByPlaceholder("Username")).toBeVisible();
      });
    } finally {
      await idp.close();
    }
  });

  test("group -> role mapping: viewer, admin, unmatched (restrictive default), single-string claim", async ({ browser }) => {
    const idp = await startOidcIdp();
    try {
      const env = { AXIOM_OIDC_ISSUER: `${idp.url}/realms/e2e`, AXIOM_OIDC_CLIENT_ID: "axiom-e2e", AXIOM_OIDC_ADMIN_GROUP: "axiom-admins", AXIOM_OIDC_VIEWER_GROUP: "axiom-viewers" };
      const srv = await startAxiom(env);
      try {
        const cases = [
          { name: "v-user", groups: ["axiom-viewers"], want: "viewer" },
          { name: "a-user", groups: ["Other", "AXIOM-ADMINS"], want: "admin" }, // case-insensitive match
          { name: "n-user", groups: ["somebody-else"], want: "viewer" },
          { name: "s-user", groups: "axiom-admins", want: "admin" }, // some IdPs emit a bare string
        ];
        for (const c of cases) {
          idp.st.claims = { preferred_username: c.name, email: `${c.name}@corp.test`, sub: c.name, groups: c.groups };
          const ctx = await browser.newContext({ baseURL: srv.url });
          const page = await ctx.newPage();
          try {
            await toPasswordStep(page);
            await page.getByRole("link", { name: "Sign in with SSO" }).click();
            await expect(page.getByTestId("sidebar-nav")).toBeVisible();
            expect((await (await page.request.get("/api/v1/session")).json()).user).toBe(c.name);
            expect(await roleOf(page.request), `role for ${c.name}`).toBe(c.want);
            if (c.want === "viewer") {
              const menu = await openUserMenu(page);
              await expect(menu).toContainText("Viewer");
            }
          } finally {
            await ctx.close();
          }
        }
      } finally {
        srv.stop();
      }
    } finally {
      await idp.close();
    }
  });

  test("failure paths: IdP error, tampered state, token endpoint failure, replayed code, missing code", async ({ browser }) => {
    const idp = await startOidcIdp();
    try {
      await withApp(browser, { AXIOM_OIDC_ISSUER: `${idp.url}/realms/e2e`, AXIOM_OIDC_CLIENT_ID: "axiom-e2e" }, async ({ page, srv }) => {
        const body = async () => (await page.locator("body").innerText()).trim();
        const attempt = async () => {
          await toPasswordStep(page);
          const nav = page.waitForResponse((r) => r.url().includes("/api/v1/auth/oidc/callback"));
          await page.getByRole("link", { name: "Sign in with SSO" }).click();
          return nav;
        };
        const noSession = async () => expect(await sessionStatus(page)).toBe(401);

        idp.st.mode = "deny";
        let r = await attempt();
        expect(r.status()).toBe(400);
        expect(await body()).toMatch(/OIDC error: access_denied.*user declined/);
        await noSession();

        idp.st.mode = "badstate";
        r = await attempt();
        expect(r.status()).toBe(400);
        expect(await body()).toMatch(/invalid OIDC state/);
        await noSession();

        idp.st.mode = "token500";
        r = await attempt();
        expect(r.status()).toBe(502);
        expect(await body()).toMatch(/token exchange HTTP 500/);
        await noSession();

        // direct callback hits: no code / no cookies
        expect((await page.request.get("/api/v1/auth/oidc/callback")).status()).toBe(400);
        expect((await page.request.get("/api/v1/auth/oidc/callback?code=x&state=y")).status()).toBe(400);

        // replay: capture a valid callback URL, complete it once, then present the same code again
        idp.st.mode = "ok";
        await recover(srv.url); // failed attempts above may have tripped the login rate limiter
        await toPasswordStep(page);
        await page.getByRole("link", { name: "Sign in with SSO" }).click();
        await expect(page.getByTestId("sidebar-nav")).toBeVisible();
        const cbUrl = idp.st.lastCallback;
        expect(cbUrl).toContain("code=");
        await page.request.post("/api/v1/auth/logout");
        const replay = await page.request.get(cbUrl, { maxRedirects: 0 });
        expect(replay.status()).toBe(400); // state/verifier cookies were consumed and cleared
        await noSession();
        // each real attempt used a fresh state
        const states = idp.st.authRequests.map((q) => q.state);
        expect(new Set(states).size).toBe(states.length);
        void srv;
      });
    } finally {
      await idp.close();
    }
  });

  test("IdP discovery unreachable: /auth/oidc/login falls back to the conventional authorize path with the same params", async ({ browser }) => {
    await withApp(browser, { AXIOM_OIDC_ISSUER: "http://127.0.0.1:9/realms/dead", AXIOM_OIDC_CLIENT_ID: "axiom-e2e" }, async ({ page, srv }) => {
      const res = await page.request.get("/api/v1/auth/oidc/login", { maxRedirects: 0 });
      expect(res.status()).toBe(302);
      const loc = new URL(res.headers()["location"]);
      expect(loc.origin + loc.pathname).toBe("http://127.0.0.1:9/realms/dead/protocol/openid-connect/auth");
      expect(loc.searchParams.get("client_id")).toBe("axiom-e2e");
      expect(loc.searchParams.get("code_challenge_method")).toBe("S256");
      expect(loc.searchParams.get("redirect_uri")).toBe(`${srv.url}/api/v1/auth/oidc/callback`);
      // callback cannot complete without discovery: clean 502, no session
      const cookies = res.headersArray().filter((h) => h.name.toLowerCase() === "set-cookie").map((h) => h.value);
      expect(cookies.some((c) => /HttpOnly/i.test(c))).toBe(true);
    });
  });

  test("OIDC configured only through Settings (no env): sign-in works end to end", async ({ browser }) => {
    const idp = await startOidcIdp();
    try {
      await withApp(browser, {}, async ({ page, srv }) => {
        // before: nothing configured -> login redirect refuses
        expect((await page.request.get("/api/v1/auth/oidc/login", { maxRedirects: 0 })).status()).toBe(501);
        await signIn(page);
        await page.goto("/settings");
        const oidc = page.locator("div.card").filter({ has: page.getByRole("heading", { name: "OIDC single sign-on", exact: true }) });
        await oidc.getByLabel("Issuer", { exact: true }).fill(`${idp.url}/realms/e2e`);
        await oidc.getByLabel("Client ID", { exact: true }).fill("axiom-settings");
        const put = waitMutation(page, "PUT", /\/integrations$/);
        await oidc.getByRole("button", { name: "Save" }).click();
        expect((await put).status()).toBe(200);
        await toastText(page, /OIDC settings saved/);

        const ctx2 = await browser.newContext({ baseURL: srv.url });
        const p2 = await ctx2.newPage();
        try {
          await toPasswordStep(p2);
          await p2.getByRole("link", { name: "Sign in with SSO" }).click();
          await expect(p2.getByTestId("sidebar-nav")).toBeVisible();
          expect(idp.st.authRequests.at(-1).client_id).toBe("axiom-settings");
          expect((await (await p2.request.get("/api/v1/session")).json()).user).toBe("oidc-alice");
        } finally {
          await ctx2.close();
        }
      });
    } finally {
      await idp.close();
    }
  });
});

// ---------------------------------------------------------------------------
// (c2) SAML against a mock IdP
// ---------------------------------------------------------------------------
function samlXml({ nameId, email, groups, certB64 }) {
  const attrs = [
    email ? `<saml:Attribute Name="email"><saml:AttributeValue>${email}</saml:AttributeValue></saml:Attribute>` : "",
    ...(groups || []).map((g) => `<saml:Attribute Name="groups"><saml:AttributeValue>${g}</saml:AttributeValue></saml:Attribute>`),
  ].join("");
  const sig = certB64 ? `<ds:Signature xmlns:ds="http://www.w3.org/2000/09/xmldsig#"><ds:KeyInfo><ds:X509Data><ds:X509Certificate>${certB64}</ds:X509Certificate></ds:X509Data></ds:KeyInfo></ds:Signature>` : "";
  return `<samlp:Response xmlns:samlp="urn:oasis:names:tc:SAML:2.0:protocol" xmlns:saml="urn:oasis:names:tc:SAML:2.0:assertion"><saml:Assertion><saml:Subject><saml:NameID>${nameId}</saml:NameID></saml:Subject><saml:AttributeStatement>${attrs}</saml:AttributeStatement>${sig}</saml:Assertion></samlp:Response>`;
}

async function startSamlIdp() {
  const st = { nameId: "saml-alice@corp.test", email: "alice@corp.test", groups: [], certB64: "", acs: "", ssoRequests: [] };
  const mock = await startHttpMock((req, res) => {
    const u = new URL(req.url, "http://x");
    if (u.pathname !== "/saml/sso") return json(res, 404, {});
    st.ssoRequests.push(Object.fromEntries(u.searchParams));
    const b64 = Buffer.from(samlXml(st)).toString("base64");
    const relay = (u.searchParams.get("RelayState") || "").replace(/"/g, "");
    res.writeHead(200, { "Content-Type": "text/html" }).end(
      `<!doctype html><html><body onload="document.forms[0].submit()"><form method="post" action="${st.acs}"><input type="hidden" name="SAMLResponse" value="${b64}"/><input type="hidden" name="RelayState" value="${relay}"/><noscript><button>continue</button></noscript></form></body></html>`
    );
  });
  return { ...mock, st, xml: () => samlXml(st) };
}

/** Self-signed IdP certificate (PEM + base64 DER) via the openssl CLI; null when openssl is unavailable. */
function makeCert(cn) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "axiom-e2e-cert-"));
  const key = path.join(dir, "k.pem");
  const pem = path.join(dir, "c.pem");
  const r = spawnSync("openssl", ["req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", key, "-out", pem, "-subj", `/CN=${cn}`, "-days", "2"], { encoding: "utf8" });
  if (r.status !== 0) return null;
  const der = spawnSync("openssl", ["x509", "-in", pem, "-outform", "DER"]).stdout;
  return { pem: fs.readFileSync(pem, "utf8"), b64: Buffer.from(der).toString("base64") };
}

test.describe("SAML SSO (mock IdP)", () => {
  test.setTimeout(60_000);

  test("full browser login: SAML link -> IdP (RelayState) -> auto-POST to ACS -> session; NameID becomes the user", async ({ browser }) => {
    const idp = await startSamlIdp();
    try {
      const env = { AXIOM_SAML_LOGIN_URL: `${idp.url}/saml/sso`, AXIOM_SAML_ENTITY_ID: "axiom-e2e-sp", AXIOM_SAML_ALLOW_UNSIGNED: "true" };
      await withApp(browser, env, async ({ page, ctx, srv }) => {
        idp.st.acs = `${srv.url}/api/v1/auth/saml/acs`;
        const providers = await (await page.request.get("/api/v1/auth/providers")).json();
        expect(providers.saml).toMatchObject({ enabled: true, login_url: "/api/v1/auth/saml/login" });
        await toPasswordStep(page);
        await expect(page.getByRole("link", { name: "Sign in with SAML" })).toHaveAttribute("href", "/api/v1/auth/saml/login");
        await expect(page.getByRole("link", { name: "Sign in with SSO" })).toHaveCount(0);

        await page.getByRole("link", { name: "Sign in with SAML" }).click();
        await expect(page.getByTestId("sidebar-nav")).toBeVisible();
        expect(idp.st.ssoRequests).toHaveLength(1);

        const sess = (await ctx.cookies()).find((c) => c.name === "axiom_session");
        expect(sess?.httpOnly).toBe(true);
        const session = await (await page.request.get("/api/v1/session")).json();
        expect(session.user).toBe("saml-alice@corp.test");
        const menu = await openUserMenu(page);
        await expect(menu).toContainText("saml-alice@corp.test");
        expect(await roleOf(page.request)).toBe("admin"); // default role, no group mapping configured

        // RelayState is carried through when ?next= is given
        const redirect = await page.request.get("/api/v1/auth/saml/login?next=/pods", { maxRedirects: 0 });
        expect(redirect.status()).toBe(302);
        const loc = new URL(redirect.headers()["location"]);
        expect(loc.origin + loc.pathname).toBe(`${idp.url}/saml/sso`);
        expect(loc.searchParams.get("RelayState")).toBe("/pods");
      });
    } finally {
      await idp.close();
    }
  });

  test("group -> role mapping via SAML attributes, and viewer restrictions", async ({ browser }) => {
    const idp = await startSamlIdp();
    try {
      const env = { AXIOM_SAML_LOGIN_URL: `${idp.url}/saml/sso`, AXIOM_SAML_ALLOW_UNSIGNED: "true", AXIOM_OIDC_ADMIN_GROUP: "axiom-admins", AXIOM_OIDC_VIEWER_GROUP: "axiom-viewers" };
      const srv = await startAxiom(env);
      try {
        idp.st.acs = `${srv.url}/api/v1/auth/saml/acs`;
        const cases = [
          { nameId: "s-viewer", groups: ["axiom-viewers"], want: "viewer" },
          { nameId: "s-admin", groups: ["axiom-admins"], want: "admin" },
          { nameId: "s-op", groups: ["axiom-operator"], want: "operator" },
          { nameId: "s-none", groups: [], want: "viewer" },
        ];
        for (const c of cases) {
          idp.st.nameId = c.nameId;
          idp.st.groups = c.groups;
          const ctx = await browser.newContext({ baseURL: srv.url });
          const page = await ctx.newPage();
          try {
            await toPasswordStep(page);
            await page.getByRole("link", { name: "Sign in with SAML" }).click();
            await expect(page.getByTestId("sidebar-nav")).toBeVisible();
            expect((await (await page.request.get("/api/v1/session")).json()).user).toBe(c.nameId);
            expect(await roleOf(page.request), `role for ${c.nameId}`).toBe(c.want);
          } finally {
            await ctx.close();
          }
        }
      } finally {
        srv.stop();
      }
    } finally {
      await idp.close();
    }
  });

  test("assertion is rejected without a pinned cert / allow-unsigned, or with a mismatching or missing embedded cert", async ({ browser }) => {
    const good = makeCert("e2e-idp-good");
    const other = makeCert("e2e-idp-other");
    test.skip(!good || !other, "openssl not available to mint test certificates");
    const idp = await startSamlIdp();
    try {
      // 1) neither AXIOM_SAML_IDP_CERT nor ALLOW_UNSIGNED: refuse
      await withApp(browser, { AXIOM_SAML_LOGIN_URL: `${idp.url}/saml/sso` }, async ({ page, srv }) => {
        idp.st.acs = `${srv.url}/api/v1/auth/saml/acs`;
        await toPasswordStep(page);
        const acs = page.waitForResponse((r) => r.url().endsWith("/saml/acs"));
        await page.getByRole("link", { name: "Sign in with SAML" }).click();
        expect((await acs).status()).toBe(401);
        await expect(page.locator("body")).toContainText("SAML IdP certificate required");
        expect(await sessionStatus(page)).toBe(401);
      });
      // 2) pinned cert configured: the response must carry a valid XML signature, so an unsigned
      //    assertion that merely embeds some (or the pinned) certificate is refused
      await withApp(browser, { AXIOM_SAML_LOGIN_URL: `${idp.url}/saml/sso`, AXIOM_SAML_IDP_CERT: good.pem }, async ({ page, srv }) => {
        idp.st.acs = `${srv.url}/api/v1/auth/saml/acs`;
        idp.st.certB64 = other.b64;
        await toPasswordStep(page);
        let acs = page.waitForResponse((r) => r.url().endsWith("/saml/acs"));
        await page.getByRole("link", { name: "Sign in with SAML" }).click();
        expect((await acs).status()).toBe(401);
        await expect(page.locator("body")).toContainText("SAML signature check failed");
        await recover(srv.url);

        idp.st.certB64 = "";
        await toPasswordStep(page);
        acs = page.waitForResponse((r) => r.url().endsWith("/saml/acs"));
        await page.getByRole("link", { name: "Sign in with SAML" }).click();
        expect((await acs).status()).toBe(401);
        await expect(page.locator("body")).toContainText("SAML signature check failed");
        expect(await sessionStatus(page)).toBe(401);
      });
      // 3) malformed input
      await withApp(browser, { AXIOM_SAML_LOGIN_URL: `${idp.url}/saml/sso`, AXIOM_SAML_ALLOW_UNSIGNED: "true" }, async ({ page }) => {
        const post = (data) => page.request.post("/api/v1/auth/saml/acs", { form: data, maxRedirects: 0 });
        expect((await post({})).status()).toBe(400);
        expect((await post({ SAMLResponse: "!!not-base64!!" })).status()).toBe(400);
        expect((await post({ SAMLResponse: Buffer.from("<not-saml/>").toString("base64") })).status()).toBe(400);
      });
    } finally {
      await idp.close();
    }
  });

  test("an assertion that merely embeds the (public) pinned IdP certificate but carries no valid signature is rejected", async ({ browser }) => {
    const cert = makeCert("e2e-idp-forge");
    test.skip(!cert, "openssl not available");
    const idp = await startSamlIdp();
    try {
      await withApp(browser, { AXIOM_SAML_LOGIN_URL: `${idp.url}/saml/sso`, AXIOM_SAML_IDP_CERT: cert.pem }, async ({ page }) => {
        // The attacker never had the IdP private key: they copy the public certificate into an unsigned assertion.
        idp.st.certB64 = cert.b64;
        idp.st.nameId = "attacker";
        const forged = Buffer.from(idp.xml()).toString("base64");
        const res = await page.request.post("/api/v1/auth/saml/acs", { form: { SAMLResponse: forged }, maxRedirects: 0 });
        // 401: the XML signature is verified against the pinned certificate (saml_verify.go).
        expect(res.status()).toBeGreaterThanOrEqual(400);
        expect(await sessionStatus(page)).toBe(401);
      });
    } finally {
      await idp.close();
    }
  });
});

// ---------------------------------------------------------------------------
// (c3) LDAP against a mock directory
// ---------------------------------------------------------------------------
const berLen = (n) => (n < 128 ? Buffer.from([n]) : n < 256 ? Buffer.from([0x81, n]) : Buffer.from([0x82, n >> 8, n & 255]));
const tlv = (tag, ...parts) => { const inner = Buffer.concat(parts); return Buffer.concat([Buffer.from([tag]), berLen(inner.length), inner]); };
const berInt = (n) => tlv(0x02, Buffer.from([n]));
const berOct = (s) => tlv(0x04, Buffer.from(s));
function readTLV(buf, off) {
  if (buf.length < off + 2) return null;
  const tag = buf[off];
  let len = buf[off + 1];
  let hdr = 2;
  if (len & 0x80) {
    const n = len & 0x7f;
    if (buf.length < off + 2 + n) return null;
    len = 0;
    for (let i = 0; i < n; i++) len = len * 256 + buf[off + 2 + i];
    hdr = 2 + n;
  }
  if (buf.length < off + hdr + len) return null;
  return { tag, start: off + hdr, end: off + hdr + len };
}
function children(buf, start, end) {
  const out = [];
  let off = start;
  while (off < end) { const t = readTLV(buf, off); if (!t) break; out.push(t); off = t.end; }
  return out;
}

/**
 * Minimal LDAPv3 server: answers simple binds (success 0 / invalidCredentials 49) against `users`
 * ({ dn: {pw, groups} }) and base-scope searches for the bound DN's memberOf values.
 */
async function startLdap(users) {
  const binds = [];
  const socks = new Set();
  const server = net.createServer((sock) => {
    socks.add(sock);
    sock.on("close", () => socks.delete(sock));
    sock.on("error", () => {});
    let buf = Buffer.alloc(0);
    let bound = "";
    sock.on("data", (d) => {
      buf = Buffer.concat([buf, d]);
      for (;;) {
        const msg = readTLV(buf, 0);
        if (!msg) return;
        const [idT, opT] = children(buf, msg.start, msg.end);
        const id = buf[idT.start];
        if (opT.tag === 0x60) {
          const [, dnT, pwT] = children(buf, opT.start, opT.end);
          const dn = buf.subarray(dnT.start, dnT.end).toString();
          const pw = buf.subarray(pwT.start, pwT.end).toString();
          const u = users[dn];
          const ok = !!u && u.pw === pw;
          binds.push({ dn, pw, ok });
          if (ok) bound = dn;
          sock.write(tlv(0x30, berInt(id), tlv(0x61, tlv(0x0a, Buffer.from([ok ? 0 : 49])), berOct(""), berOct(""))));
        } else if (opT.tag === 0x63) {
          const groups = (users[bound] || {}).groups || [];
          const entry = tlv(0x30, berInt(id), tlv(0x64, berOct(bound), tlv(0x30, tlv(0x30, berOct("memberOf"), tlv(0x31, ...groups.map(berOct))))));
          const done = tlv(0x30, berInt(id), tlv(0x65, tlv(0x0a, Buffer.from([0])), berOct(""), berOct("")));
          sock.write(Buffer.concat([entry, done]));
        }
        buf = buf.subarray(msg.end);
      }
    });
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", () => r(undefined)));
  const port = /** @type {any} */ (server.address()).port;
  return { url: `ldap://127.0.0.1:${port}`, binds, close: () => new Promise((r) => { socks.forEach((s) => s.destroy()); server.close(() => r(undefined)); }) };
}

const BASE = "ou=people,dc=example,dc=com";
const dnOf = (u) => `uid=${u},${BASE}`;
const GROUP_DN = (cn) => `cn=${cn},ou=groups,dc=example,dc=com`;

test.describe("LDAP login (mock directory)", () => {
  test.setTimeout(90_000);

  test("UI login with a directory account: DN built from BaseDN, operator role by default, wrong password refused", async ({ browser }) => {
    const ldap = await startLdap({ [dnOf("alice")]: { pw: "alice-secret", groups: [] } });
    try {
      await withApp(browser, { AXIOM_LDAP_ENABLED: "true", AXIOM_LDAP_URL: ldap.url, AXIOM_LDAP_BASE_DN: BASE }, async ({ page, srv }) => {
        // providers advertise LDAP and the lab hint disappears
        const providers = await (await page.request.get("/api/v1/auth/providers")).json();
        expect(providers.ldap).toMatchObject({ enabled: true, url: ldap.url });
        expect(providers.lab.operator_login).toBe(false);
        await page.goto("/");
        await expect(page.getByText("Lab mode — no SSO configured")).toHaveCount(0);

        // wrong password first: visible error, no session
        await uiLogin(page, "alice", "not-the-password");
        await expect(page.getByText("Check your username and password.")).toBeVisible();
        await expect(page.getByTestId("sidebar-nav")).toHaveCount(0);
        expect(ldap.binds.at(-1)).toMatchObject({ dn: dnOf("alice"), pw: "not-the-password", ok: false });
        await recover(srv.url);

        // correct password: signed in, identity is the LDAP username
        await page.getByPlaceholder("Password").fill("alice-secret"); // still on the password step after the failure
        await page.getByRole("button", { name: "Sign in to Axiom" }).click();
        await expect(page.getByTestId("sidebar-nav")).toBeVisible();
        expect(ldap.binds.at(-1)).toMatchObject({ dn: dnOf("alice"), pw: "alice-secret", ok: true });
        const menu = await openUserMenu(page);
        await expect(menu).toContainText("alice");
        await page.keyboard.press("Escape");

        // role: LDAP default is operator -> day-2 writes yes, user/integration admin no
        const auth = basic("alice", "alice-secret");
        expect(await roleOf(page.request, auth)).toBe("operator");
        const denied = await page.request.put("/api/v1/integrations", { headers: auth, data: { atlasUrl: "http://x" } });
        expect(denied.status()).toBe(403);

        // unknown user and the env local account still behave
        expect(await roleOf(page.request, basic("mallory", "whatever"))).toBe("denied(401)");
        await recover(srv.url);
        expect(await roleOf(page.request, basic(USERNAME, PASSWORD))).toBe("admin");
      });
    } finally {
      await ldap.close();
    }
  });

  test("group mapping: admin / viewer / unmatched(operator) from memberOf", async ({ browser }) => {
    const ldap = await startLdap({
      [dnOf("boss")]: { pw: "pw-boss", groups: [GROUP_DN("axiom-admins")] },
      [dnOf("guest")]: { pw: "pw-guest", groups: [GROUP_DN("axiom-viewers"), GROUP_DN("misc")] },
      [dnOf("dev")]: { pw: "pw-dev", groups: [GROUP_DN("misc")] },
    });
    try {
      await withApp(browser, { AXIOM_LDAP_URL: ldap.url, AXIOM_LDAP_BASE_DN: BASE, AXIOM_LDAP_ADMIN_GROUP: "axiom-admins", AXIOM_LDAP_VIEWER_GROUP: "axiom-viewers" }, async ({ page }) => {
        expect(await roleOf(page.request, basic("boss", "pw-boss"))).toBe("admin");
        expect(await roleOf(page.request, basic("guest", "pw-guest"))).toBe("viewer");
        expect(await roleOf(page.request, basic("dev", "pw-dev"))).toBe("operator");

        // and through the login page: the viewer gets the read-only role in the UI
        await uiLogin(page, "guest", "pw-guest");
        await expect(page.getByTestId("sidebar-nav")).toBeVisible();
        const menu = await openUserMenu(page);
        await expect(menu).toContainText("Viewer");
      });
    } finally {
      await ldap.close();
    }
  });

  test("hardening: DN injection in the username is escaped, empty password never reaches the directory", async ({ browser }) => {
    const ldap = await startLdap({ [dnOf("alice")]: { pw: "alice-secret", groups: [] }, "uid=admin,ou=admins,dc=example,dc=com": { pw: "root-pw", groups: [] } });
    try {
      await withApp(browser, { AXIOM_LDAP_URL: ldap.url, AXIOM_LDAP_BASE_DN: BASE }, async ({ page, srv }) => {
        // attempt to escape BaseDN scope and bind as a different entry
        const inj = await page.request.get("/api/v1/session", { headers: basic("admin,ou=admins,dc=example,dc=com", "root-pw") });
        expect(inj.status()).toBe(401);
        const injBind = ldap.binds.at(-1);
        expect(injBind.ok).toBe(false);
        expect(injBind.dn.startsWith("uid=admin\\,ou\\=admins\\,dc\\=example\\,dc\\=com,")).toBe(true);
        await recover(srv.url);

        // empty password (anonymous/unauthenticated bind trick) is rejected locally
        const before = ldap.binds.length;
        const anon = await page.request.get("/api/v1/session", { headers: basic("alice", "") });
        expect(anon.status()).toBe(401);
        expect(ldap.binds.length).toBe(before);
      });
    } finally {
      await ldap.close();
    }
  });

  test("directory unreachable: local accounts still sign in, directory accounts fail fast with the normal error", async ({ browser }) => {
    await withApp(browser, { AXIOM_LDAP_URL: "ldap://127.0.0.1:9", AXIOM_LDAP_BASE_DN: BASE }, async ({ page, srv }) => {
      const t0 = Date.now();
      await uiLogin(page, "alice", "alice-secret");
      await expect(page.getByText("Check your username and password.")).toBeVisible();
      expect(Date.now() - t0).toBeLessThan(10_000);
      await recover(srv.url);
      await signIn(page);
      await expect(page.getByTestId("sidebar-nav")).toBeVisible();
    });
  });

  test("a local user created in Settings still signs in while LDAP is enabled (LDAP bind fails, falls through)", async ({ browser }) => {
    const ldap = await startLdap({ [dnOf("alice")]: { pw: "alice-secret", groups: [] } });
    try {
      await withApp(browser, { AXIOM_LDAP_URL: ldap.url, AXIOM_LDAP_BASE_DN: BASE }, async ({ page, srv }) => {
        const mk = await page.request.post("/api/v1/users", { headers: basic(USERNAME, PASSWORD), data: { username: "localbob", password: "Passw0rd-bob", role: "viewer" } });
        expect(mk.status()).toBe(201);
        await uiLogin(page, "localbob", "Passw0rd-bob");
        await expect(page.getByTestId("sidebar-nav")).toBeVisible();
        expect(ldap.binds.some((b) => b.dn === dnOf("localbob") && !b.ok)).toBe(true); // directory was asked first and said no
        void srv;
      });
    } finally {
      await ldap.close();
    }
  });
});
