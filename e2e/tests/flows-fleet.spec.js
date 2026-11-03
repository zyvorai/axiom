// @ts-check
// Fleet health: the cross-cluster page, the per-cluster state in the switcher
// and Settings, and access control. Needs its OWN demo servers (the simulated
// outage is switched on with AXIOM_DEMO_FLEET_DOWN / AXIOM_DEMO_FLEET_DEGRADED),
// started from ../../bin/zyvor-axiom like flows-integrations.spec.js.
const { test, expect } = require("@playwright/test");
const net = require("net");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { spawn } = require("child_process");
const { USERNAME, PASSWORD } = require("./helpers");

const BIN_SRC = path.resolve(__dirname, "../../bin/zyvor-axiom");
/** @type {string} */
let BIN = "";
const running = new Set();
const basic = (u, p) => ({ Authorization: `Basic ${Buffer.from(`${u}:${p}`).toString("base64")}` });

test.beforeAll(() => {
  if (!fs.existsSync(BIN_SRC)) return;
  BIN = path.join(os.tmpdir(), `axiom-e2e-fleet-${process.pid}`);
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
    env: { PATH: process.env.PATH, HOME: process.env.HOME, AXIOM_MODE: "demo", AXIOM_USERNAME: USERNAME, AXIOM_PASSWORD: PASSWORD, AXIOM_ADDR: `127.0.0.1:${port}`, ...env },
    stdio: "ignore",
  });
  const url = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 80; i++) {
    try { if ((await fetch(`${url}/api/v1/health`)).ok) break; } catch { /* not listening yet */ }
    await new Promise((r) => setTimeout(r, 150));
  }
  const stop = () => { running.delete(stop); child.kill("SIGTERM"); };
  running.add(stop);
  return { url, stop };
}

async function withApp(browser, env, fn) {
  const srv = await startAxiom(env);
  const ctx = await browser.newContext({ baseURL: srv.url, ignoreHTTPSErrors: true });
  const page = await ctx.newPage();
  try { await fn({ page, srv }); } finally { await ctx.close(); srv.stop(); }
}

async function signInAs(page, user, pass) {
  await page.goto("/");
  await page.getByPlaceholder("Username").fill(user);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.getByPlaceholder("Password").fill(pass);
  await page.getByRole("button", { name: "Sign in to Axiom" }).click();
  await expect(page.getByTestId("sidebar-nav")).toBeVisible();
}

