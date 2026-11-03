// @ts-check
// Follow-ups: cross-cluster search, applying a rightsizing recommendation, GitOps
// application deletion, QR enrolment and the require-MFA policy. Each runs on its
// OWN demo server (started from ../../bin/zyvor-axiom like flows-fleet.spec.js) so
// destructive actions and server-wide policy never disturb the shared demo state.
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
  BIN = path.join(os.tmpdir(), `axiom-e2e-followups-${process.pid}`);
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


const crypto = require("crypto");
const B32 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
function b32decode(s) {
  let bits = "";
  for (const c of s.replace(/[\s=-]/g, "").toUpperCase()) bits += B32.indexOf(c).toString(2).padStart(5, "0");
  const out = [];
  for (let i = 0; i + 8 <= bits.length; i += 8) out.push(parseInt(bits.slice(i, i + 8), 2));
  return Buffer.from(out);
}
function totp(secret, stepOffset = 0) {
  const counter = Math.floor(Date.now() / 30000) + stepOffset;
  const msg = Buffer.alloc(8);
  msg.writeBigUInt64BE(BigInt(counter));
  const h = crypto.createHmac("sha1", b32decode(secret)).update(msg).digest();
  const o = h[h.length - 1] & 0x0f;
  const n = ((h[o] & 0x7f) << 24) | (h[o + 1] << 16) | (h[o + 2] << 8) | h[o + 3];
  return String(n % 1_000_000).padStart(6, "0");
}
const ADMIN = basic(USERNAME, PASSWORD);

test.describe("cross-cluster search", () => {
  test.setTimeout(90_000);

  test("finds a workload in every cluster, skips a dead one, and Open switches to the cluster that has it", async ({ browser }) => {
    await withApp(browser, { AXIOM_DEMO_FLEET_DOWN: "ap-south-1" }, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/fleet");
      await page.getByTestId("fleet-search").fill("payments-api");

      // Two reachable clusters answer; the unreachable one is named, not silently dropped.
      await expect(page.getByTestId("fleet-hit-eu-west-1-Workload-payments-payments-api")).toBeVisible();
      await expect(page.getByTestId("fleet-hit-us-east-1-Workload-payments-payments-api")).toBeVisible();
      await expect(page.getByTestId("fleet-hit-ap-south-1-Workload-payments-payments-api")).toHaveCount(0);
      await expect(page.getByTestId("fleet-search-skipped")).toContainText("ap-south-1");

      // The kind filter narrows to services.
      await page.getByTestId("fleet-search-kind").selectOption("Service");
      await expect(page.getByTestId("fleet-search-results")).toContainText("Service");
      await expect(page.getByTestId("fleet-search-results")).not.toContainText("Deployment");

      // Out of band: the API agrees.
      const api = await (await page.request.get("/api/v1/fleet/search?q=payments-api", { headers: ADMIN })).json();
      expect(api.clusters.skipped.map((s) => s.cluster)).toEqual(["ap-south-1"]);
      expect(new Set(api.results.map((r) => r.cluster))).toEqual(new Set(["eu-west-1", "us-east-1"]));

      // Open on the other cluster's hit: switches this tab's cluster and lands on the workload.
      await page.getByTestId("fleet-search-kind").selectOption("Workload");
      await page.getByTestId("fleet-hit-open-us-east-1-Workload-payments-payments-api").click();
      await page.waitForURL(/\/workloads\/payments\/deployment\/payments-api/);
      await expect(page.locator('a.cluster[data-testid="cluster-switcher"]')).toContainText("zyvor-demo-us");

      // Bad input is refused.
      expect((await page.request.get("/api/v1/fleet/search?q=a", { headers: ADMIN })).status()).toBe(400);
      expect((await page.request.get("/api/v1/fleet/search?q=ok&kind=Pod", { headers: ADMIN })).status()).toBe(400);
    });
  });
});

