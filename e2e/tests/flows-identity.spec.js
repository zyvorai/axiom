// @ts-check
// Account security flows: two-factor (TOTP) sign-in for a local account —
// enrolment from the user menu, the code step on the sign-in page, recovery
// codes, the admin reset and sign-out-everywhere on the Users page — driven
// through the real UI against the shared demo server.
//
// Failed sign-ins count toward the per-IP login throttle, so each test keeps
// its deliberate failures to one or two and always follows them with a success.
const { test, expect } = require("@playwright/test");
const crypto = require("crypto");
const net = require("net");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { spawn } = require("child_process");
const { signIn, apiGet, apiSend, confirmDialog, waitMutation, USERNAME, PASSWORD } = require("./helpers");

const uid = () => Math.random().toString(36).slice(2, 8);

const B32 = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
function b32decode(s) {
  let bits = "";
  for (const c of s.replace(/[\s=-]/g, "").toUpperCase()) bits += B32.indexOf(c).toString(2).padStart(5, "0");
  const out = [];
  for (let i = 0; i + 8 <= bits.length; i += 8) out.push(parseInt(bits.slice(i, i + 8), 2));
  return Buffer.from(out);
}
/** RFC 6238 code (SHA-1, 6 digits, 30 s) for `stepOffset` steps from now. */
function totp(secret, stepOffset = 0) {
  const counter = Math.floor(Date.now() / 30000) + stepOffset;
  const msg = Buffer.alloc(8);
  msg.writeBigUInt64BE(BigInt(counter));
  const h = crypto.createHmac("sha1", b32decode(secret)).update(msg).digest();
  const o = h[h.length - 1] & 0x0f;
  const n = ((h[o] & 0x7f) << 24) | (h[o + 1] << 16) | (h[o + 2] << 8) | h[o + 3];
  return String(n % 1_000_000).padStart(6, "0");
}

async function passwordStep(page, user, pass) {
  await page.goto("/login");
  await page.getByPlaceholder("Username").fill(user);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.getByPlaceholder("Password").fill(pass);
  await page.getByRole("button", { name: "Sign in to Axiom" }).click();
}

// Each test runs on its OWN server (like flows-fleet.spec.js): failed sign-ins here would
// otherwise leave the shared server's per-IP login backoff locked for minutes.
const BIN_SRC = path.resolve(__dirname, "../../bin/zyvor-axiom");
/** @type {string} */
let BIN = "";
const running = new Set();
const basic = (u, p) => ({ Authorization: `Basic ${Buffer.from(`${u}:${p}`).toString("base64")}` });

