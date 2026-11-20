// @ts-check
// Shared helpers for the functional (state-changing) specs.
//
// Every flow drives the real UI and asserts three independent things:
//   1. the mutating request the UI sent came back 2xx (recorded by `track`),
//   2. the UI reflects the change (row appears / disappears, toast),
//   3. an out-of-band API read (`apiGet`) agrees with the UI.
// Any mutating call that returns >= 400 fails the test unless the test
// declared it expected via `track.allow`.
const { expect } = require("@playwright/test");

const USERNAME = process.env.AXIOM_E2E_USERNAME || "demo";
const PASSWORD = process.env.AXIOM_E2E_PASSWORD || "demo";

async function signIn(page) {
  await page.goto("/");
  await page.getByPlaceholder("Username").fill(USERNAME);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.getByPlaceholder("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Sign in to Axiom" }).click();
  await expect(page.getByTestId("sidebar-nav")).toBeVisible();
}

// Records every non-GET /api/ request and its response status, plus console
// errors and uncaught exceptions. `track.failures()` is what a test asserts on.
function track(page) {
  /** @type {{method:string,url:string,status:number,body:any}[]} */
  const calls = [];
  const errors = [];
  const allowed = [];
  page.on("response", async (res) => {
    const req = res.request();
    if (req.method() === "GET" || !res.url().includes("/api/")) return;
    let body = null;
    try { body = req.postDataJSON(); } catch { body = req.postData(); }
    calls.push({ method: req.method(), url: new URL(res.url()).pathname, status: res.status(), body });
  });
  page.on("pageerror", (e) => errors.push(`pageerror: ${e.message}`));
  page.on("console", (m) => {
    if (m.type() !== "error") return;
    const t = m.text();
    // A failed request is already captured (with status) via `calls`; the
    // browser also logs it as a console error, which is just noise here.
    if (/Failed to load resource/.test(t)) return;
    errors.push(`console: ${t}`);
  });
  return {
    calls,
    errors,
    /** Declare that a call matching `re` may legitimately fail (e.g. negative tests). */
    allow(re) { allowed.push(re); },
    /** Calls matching method + path regex. */
    find(method, re) { return calls.filter((c) => c.method === method && re.test(c.url)); },
    failures() {
      return calls.filter((c) => c.status >= 400 && !allowed.some((re) => re.test(`${c.method} ${c.url}`)));
    },
  };
}

// Out-of-band API access, deliberately independent of the UI's own session:
// it authenticates with Basic credentials directly, so it can confirm what the
// backend actually holds regardless of what the page rendered.
const AUTH = { Authorization: `Basic ${Buffer.from(`${USERNAME}:${PASSWORD}`).toString("base64")}` };

async function apiGet(page, path) {
  const res = await page.request.get(`/api/v1${path}`, { headers: AUTH });
  return { status: res.status(), data: res.ok() ? await res.json().catch(() => null) : null };
}

async function apiSend(page, method, path, data) {
  const res = await page.request.fetch(`/api/v1${path}`, { method, headers: AUTH, data });
  return { status: res.status(), data: await res.json().catch(() => null) };
}

// Waits for the next successful mutating response matching method + path.
function waitMutation(page, method, re) {
  return page.waitForResponse((r) => r.request().method() === method && re.test(new URL(r.url()).pathname));
}

// Confirms the ConfirmDialog (the app's shared "are you sure?" dialog).
async function confirmDialog(page, label = "Delete") {
  const dlg = page.getByRole("alertdialog");
  await expect(dlg).toBeVisible();
  await dlg.getByRole("button", { name: label, exact: true }).click();
  await expect(dlg).toBeHidden();
}

async function toastText(page, re) {
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: re }).first()).toBeVisible();
}

module.exports = { signIn, track, apiGet, apiSend, waitMutation, confirmDialog, toastText, USERNAME, PASSWORD };