test.describe("apply a rightsizing recommendation", () => {
  test.setTimeout(90_000);

  test("review the exact change, apply it, and the container is right-sized afterwards", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/ops/cost");
      const row = "payments-payments-api-api";
      await expect(page.getByTestId(`cost-rs-verdict-${row}`)).toContainText("Over-provisioned");

      await page.getByTestId(`cost-rs-apply-${row}`).click();
      const dlg = page.getByTestId("cost-apply-dialog");
      // The dry run shows exactly what would change before anything is written.
      await expect(dlg.getByTestId("cost-apply-changes")).toContainText("cpu requests");
      await expect(dlg.getByTestId("cost-apply-changes")).toContainText("→");
      const before = (await (await page.request.get("/api/v1/cost/rightsizing", { headers: ADMIN })).json()).items.find((i) => i.workload === "payments-api" && i.container === "api");
      expect(before.verdict).toBe("over");

      // Unticking memory re-previews with CPU only.
      await dlg.getByTestId("cost-apply-memory").uncheck();
      await expect(dlg.getByTestId("cost-apply-changes")).not.toContainText("memory");
      await dlg.getByTestId("cost-apply-memory").check();

      const applied = page.waitForResponse((r) => r.request().method() === "POST" && r.url().endsWith("/cost/rightsizing/apply") && r.request().postDataJSON().dryRun === false);
      await dlg.getByTestId("cost-apply-confirm").click();
      const res = await applied;
      expect(res.status()).toBe(200);
      // The request carried no target numbers: the server decided them.
      const sent = res.request().postDataJSON();
      expect(Object.keys(sent).sort()).toEqual(["container", "dryRun", "kind", "namespace", "observed", "resources", "window", "workload"]);

      // Afterwards the container is right-sized; the row leaves "needs attention"; a second apply is refused.
      await expect(page.getByTestId(`cost-rs-${row}`)).toHaveCount(0);
      const after = (await (await page.request.get("/api/v1/cost/rightsizing", { headers: ADMIN })).json()).items.find((i) => i.workload === "payments-api" && i.container === "api");
      expect(after.verdict).toBe("ok");
      expect(after.cpu.requestMilli).toBe(after.cpu.recommendedMilli);
      const again = await page.request.post("/api/v1/cost/rightsizing/apply", { headers: ADMIN, data: { namespace: "payments", kind: "Deployment", workload: "payments-api", container: "api", resources: ["cpu"], dryRun: true } });
      expect(again.status()).toBe(409);
    });
  });

  test("a stale view is refused, viewers cannot apply, and the server never takes target numbers", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      const base = { namespace: "payments", kind: "Deployment", workload: "payments-api", container: "api", resources: ["cpu"], dryRun: true };
      const stale = await page.request.post("/api/v1/cost/rightsizing/apply", { headers: ADMIN, data: { ...base, observed: { cpuRequestMilli: 1, memoryRequestMiB: 1 } } });
      expect(stale.status()).toBe(409);
      expect(await stale.text()).toContain("changed since you looked");
      const injected = await page.request.post("/api/v1/cost/rightsizing/apply", { headers: ADMIN, data: { ...base, cpuRequestMilli: 5 } });
      expect(injected.status()).toBe(400);
      const mk = await page.request.post("/api/v1/users", { headers: ADMIN, data: { username: "rs-viewer", password: "pw-pw-pw-pw", role: "viewer" } });
      expect(mk.status()).toBe(201);
      const viewer = await page.request.post("/api/v1/cost/rightsizing/apply", { headers: basic("rs-viewer", "pw-pw-pw-pw"), data: base });
      expect(viewer.status()).toBe(403);
    });
  });
});

test.describe("delete GitOps applications", () => {
  test.setTimeout(90_000);

  test("a cascading delete says what it removes and needs the name typed; a plain one does not", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/ops/gitops");

      // Argo app with the resources finalizer: deleting also removes what it deployed.
      await page.getByTestId("gitops-delete-Application-argocd-platform-apps").click();
      const dlg = page.getByTestId("gitops-delete-dialog");
      await expect(dlg.getByTestId("gitops-delete-consequence")).toContainText(/also delete|resources/i);
      await expect(dlg.getByTestId("gitops-delete-confirm")).toBeDisabled();
      await dlg.getByTestId("gitops-delete-confirm-input").fill("platform");
      await expect(dlg.getByTestId("gitops-delete-confirm")).toBeDisabled();
      await dlg.getByTestId("gitops-delete-confirm-input").fill("platform-apps");
      await expect(dlg.getByTestId("gitops-delete-confirm")).toBeEnabled();
      const deleted = page.waitForResponse((r) => r.request().method() === "DELETE" && /\/gitops\/Application\/argocd\/platform-apps/.test(r.url()));
      await dlg.getByTestId("gitops-delete-confirm").click();
      expect((await deleted).status()).toBe(200);
      await expect(page.getByTestId("gitops-delete-Application-argocd-platform-apps")).toHaveCount(0);

      // No finalizer: only the object goes, and nothing needs typing.
      await page.getByTestId("gitops-delete-Application-argocd-docs-site").click();
      await expect(dlg.getByTestId("gitops-delete-cascade")).toHaveCount(0);
      await expect(dlg.getByTestId("gitops-delete-consequence")).toContainText(/keep running|only the/i);
      await dlg.getByTestId("gitops-delete-confirm").click();
      await expect(page.getByTestId("gitops-delete-Application-argocd-docs-site")).toHaveCount(0);

      // A Flux source shared by two Kustomizations is never offered for removal.
      await page.getByTestId("gitops-delete-Kustomization-flux-system-infra").click();
      await expect(dlg.getByTestId("gitops-delete-shared")).toContainText("still used");
      await expect(dlg.getByTestId("gitops-delete-remove-source")).toHaveCount(0);
    });
  });

  test("the server demands the typed confirmation and only admins can delete", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      const del = (q, h = ADMIN) => page.request.delete(`/api/v1/gitops/Application/argocd/platform-apps${q}`, { headers: h });
      expect((await del("")).status()).toBe(400);
      expect((await del("?confirm=wrong")).status()).toBe(400);
      const mk = await page.request.post("/api/v1/users", { headers: ADMIN, data: { username: "gd-oper", password: "pw-pw-pw-pw", role: "operator" } });
      expect(mk.status()).toBe(201);
      const oper = basic("gd-oper", "pw-pw-pw-pw");
      expect((await del("?confirm=platform-apps", oper)).status()).toBe(403);
      // An operator may still read the preview and trigger a sync.
      expect((await page.request.get("/api/v1/gitops/Application/argocd/platform-apps/delete-preview", { headers: oper })).status()).toBe(200);
      expect((await page.request.post("/api/v1/gitops/Application/argocd/platform-apps/sync", { headers: oper })).status()).toBe(200);
      expect((await del("?confirm=platform-apps")).status()).toBe(200);
      expect((await del("?confirm=platform-apps")).status()).toBe(404);
    });
  });
});