test.beforeAll(() => {
  if (!fs.existsSync(BIN_SRC)) return;
  BIN = path.join(os.tmpdir(), `axiom-e2e-identity-${process.pid}`);
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


// The deliberate failed requests above (and the login page's own unauthenticated probe)
// trigger a short per-IP backoff; wait for it to clear before the next sign-in.
async function waitForThrottleToClear(page) {
  const deadline = Date.now() + 60_000;
  while (Date.now() < deadline) {
    const r = await page.request.get("/api/v1/session", { headers: { Authorization: `Basic ${Buffer.from(`${USERNAME}:${PASSWORD}`).toString("base64")}` } });
    if (r.status() !== 429) return;
    await new Promise((res) => setTimeout(res, Math.max(500, Number(r.headers()["retry-after"] || 1) * 1000)));
  }
}

// Sign in as the admin, retrying after a short pause if a brief login backoff rejects an
// attempt (the UI clears the password and shows an error, exactly what a person would retry).
async function signInAdmin(page) {
  for (let attempt = 0; attempt < 4; attempt++) {
    await waitForThrottleToClear(page);
    try {
      await signIn(page);
      return;
    } catch (e) {
      if (attempt === 3) throw e;
      await new Promise((res) => setTimeout(res, 6000));
    }
  }
}

async function createUser(page, name, pass) {
  const r = await apiSend(page, "POST", "/users", { username: name, password: pass, role: "operator" });
  expect(r.status).toBe(201);
}

test.describe("two-factor sign-in", () => {
  test.setTimeout(90_000);

  test("enrol from the user menu, sign in with a code, then with a recovery code", async ({ browser }) => {
    await withApp(browser, {}, async ({ page, srv }) => {
    const name = `mfa-${uid()}`;
    const pass = `pw-${uid()}-${uid()}`;
    await createUser(page, name, pass);
    try {
      // --- Enrol ---
      await passwordStep(page, name, pass);
      await expect(page.getByTestId("sidebar-nav")).toBeVisible();
      await page.getByTestId("user-menu-trigger").click();
      await page.getByTestId("user-menu-account-security").click();
      const dialog = page.getByTestId("account-security-dialog");
      await expect(dialog.getByTestId("account-security-status")).toContainText("off");

      // The wrong password is refused and no secret is issued.
      await dialog.getByTestId("account-security-setup-password").fill("not-the-password");
      await dialog.getByTestId("account-security-setup-start").click();
      await expect(dialog.getByTestId("account-security-secret")).toHaveCount(0);
      await dialog.getByTestId("account-security-setup-password").fill(pass);
      await dialog.getByTestId("account-security-setup-start").click();
      const secret = ((await dialog.getByTestId("account-security-secret").textContent()) || "").replace(/\s/g, "");
      expect(secret).toMatch(/^[A-Z2-7]{32}$/);

      // A wrong code is refused; the right one turns it on and shows the recovery codes once.
      await dialog.getByTestId("account-security-enable-code").fill("000000");
      await dialog.getByTestId("account-security-enable-submit").click();
      await expect(dialog.getByTestId("account-security-recovery-codes")).toHaveCount(0);
      await dialog.getByTestId("account-security-enable-code").fill(totp(secret));
      await dialog.getByTestId("account-security-enable-submit").click();
      const items = dialog.getByTestId("account-security-recovery-codes").locator("li");
      await expect(items).toHaveCount(10);
      const recovery = (await items.allTextContents()).map((s) => s.trim());
      expect(recovery[0]).toMatch(/^[a-z2-9]{5}-[a-z2-9]{5}$/);
      await dialog.getByTestId("account-security-recovery-done").click();
      await expect(page).toHaveURL(/\/login/);

      // --- Out of band: Basic alone is now refused, and says why ---
      const basic = await page.request.get("/api/v1/session", { headers: { Authorization: `Basic ${Buffer.from(`${name}:${pass}`).toString("base64")}` } });
      expect(basic.status()).toBe(401);
      expect(basic.headers()["x-axiom-mfa"]).toBe("required");

      // --- Sign in: password, then the code ---
      await passwordStep(page, name, pass);
      await expect(page.getByTestId("login-mfa-code")).toBeVisible();
      await expect(page.getByTestId("sidebar-nav")).toHaveCount(0);
      // The step before (enrolment) used the current time step, so use the next one.
      const signInCode = totp(secret, 1); // keep the exact code: recomputing later can cross a 30 s boundary and yield a NEW valid code
      await page.getByTestId("login-mfa-code").fill(signInCode);
      await page.getByTestId("login-mfa-submit").click();
      await expect(page.getByTestId("sidebar-nav")).toBeVisible();
      const session = await (await page.request.get("/api/v1/session")).json();
      expect(session.user).toBe(name);
      expect(session.auth).toBe("local");

      // (Replay of a TOTP code and reuse of a recovery code are covered by the Go tests; repeating
      // those failures here only feeds the per-IP login backoff and makes the browser flow flaky.)

      // --- Recovery code: works once ---
      await page.getByTestId("user-menu-trigger").click();
      await page.getByTestId("user-menu-signout").click();
      await passwordStep(page, name, pass);
      await page.getByTestId("login-mfa-use-recovery").click();
      await page.getByTestId("login-mfa-code").fill(recovery[0]);
      await page.getByTestId("login-mfa-submit").click();
      await expect(page.getByTestId("sidebar-nav")).toBeVisible();

      // --- Admin view: the Users page shows it, and an admin can reset it ---
      await page.getByTestId("user-menu-trigger").click();
      await page.getByTestId("user-menu-signout").click();
      await signInAdmin(page);
      await page.goto("/users");
      await expect(page.getByTestId(`user-mfa-${name}`)).toContainText("on");
      await page.getByTestId(`user-mfa-reset-${name}`).click();
      const reset = waitMutation(page, "DELETE", new RegExp(`/users/${name}/mfa$`));
      await confirmDialog(page, "Reset");
      expect((await reset).status()).toBe(200);
      await expect(page.getByTestId(`user-mfa-${name}`)).toContainText("off");
      const ok = await page.request.get("/api/v1/session", { headers: { Authorization: `Basic ${Buffer.from(`${name}:${pass}`).toString("base64")}` } });
      expect(ok.status()).toBe(200);
    } finally {
      await apiSend(page, "DELETE", `/users/${name}`);
    }
    });
  });

  test("sign out everywhere ends the account's other sessions", async ({ browser }) => {
    await withApp(browser, {}, async ({ page, srv }) => {
    const name = `sess-${uid()}`;
    const pass = `pw-${uid()}-${uid()}`;
    await createUser(page, name, pass);
    try {
      // A second device holding a session token.
      const device = await browser.newContext({ baseURL: srv.url, ignoreHTTPSErrors: true });
      const login = await device.request.post("/api/v1/auth/login", { data: { username: name, password: pass } });
      expect(login.status()).toBe(200);
      const { token } = await login.json();
      const bearer = { Authorization: `Bearer ${token}` };
      expect((await device.request.get("/api/v1/session", { headers: bearer })).status()).toBe(200);

      // The admin signs that user out from the Users page.
      await signInAdmin(page);
      await page.goto("/users");
      await page.getByTestId(`user-revoke-sessions-${name}`).click();
      const revoke = waitMutation(page, "POST", new RegExp(`/users/${name}/revoke-sessions$`));
      await confirmDialog(page, "Sign out");
      expect((await revoke).status()).toBe(200);
      expect((await device.request.get("/api/v1/session", { headers: bearer })).status()).toBe(401);

      // ...and the user can simply sign in again.
      const again = await device.request.post("/api/v1/auth/login", { data: { username: name, password: pass } });
      expect(again.status()).toBe(200);
      await device.close();
    } finally {
      await apiSend(page, "DELETE", `/users/${name}`);
    }
    });
  });

  test("an account managed by the environment points at AXIOM_TOTP_SECRET", async ({ browser }) => {
    await withApp(browser, {}, async ({ page, srv }) => {
    await signInAdmin(page);
    await page.getByTestId("user-menu-trigger").click();
    await page.getByTestId("user-menu-account-security").click();
    const dialog = page.getByTestId("account-security-dialog");
    await expect(dialog.getByTestId("account-security-status")).toContainText("AXIOM_TOTP_SECRET");
    await expect(dialog.getByTestId("account-security-setup-start")).toHaveCount(0);
    const st = await apiGet(page, "/auth/mfa");
    expect(st.status).toBe(200);
    });
  });
});