test.describe("fleet health", () => {
  test.setTimeout(60_000);

  test("every demo cluster is healthy by default, and Switch to changes the active cluster", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/fleet");
      await expect(page.getByTestId("fleet-page")).toBeVisible();
      await expect(page.getByTestId("fleet-summary-clusters")).toContainText("3");
      await expect(page.getByTestId("fleet-summary-healthy")).toContainText("3");
      await expect(page.getByTestId("fleet-summary-attention")).toContainText("0");
      for (const name of ["eu-west-1", "us-east-1", "ap-south-1"]) {
        await expect(page.getByTestId(`fleet-state-${name}`)).toContainText("Healthy");
      }
      // The active cluster is marked and has no switch button.
      await expect(page.getByTestId("fleet-cluster-eu-west-1")).toContainText("active");
      await expect(page.getByTestId("fleet-switch-eu-west-1")).toHaveCount(0);

      const act = page.waitForResponse((r) => r.request().method() === "POST" && /\/clusters\/us-east-1\/activate$/.test(r.url()));
      await page.getByTestId("fleet-switch-us-east-1").click();
      expect((await act).status()).toBe(200);
      await page.waitForLoadState("load");
      // Selection is per browser tab (a session header), so the page itself is the evidence.
      await expect(page.getByTestId("fleet-cluster-us-east-1")).toContainText("active");
      await expect(page.getByTestId("fleet-cluster-eu-west-1")).not.toContainText("active");
    });
  });

  test("an unreachable and a degraded cluster are called out, and Check now refreshes", async ({ browser }) => {
    await withApp(browser, { AXIOM_DEMO_FLEET_DOWN: "ap-south-1", AXIOM_DEMO_FLEET_DEGRADED: "us-east-1" }, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/fleet");
      await expect(page.getByTestId("fleet-summary-healthy")).toContainText("1");
      await expect(page.getByTestId("fleet-summary-attention")).toContainText("2");

      await expect(page.getByTestId("fleet-state-ap-south-1")).toContainText("Unreachable");
      await expect(page.getByTestId("fleet-detail-ap-south-1")).toContainText("connection refused");
      await expect(page.getByTestId("fleet-state-us-east-1")).toContainText("Degraded");
      await expect(page.getByTestId("fleet-detail-us-east-1")).toContainText("not ready");
      await expect(page.getByTestId("fleet-state-eu-west-1")).toContainText("Healthy");

      // The unreachable cluster shows no capacity numbers; the reachable ones do.
      await expect(page.getByTestId("fleet-cluster-ap-south-1")).not.toContainText("CPU");
      await expect(page.getByTestId("fleet-cluster-eu-west-1")).toContainText("CPU");

      // Out of band: the API agrees with the page.
      const api = await (await page.request.get("/api/v1/fleet", { headers: basic(USERNAME, PASSWORD) })).json();
      expect(api.summary).toMatchObject({ clusters: 3, healthy: 1, degraded: 1, unreachable: 1 });
      expect(api.clusters.find((c) => c.name === "ap-south-1").state).toBe("unreachable");

      // Check now POSTs a refresh and the page stays consistent.
      const refresh = page.waitForResponse((r) => r.request().method() === "POST" && r.url().endsWith("/api/v1/fleet/refresh"));
      await page.getByTestId("fleet-refresh").click();
      expect((await refresh).status()).toBe(200);
      await expect(page.getByTestId("fleet-state-ap-south-1")).toContainText("Unreachable");

      // Settings -> Clusters carries the same state.
      await page.goto("/settings#clusters");
      await expect(page.getByTestId("cluster-option-ap-south-1")).toContainText("Not");
      await expect(page.getByTestId("cluster-option-eu-west-1")).toContainText("Healthy");

      // The top-bar dot follows the ACTIVE cluster: switch to the dead one.
      await page.goto("/fleet");
      await page.getByTestId("fleet-switch-ap-south-1").click();
      await page.waitForLoadState("load");
      await page.goto("/overview");
      const chip = page.locator('a.cluster[data-testid="cluster-switcher"]'); // the top-bar chip (the sidebar switcher shares the test id)
      await expect(chip).toHaveAttribute("data-state", "unreachable");
      await expect(chip).toHaveAttribute("title", /connection refused/);
    });
  });

  test("namespace-restricted accounts cannot see the fleet; read-only viewers can", async ({ browser }) => {
    await withApp(browser, {}, async ({ page, srv }) => {
      const admin = basic(USERNAME, PASSWORD);
      const mk = async (username, body) => {
        const r = await page.request.post("/api/v1/users", { headers: admin, data: { username, password: "pw-pw-pw-pw", role: "viewer", ...body } });
        expect(r.status()).toBe(201);
      };
      await mk("fleet-viewer", {});
      await mk("fleet-scoped", { namespaces: ["payments"] });

      const get = (u) => fetch(`${srv.url}/api/v1/fleet`, { headers: basic(u, "pw-pw-pw-pw") });
      expect((await get("fleet-viewer")).status).toBe(200);
      expect((await get("fleet-scoped")).status).toBe(403);
      const refresh = await fetch(`${srv.url}/api/v1/fleet/refresh`, { method: "POST", headers: basic("fleet-viewer", "pw-pw-pw-pw") });
      expect(refresh.status).toBe(200);

      // The page tells the restricted person why instead of rendering an empty fleet.
      await signInAs(page, "fleet-scoped", "pw-pw-pw-pw");
      await page.goto("/fleet");
      await expect(page.getByRole("alert")).toContainText(/restricted|namespace/i);
      await expect(page.getByTestId("fleet-summary-clusters")).toHaveCount(0);
    });
  });
});