test.describe("require-MFA policy and QR enrolment", () => {
  test.setTimeout(120_000);

  test("a covered account is held at the enrolment screen until it has a second factor", async ({ browser }) => {
    await withApp(browser, { AXIOM_REQUIRE_MFA: "all" }, async ({ page }) => {
      const mk = await page.request.post("/api/v1/users", { headers: ADMIN, data: { username: "must-mfa", password: "pw-pw-pw-pw", role: "operator" } });
      expect(mk.status()).toBe(201);

      // The API holds everything but enrolment, with a body the UI can act on.
      const blocked = await page.request.get("/api/v1/pods", { headers: basic("must-mfa", "pw-pw-pw-pw") });
      expect(blocked.status()).toBe(403);
      expect((await blocked.json()).mfaEnrollmentRequired).toBe(true);
      const session = await (await page.request.get("/api/v1/session", { headers: basic("must-mfa", "pw-pw-pw-pw") })).json();
      expect(session.mfa).toEqual({ required: true, enrolled: false });

      // The break-glass env admin is exempt (no AXIOM_TOTP_SECRET configured).
      expect((await page.request.get("/api/v1/pods", { headers: ADMIN })).status()).toBe(200);

      // In the UI: only the enrolment gate, with a QR code and the typed setup key.
      await signInAs(page, "must-mfa", "pw-pw-pw-pw");
      await expect(page.getByTestId("mfa-enrollment-gate")).toBeVisible();
      await expect(page.getByRole("heading", { name: "Overview" })).toHaveCount(0); // the app itself is not rendered
      const dialog = page.getByTestId("account-security-dialog");
      await dialog.getByTestId("account-security-setup-password").fill("pw-pw-pw-pw");
      await dialog.getByTestId("account-security-setup-start").click();
      await expect(dialog.getByTestId("account-security-qr")).toBeVisible();
      const secret = ((await dialog.getByTestId("account-security-secret").textContent()) || "").replace(/\s/g, "");
      await dialog.getByTestId("account-security-enable-code").fill(totp(secret));
      await dialog.getByTestId("account-security-enable-submit").click();
      await expect(dialog.getByTestId("account-security-recovery-codes").locator("li")).toHaveCount(10);
      await dialog.getByTestId("account-security-recovery-done").click();
      await expect(page).toHaveURL(/\/login/);

      // Signing in with a code lifts the hold.
      await page.getByPlaceholder("Username").fill("must-mfa");
      await page.getByRole("button", { name: "Continue", exact: true }).click();
      await page.getByPlaceholder("Password").fill("pw-pw-pw-pw");
      await page.getByRole("button", { name: "Sign in to Axiom" }).click();
      await page.getByTestId("login-mfa-code").fill(totp(secret, 1));
      await page.getByTestId("login-mfa-submit").click();
      await expect(page.getByTestId("sidebar-nav")).toBeVisible();
      await expect(page.getByTestId("mfa-enrollment-gate")).toHaveCount(0);
      const after = await (await page.request.get("/api/v1/session")).json();
      expect(after.mfa).toEqual({ required: true, enrolled: true });
    });
  });

  test("without the policy nothing is held back", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      const mk = await page.request.post("/api/v1/users", { headers: ADMIN, data: { username: "free-mfa", password: "pw-pw-pw-pw", role: "operator" } });
      expect(mk.status()).toBe(201);
      const session = await (await page.request.get("/api/v1/session", { headers: basic("free-mfa", "pw-pw-pw-pw") })).json();
      expect(session.mfa.required).toBe(false);
      expect((await page.request.get("/api/v1/pods", { headers: basic("free-mfa", "pw-pw-pw-pw") })).status()).toBe(200);
    });
  });
});
