// @ts-check
// Functional specs for the configuration / networking / storage write flows:
// ConfigMaps, Secrets, HPA (autoscaling), NetworkPolicy + the shared
// ApplyManifestDialog, Atlas volumes, plus the read-only Quotas / Ingresses /
// Services / Storage pages (which have no create/delete UI today).
//
// Same pattern as flows-core.spec.js: drive the real UI, assert the mutating
// request came back 2xx (recorded by `track`), assert the UI reflects it,
// assert an out-of-band API read agrees, and assert nothing else failed.
//
// Designed for a *demo-mode* server (in-memory). Where demo mode genuinely
// cannot perform something (POST /apply is "not available in demo mode"), the
// test either asserts that honest message or drives the UI contract against a
// mocked backend and says so in the title. See e2e/README.md.
const { test, expect } = require("@playwright/test");
const { signIn, track, apiGet, apiSend, waitMutation, confirmDialog, toastText } = require("./helpers");

const uid = () => Math.random().toString(36).slice(2, 8);

/** @type {(() => Promise<any>)[]} */
let cleanups = [];

test.beforeEach(async ({ page }) => {
  cleanups = [];
  await signIn(page);
});

test.afterEach(async ({ page }) => {
  // Best-effort: a test that already deleted its resource just gets a 404 here.
  for (const fn of cleanups.reverse()) {
    try { await fn(); } catch { /* ignore */ }
  }
  void page;
});

async function go(page, path, heading) {
  await page.goto(path);
  await expect(page.getByRole("heading", { name: heading }).first()).toBeVisible();
}

// Calls that failed, including the ones a negative test declared expected via
// t.allow (t.failures() excludes those). Lets a test pin the exact status.
const failed = (t) => t.calls.filter((c) => c.status >= 400);

const row = (page, text) => page.getByRole("row").filter({ hasText: text });

// Adds/removes key-value rows inside the Create ConfigMap / Secret dialogs.
async function fillPairs(dlg, pairs) {
  for (let i = 0; i < pairs.length; i++) {
    if (i > 0) await dlg.getByRole("button", { name: "Add key" }).click();
    await dlg.getByLabel(`Key ${i + 1}`).fill(pairs[i][0]);
    await dlg.getByLabel(`Value ${i + 1}`).fill(pairs[i][1]);
  }
}

// True when the backend is the in-memory demo provider (POST /apply is a stub).
let demoCache;
async function isDemo(page) {
  if (demoCache === undefined) {
    const r = await apiSend(page, "POST", "/apply", { manifest: "kind: Probe" });
    demoCache = /not available in demo mode/i.test(JSON.stringify(r.data || ""));
  }
  return demoCache;
}

// Applying a manifest is a two-step flow: "Review changes" (POST /apply/diff, a dry-run preview)
// must succeed for the current text before "Apply" enables. Returns once the review finished.
async function reviewChanges(dlg) {
  await dlg.getByRole("button", { name: /^Review/ }).click();
  await expect(dlg.getByRole("button", { name: "Apply", exact: true })).toBeEnabled();
}

const NETPOL_YAML = (name, ns = "default") => `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: ${name}
  namespace: ${ns}
spec:
  podSelector: {}
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector: {}
`;

// ---------------------------------------------------------------------------
// ConfigMaps
// ---------------------------------------------------------------------------
test.describe("ConfigMaps", () => {
  test("create, edit on the detail page (PUT), delete", async ({ page }) => {
    const t = track(page);
    const name = `e2e-cm-${uid()}`;
    cleanups.push(() => apiSend(page, "DELETE", `/configmaps/default/${name}`));

    // --- create
    await go(page, "/configmaps", "ConfigMaps");
    await page.getByRole("button", { name: "Create ConfigMap" }).click();
    const dlg = page.getByRole("dialog");
    await dlg.locator("#config-maps-name").fill(name);
    await fillPairs(dlg, [["LOG_LEVEL", "debug"], ["REGION", "eu-west-1"]]);
    const posted = waitMutation(page, "POST", /\/configmaps$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await posted).status()).toBe(201);

    await toastText(page, new RegExp(`ConfigMap ${name} created`));
    await expect(dlg).toBeHidden();
    const created = row(page, name);
    await expect(created).toBeVisible();
    await expect(created).toContainText("LOG_LEVEL");
    await expect(created).toContainText("REGION");
    expect(t.find("POST", /\/configmaps$/)[0].body).toMatchObject({
      namespace: "default", name, data: { LOG_LEVEL: "debug", REGION: "eu-west-1" },
    });
    const got = await apiGet(page, `/configmaps/default/${name}`);
    expect(got.data.data).toEqual({ LOG_LEVEL: "debug", REGION: "eu-west-1" });

    // --- edit on the detail page
    await created.getByRole("link", { name }).click();
    await expect(page).toHaveURL(new RegExp(`/configmaps/default/${name}$`));
    await expect(page.getByText("LOG_LEVEL").first()).toBeVisible();
    await page.getByRole("button", { name: "Edit data" }).click();
    const editor = page.getByRole("textbox");
    await expect(editor).toHaveValue(/"LOG_LEVEL": "debug"/); // seeded from the current data
    await editor.fill(JSON.stringify({ LOG_LEVEL: "info", NEW_KEY: "new-value" }, null, 2));
    const put = waitMutation(page, "PUT", new RegExp(`/configmaps/default/${name}$`));
    await page.getByRole("button", { name: "Save", exact: true }).click();
    expect((await put).status()).toBe(200);

    await toastText(page, /ConfigMap updated/);
    await expect(page.getByRole("button", { name: "Edit data" })).toBeVisible(); // left edit mode
    await expect(page.getByText("NEW_KEY").first()).toBeVisible();
    await expect(page.getByText("new-value").first()).toBeVisible();
    await expect(page.getByText("REGION")).toHaveCount(0); // removed key is really gone in the UI
    const edited = await apiGet(page, `/configmaps/default/${name}`);
    expect(edited.data.data).toEqual({ LOG_LEVEL: "info", NEW_KEY: "new-value" });
    // The list page shows the new key set too.
    await page.getByRole("button", { name: "All ConfigMaps" }).click();
    await expect(row(page, name)).toContainText("NEW_KEY");
    await expect(row(page, name)).not.toContainText("REGION");

    // --- delete
    const del = waitMutation(page, "DELETE", new RegExp(`/configmaps/default/${name}$`));
    await page.getByRole("button", { name: `Delete ConfigMap ${name}` }).click();
    await confirmDialog(page);
    expect((await del).status()).toBeLessThan(300);
    await toastText(page, new RegExp(`Deleted ${name}`));
    await expect(row(page, name)).toHaveCount(0);
    expect((await apiGet(page, `/configmaps/default/${name}`)).status).toBe(404);
    expect((await apiGet(page, "/configmaps")).data.map((c) => c.name)).not.toContain(name);

    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("negative: duplicate name shows the error, keeps the dialog usable and the original intact", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/configmaps$/);
    const name = `e2e-cm-${uid()}`;
    cleanups.push(() => apiSend(page, "DELETE", `/configmaps/default/${name}`));
    expect((await apiSend(page, "POST", "/configmaps", { namespace: "default", name, data: { ORIGINAL: "yes" } })).status).toBe(201);

    await go(page, "/configmaps", "ConfigMaps");
    await page.getByRole("button", { name: "Create ConfigMap" }).click();
    const dlg = page.getByRole("dialog");
    await dlg.locator("#config-maps-name").fill(name);
    await fillPairs(dlg, [["OVERWRITE", "attempt"]]);
    const dup = waitMutation(page, "POST", /\/configmaps$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await dup).status()).toBe(409);

    await toastText(page, /Create failed/);
    await toastText(page, /already exists/);
    await expect(dlg).toBeVisible(); // the form was not discarded
    await expect(dlg.locator("#config-maps-name")).toHaveValue(name);
    await expect(dlg.getByRole("button", { name: "Create", exact: true })).toBeEnabled(); // not stuck on "Creating…"
    expect((await apiGet(page, `/configmaps/default/${name}`)).data.data).toEqual({ ORIGINAL: "yes" });

    // UI stays usable: rename and it goes through.
    const name2 = `${name}-b`;
    cleanups.push(() => apiSend(page, "DELETE", `/configmaps/default/${name2}`));
    await dlg.locator("#config-maps-name").fill(name2);
    const ok = waitMutation(page, "POST", /\/configmaps$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await ok).status()).toBe(201);
    await expect(row(page, name2)).toBeVisible();

    expect(failed(t).map((f) => f.status)).toEqual([409]);
    expect(t.errors).toEqual([]);
  });

  test("negative: empty name blocks submit; no keys is rejected with a visible error", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/configmaps$/);
    const name = `e2e-cm-${uid()}`;
    cleanups.push(() => apiSend(page, "DELETE", `/configmaps/default/${name}`));

    await go(page, "/configmaps", "ConfigMaps");
    await page.getByRole("button", { name: "Create ConfigMap" }).click();
    const dlg = page.getByRole("dialog");
    const create = dlg.getByRole("button", { name: "Create", exact: true });
    await expect(create).toBeDisabled(); // required name is empty
    await dlg.locator("#config-maps-name").fill(name);
    await expect(create).toBeEnabled();

    // A ConfigMap with no data entries: the API refuses and the UI says why.
    const rejected = waitMutation(page, "POST", /\/configmaps$/);
    await create.click();
    expect((await rejected).status()).toBe(400);
    await toastText(page, /at least one data entry/);
    await expect(dlg).toBeVisible();
    expect((await apiGet(page, `/configmaps/default/${name}`)).status).toBe(404);

    // A blank key row is ignored client-side (same 400), then a real key succeeds.
    await fillPairs(dlg, [["K", "v"]]);
    const ok = waitMutation(page, "POST", /\/configmaps$/);
    await create.click();
    expect((await ok).status()).toBe(201);
    await expect(row(page, name)).toBeVisible();

    expect(failed(t).map((f) => f.status)).toEqual([400]);
    expect(t.errors).toEqual([]);
  });

  test("negative: invalid JSON or non-string values in the editor show an error and keep the edit intact", async ({ page }) => {
    const t = track(page);
    t.allow(/PUT \/api\/v1\/configmaps\//);
    const name = `e2e-cm-${uid()}`;
    cleanups.push(() => apiSend(page, "DELETE", `/configmaps/default/${name}`));
    await apiSend(page, "POST", "/configmaps", { namespace: "default", name, data: { A: "1" } });

    await go(page, `/configmaps/default/${name}`, name);
    await page.getByRole("button", { name: "Edit data" }).click();
    const editor = page.getByRole("textbox");
    const save = page.getByRole("button", { name: "Save", exact: true });

    // 1. Not JSON: the client-side parse fails, no request leaves the browser.
    await editor.fill("{ not json");
    await save.click();
    await toastText(page, /Update failed/);
    expect(t.find("PUT", /configmaps/)).toHaveLength(0);
    await expect(editor).toHaveValue("{ not json"); // the user's text is preserved
    await expect(save).toBeEnabled();

    // 2. JSON but not a string map: the server rejects it (400) and the UI shows that.
    await editor.fill('{"A": 1}');
    const bad = waitMutation(page, "PUT", new RegExp(`/configmaps/default/${name}$`));
    await save.click();
    expect((await bad).status()).toBeGreaterThanOrEqual(400);
    await expect(page.locator("[data-sonner-toast]").filter({ hasText: /Update failed/ })).not.toHaveCount(0);
    await expect(editor).toBeVisible(); // still editing, not silently "saved"
    expect((await apiGet(page, `/configmaps/default/${name}`)).data.data).toEqual({ A: "1" }); // unchanged

    // 3. Correct it and it saves.
    await editor.fill('{"A": "2"}');
    const ok = waitMutation(page, "PUT", new RegExp(`/configmaps/default/${name}$`));
    await save.click();
    expect((await ok).status()).toBe(200);
    expect((await apiGet(page, `/configmaps/default/${name}`)).data.data).toEqual({ A: "2" });

    expect(failed(t).map((f) => f.method)).toEqual(["PUT"]);
    expect(t.errors).toEqual([]);
  });

  test("negative: cancelling the delete confirmation sends nothing and keeps the row", async ({ page }) => {
    const t = track(page);
    const name = `e2e-cm-${uid()}`;
    cleanups.push(() => apiSend(page, "DELETE", `/configmaps/default/${name}`));
    await apiSend(page, "POST", "/configmaps", { namespace: "default", name, data: { A: "1" } });

    await go(page, "/configmaps", "ConfigMaps");
    await page.getByRole("button", { name: `Delete ConfigMap ${name}` }).click();
    const dlg = page.getByRole("alertdialog");
    await expect(dlg).toContainText(`Delete ConfigMap ${name}?`);
    await dlg.getByRole("button", { name: "Cancel" }).click();
    await expect(dlg).toBeHidden();
    await expect(row(page, name)).toBeVisible();
    expect(t.calls).toHaveLength(0);
    expect((await apiGet(page, `/configmaps/default/${name}`)).status).toBe(200);
    expect(t.errors).toEqual([]);
  });

  test("negative: deleting a ConfigMap that vanished behind the UI's back reports the failure", async ({ page }) => {
    const t = track(page);
    t.allow(/DELETE \/api\/v1\/configmaps\//);
    const name = `e2e-cm-${uid()}`;
    await apiSend(page, "POST", "/configmaps", { namespace: "default", name, data: { A: "1" } });

    await go(page, "/configmaps", "ConfigMaps");
    await expect(row(page, name)).toBeVisible();
    expect((await apiSend(page, "DELETE", `/configmaps/default/${name}`)).status).toBeLessThan(300); // deleted elsewhere
    const del = waitMutation(page, "DELETE", new RegExp(`/configmaps/default/${name}$`));
    await page.getByRole("button", { name: `Delete ConfigMap ${name}` }).click();
    await confirmDialog(page);
    expect((await del).status()).toBe(404);
    await toastText(page, /Delete failed/);
    await expect(page.getByRole("heading", { name: "ConfigMaps" }).first()).toBeVisible(); // page still alive
    expect(t.errors).toEqual([]);
  });

  test("detail page for a missing ConfigMap shows a not-found state, not a crash", async ({ page }) => {
    const t = track(page);
    await page.goto(`/configmaps/default/e2e-missing-${uid()}`);
    await expect(page.getByText(/was not found/)).toBeVisible();
    await page.getByRole("button", { name: "ConfigMaps" }).click();
    await expect(page).toHaveURL(/\/configmaps$/);
    expect(t.errors).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// Secrets
// ---------------------------------------------------------------------------
test.describe("Secrets", () => {
  test("create (typed), reveal + save on the detail page (PUT), delete", async ({ page }) => {
    const t = track(page);
    const name = `e2e-sec-${uid()}`;
    cleanups.push(() => apiSend(page, "DELETE", `/secrets/default/${name}`));

    // --- create, with a non-default type picked from the Select
    await go(page, "/secrets", "Secrets");
    await page.getByRole("button", { name: "Create secret" }).click();
    const dlg = page.getByRole("dialog");
    await dlg.locator("#secrets-name").fill(name);
    await dlg.locator("#secrets-type").click();
    await page.getByRole("option", { name: "kubernetes.io/basic-auth" }).click();
    await fillPairs(dlg, [["username", "admin"], ["password", "s3cr3t-value"]]);
    const posted = waitMutation(page, "POST", /\/secrets$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await posted).status()).toBe(201);

    await toastText(page, new RegExp(`Secret ${name} created`));
    await expect(dlg).toBeHidden();
    const created = row(page, name);
    await expect(created).toBeVisible();
    await expect(created).toContainText("kubernetes.io/basic-auth");
    await expect(created).toContainText("username");
    await expect(created).toContainText("password");
    // Values never appear in the list.
    await expect(page.getByText("s3cr3t-value")).toHaveCount(0);
    expect(t.find("POST", /\/secrets$/)[0].body).toMatchObject({
      namespace: "default", name, type: "kubernetes.io/basic-auth", data: { username: "admin", password: "s3cr3t-value" },
    });
    const meta = await apiGet(page, `/secrets/default/${name}`);
    expect(meta.data.type).toBe("kubernetes.io/basic-auth");
    expect(meta.data.values ?? null).toBeNull(); // masked unless ?values=true

    // --- detail: masked, reveal, save
    await created.getByRole("link", { name }).click();
    await expect(page).toHaveURL(new RegExp(`/secrets/default/${name}$`));
    await expect(page.getByText("s3cr3t-value")).toHaveCount(0);
    await expect(page.getByText("••••••••").first()).toBeVisible();
    const reveal = page.waitForResponse((r) => r.request().method() === "GET" && r.url().includes(`/secrets/default/${name}?values=true`));
    await page.getByRole("button", { name: "Reveal values" }).click();
    expect((await reveal).status()).toBe(200);
    // Revealed values are editable fields; nothing to save until something changes.
    await expect(page.getByRole("textbox", { name: "Value of password" })).toHaveValue("s3cr3t-value");
    await expect(page.getByRole("textbox", { name: "Value of username" })).toHaveValue("admin");
    await expect(page.getByRole("button", { name: "Save values" })).toBeDisabled();

    await page.getByRole("textbox", { name: "Value of password" }).fill("rotated-value");
    const put = waitMutation(page, "PUT", new RegExp(`/secrets/default/${name}$`));
    await page.getByRole("button", { name: "Save values" }).click();
    expect((await put).status()).toBe(200);
    await toastText(page, /Secret updated/);
    expect(t.find("PUT", /\/secrets\//)[0].body).toMatchObject({ namespace: "default", name, data: { username: "admin", password: "rotated-value" } });
    await expect(page.getByRole("button", { name: "Save values" })).toBeDisabled(); // saved: no longer dirty
    // The PUT must not damage the secret: type survives and the edited value is what the backend holds.
    const after = await apiGet(page, `/secrets/default/${name}?values=true`);
    expect(after.data.type).toBe("kubernetes.io/basic-auth");
    expect(after.data.values).toEqual({ username: "admin", password: "rotated-value" });

    await page.getByRole("button", { name: "Hide values" }).click();
    await expect(page.getByRole("textbox", { name: /Value of/ })).toHaveCount(0);
    await expect(page.getByText("rotated-value")).toHaveCount(0);

    // --- delete from the list
    await page.getByRole("button", { name: "All secrets" }).click();
    const del = waitMutation(page, "DELETE", new RegExp(`/secrets/default/${name}$`));
    await page.getByRole("button", { name: `Delete secret ${name}` }).click();
    await confirmDialog(page);
    expect((await del).status()).toBeLessThan(300);
    await toastText(page, new RegExp(`Deleted ${name}`));
    await expect(row(page, name)).toHaveCount(0);
    expect((await apiGet(page, `/secrets/default/${name}`)).status).toBe(404);

    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("a value changed on the backend shows up after reveal (detail reads live data)", async ({ page }) => {
    const t = track(page);
    const name = `e2e-sec-${uid()}`;
    cleanups.push(() => apiSend(page, "DELETE", `/secrets/default/${name}`));
    await apiSend(page, "POST", "/secrets", { namespace: "default", name, type: "Opaque", data: { token: "old-token" } });

    await go(page, `/secrets/default/${name}`, name);
    expect((await apiSend(page, "PUT", `/secrets/default/${name}`, { namespace: "default", name, data: { token: "rotated-token", extra: "x" } })).status).toBe(200);
    await page.reload();
    await expect(page.getByText("2 keys")).toBeVisible();
    await page.getByRole("button", { name: "Reveal values" }).click();
    await expect(page.getByText("rotated-token")).toBeVisible();
    await expect(page.getByText("old-token")).toHaveCount(0);
    expect(t.errors).toEqual([]);
  });

  test("negative: duplicate name shows the error and keeps the dialog usable", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/secrets$/);
    const name = `e2e-sec-${uid()}`;
    cleanups.push(() => apiSend(page, "DELETE", `/secrets/default/${name}`));
    await apiSend(page, "POST", "/secrets", { namespace: "default", name, type: "Opaque", data: { ORIGINAL: "yes" } });

    await go(page, "/secrets", "Secrets");
    await page.getByRole("button", { name: "Create secret" }).click();
    const dlg = page.getByRole("dialog");
    await dlg.locator("#secrets-name").fill(name);
    await fillPairs(dlg, [["OVERWRITE", "attempt"]]);
    const dup = waitMutation(page, "POST", /\/secrets$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await dup).status()).toBe(409);
    await toastText(page, /Create failed/);
    await toastText(page, /already exists/);
    await expect(dlg).toBeVisible();
    await expect(dlg.getByRole("button", { name: "Create", exact: true })).toBeEnabled();
    expect((await apiGet(page, `/secrets/default/${name}?values=true`)).data.values).toEqual({ ORIGINAL: "yes" });

    expect(failed(t).map((f) => f.status)).toEqual([409]);
    expect(t.errors).toEqual([]);
  });

  test("negative: empty name blocks submit", async ({ page }) => {
    const t = track(page);
    await go(page, "/secrets", "Secrets");
    await page.getByRole("button", { name: "Create secret" }).click();
    const dlg = page.getByRole("dialog");
    await fillPairs(dlg, [["k", "v"]]);
    await expect(dlg.getByRole("button", { name: "Create", exact: true })).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(dlg).toBeHidden();
    expect(t.calls).toHaveLength(0);
    expect(t.errors).toEqual([]);
  });

  // Demo's CreateSecret accepts a secret with no keys, the Kubernetes provider
  // (ops_kubernetes.go CreateSecret) rejects it with "at least one data/keys
  // entry is required", and the form does no client-side check either.
  test("negative: a secret with no keys is rejected with a visible error", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/secrets$/);
    const name = `e2e-sec-${uid()}`;
    cleanups.push(() => apiSend(page, "DELETE", `/secrets/default/${name}`));
    await go(page, "/secrets", "Secrets");
    await page.getByRole("button", { name: "Create secret" }).click();
    const dlg = page.getByRole("dialog");
    await dlg.locator("#secrets-name").fill(name);
    const res = waitMutation(page, "POST", /\/secrets$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await res).status()).toBe(400);
    await toastText(page, /Create failed/);
    await expect(dlg).toBeVisible();
    expect((await apiGet(page, `/secrets/default/${name}`)).status).toBe(404);
  });

  // Revealed values are editable and "Save values" only enables once something changed.
  test("secret detail lets the user edit a value and save it via PUT", async ({ page }) => {
    const name = `e2e-sec-${uid()}`;
    cleanups.push(() => apiSend(page, "DELETE", `/secrets/default/${name}`));
    await apiSend(page, "POST", "/secrets", { namespace: "default", name, type: "Opaque", data: { token: "old" } });
    await go(page, `/secrets/default/${name}`, name);
    await page.getByRole("button", { name: "Reveal values" }).click();
    const input = page.getByRole("textbox").first();
    await expect(input).toBeVisible();
    await input.fill("new");
    const put = waitMutation(page, "PUT", new RegExp(`/secrets/default/${name}$`));
    await page.getByRole("button", { name: "Save values" }).click();
    expect((await put).status()).toBe(200);
    expect((await apiGet(page, `/secrets/default/${name}?values=true`)).data.values).toEqual({ token: "new" });
  });

  test("negative: cancelling the delete confirmation sends nothing", async ({ page }) => {
    const t = track(page);
    const name = `e2e-sec-${uid()}`;
    cleanups.push(() => apiSend(page, "DELETE", `/secrets/default/${name}`));
    await apiSend(page, "POST", "/secrets", { namespace: "default", name, type: "Opaque", data: { a: "b" } });
    await go(page, "/secrets", "Secrets");
    await page.getByRole("button", { name: `Delete secret ${name}` }).click();
    await page.getByRole("alertdialog").getByRole("button", { name: "Cancel" }).click();
    await expect(row(page, name)).toBeVisible();
    expect(t.calls).toHaveLength(0);
    expect((await apiGet(page, `/secrets/default/${name}`)).status).toBe(200);
  });

  test("detail page for a missing Secret shows a not-found state, not a crash", async ({ page }) => {
    const t = track(page);
    await page.goto(`/secrets/default/e2e-missing-${uid()}`);
    await expect(page.getByText(/was not found/)).toBeVisible();
    expect(t.errors).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// HPA (Autoscaling page)
// ---------------------------------------------------------------------------
test.describe("HPA", () => {
  // The dialog derives the HPA name as `${targetName}-hpa`, so a unique HPA
  // needs a unique target: seed a throw-away workload through the API.
  async function seedWorkload(page) {
    const wl = `e2e-wl-${uid()}`;
    const res = await apiSend(page, "POST", "/workloads", { namespace: "default", name: wl, image: "nginx:1", replicas: 1, port: 8080, cpu: "250m", memory: "256Mi" });
    expect(res.status).toBeLessThan(300);
    cleanups.push(() => apiSend(page, "DELETE", `/workloads/default/Deployment/${wl}`));
    cleanups.push(() => apiSend(page, "DELETE", `/hpas/default/${wl}-hpa`));
    return wl;
  }

  async function openCreate(page, wl) {
    await page.getByRole("button", { name: "Create autoscaler" }).click();
    const dlg = page.getByRole("dialog");
    await dlg.locator("#hpa-target-workload").click();
    await page.getByRole("option", { name: `default/Deployment/${wl}` }).click();
    return dlg;
  }

  test("create for a workload (min/max/cpu), appears in list and API, delete", async ({ page }) => {
    const t = track(page);
    const wl = await seedWorkload(page);
    const hpa = `${wl}-hpa`;

    await go(page, "/autoscaling", "Autoscaling");
    const dlg = await openCreate(page, wl);
    await dlg.locator("#hpa-min-replicas").fill("2");
    await dlg.locator("#hpa-max-replicas").fill("6");
    await dlg.locator("#hpa-target-cpu").fill("65");
    const posted = waitMutation(page, "POST", /\/hpas$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await posted).status()).toBe(201);

    await toastText(page, new RegExp(`HPA created for ${wl}`));
    await expect(dlg).toBeHidden();
    const r = row(page, hpa);
    await expect(r).toBeVisible();
    await expect(r).toContainText(`Deployment/${wl}`);
    await expect(r).toContainText("2 – 6");
    await expect(r).toContainText("65%");
    expect(t.find("POST", /\/hpas$/)[0].body).toMatchObject({
      namespace: "default", name: hpa, targetKind: "Deployment", targetName: wl, minReplicas: 2, maxReplicas: 6, targetCpuPercent: 65,
    });
    const found = (await apiGet(page, "/hpas")).data.find((h) => h.name === hpa);
    expect(found).toMatchObject({ namespace: "default", targetName: wl, minReplicas: 2, maxReplicas: 6, targetCpuPercent: 65 });

    const del = waitMutation(page, "DELETE", new RegExp(`/hpas/default/${hpa}$`));
    await page.getByRole("button", { name: `Delete autoscaler ${hpa}` }).click();
    await confirmDialog(page);
    expect((await del).status()).toBeLessThan(300);
    await toastText(page, new RegExp(`Deleted ${hpa}`));
    await expect(row(page, hpa)).toHaveCount(0);
    expect((await apiGet(page, "/hpas")).data.map((h) => h.name)).not.toContain(hpa);

    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("negative: no target selected blocks submit", async ({ page }) => {
    const t = track(page);
    await go(page, "/autoscaling", "Autoscaling");
    await page.getByRole("button", { name: "Create autoscaler" }).click();
    const dlg = page.getByRole("dialog");
    await expect(dlg.getByRole("button", { name: "Create", exact: true })).toBeDisabled();
    await page.keyboard.press("Escape");
    await expect(dlg).toBeHidden();
    expect(t.calls).toHaveLength(0);
  });

  test("negative: a second autoscaler for the same workload shows the conflict, dialog stays usable", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/hpas$/);
    const wl = await seedWorkload(page);
    const hpa = `${wl}-hpa`;
    await apiSend(page, "POST", "/hpas", { namespace: "default", name: hpa, targetKind: "Deployment", targetName: wl, minReplicas: 1, maxReplicas: 3, targetCpuPercent: 80 });

    await go(page, "/autoscaling", "Autoscaling");
    const dlg = await openCreate(page, wl);
    const dup = waitMutation(page, "POST", /\/hpas$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await dup).status()).toBe(409);
    await toastText(page, /Create HPA failed/);
    await toastText(page, /already exists/);
    await expect(dlg).toBeVisible();
    await expect(dlg.getByRole("button", { name: "Create", exact: true })).toBeEnabled();
    // (a modal dialog hides the page from the a11y tree, so use a plain locator)
    await expect(page.locator("tr").filter({ hasText: hpa })).toHaveCount(1); // no duplicate row
    expect((await apiGet(page, "/hpas")).data.filter((h) => h.name === hpa)).toHaveLength(1);

    expect(failed(t).map((f) => f.status)).toEqual([409]);
    expect(t.errors).toEqual([]);
  });

  // The form accepts max < min and the demo backend silently rewrites the
  // request (max := min) and reports success, so the user gets an HPA they did
  // not ask for. A real cluster rejects it; the UI has no client-side check.
  test("negative: max replicas below min is rejected, not silently altered", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/hpas$/);
    const wl = await seedWorkload(page);
    await go(page, "/autoscaling", "Autoscaling");
    const dlg = await openCreate(page, wl);
    await dlg.locator("#hpa-min-replicas").fill("5");
    await dlg.locator("#hpa-max-replicas").fill("2");
    const res = waitMutation(page, "POST", /\/hpas$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await res).status()).toBeGreaterThanOrEqual(400);
    await toastText(page, /Create HPA failed/);
    expect((await apiGet(page, "/hpas")).data.map((h) => h.name)).not.toContain(`${wl}-hpa`);
  });

  test("negative: cancelling the delete confirmation sends nothing", async ({ page }) => {
    const t = track(page);
    const wl = await seedWorkload(page);
    const hpa = `${wl}-hpa`;
    await apiSend(page, "POST", "/hpas", { namespace: "default", name: hpa, targetKind: "Deployment", targetName: wl, minReplicas: 1, maxReplicas: 3, targetCpuPercent: 80 });
    await go(page, "/autoscaling", "Autoscaling");
    await page.getByRole("button", { name: `Delete autoscaler ${hpa}` }).click();
    await page.getByRole("alertdialog").getByRole("button", { name: "Cancel" }).click();
    await expect(row(page, hpa)).toBeVisible();
    expect(t.calls).toHaveLength(0);
    expect((await apiGet(page, "/hpas")).data.map((h) => h.name)).toContain(hpa);
  });
});

// ---------------------------------------------------------------------------
// Networking: NetworkPolicy via YAML (POST /apply) and the ApplyManifestDialog
// ---------------------------------------------------------------------------
async function openNetpolDialog(page) {
  await go(page, "/networking", "Networking");
  await page.getByRole("button", { name: "Create via YAML" }).click();
  const dlg = page.getByRole("dialog");
  await expect(dlg.getByText("Create NetworkPolicy")).toBeVisible();
  return dlg;
}

// In-test stand-in for /networkpolicies + /apply so the UI contract can be
// exercised end to end where the demo backend cannot apply manifests.
async function mockNetpolBackend(page) {
  /** @type {any[]} */
  const store = [];
  await page.route((url) => new URL(url).pathname.startsWith("/api/v1/networkpolicies"), async (route) => {
    const req = route.request();
    if (req.method() === "GET") return route.fulfill({ json: store });
    if (req.method() === "DELETE") {
      const [, , , , ns, nm] = new URL(req.url()).pathname.split("/");
      const i = store.findIndex((p) => p.namespace === ns && p.name === decodeURIComponent(nm));
      if (i < 0) return route.fulfill({ status: 404, json: { error: "not found" } });
      store.splice(i, 1);
      return route.fulfill({ json: { ok: true } });
    }
    return route.continue();
  });
  await page.route((url) => new URL(url).pathname === "/api/v1/apply", async (route) => {
    const manifest = route.request().postDataJSON().manifest;
    const name = /^\s{2}name:\s*(\S+)/m.exec(manifest)?.[1];
    const namespace = /^\s{2}namespace:\s*(\S+)/m.exec(manifest)?.[1] || "default";
    if (!name) return route.fulfill({ json: { valid: false, errors: ["metadata.name is required"] } });
    store.push({ namespace, name, podSelector: "(all pods)", policyTypes: ["Ingress"], age: "now" });
    return route.fulfill({ json: { valid: true, kind: "NetworkPolicy", name, namespace } });
  });
  return store;
}

test.describe("NetworkPolicy + ApplyManifestDialog", () => {
  test("create via YAML, appears in the table, delete", async ({ page }) => {
    const t = track(page);
    const demo = await isDemo(page);
    const name = `e2e-np-${uid()}`;
    // Demo cannot apply manifests (see the honest-message test below), so there
    // the backend is mocked and the API read-back is the mock store.
    const store = demo ? await mockNetpolBackend(page) : null;
    if (!demo) cleanups.push(() => apiSend(page, "DELETE", `/networkpolicies/default/${name}`));
    const policies = async () => (store ? store : (await apiGet(page, "/networkpolicies")).data);

    const dlg = await openNetpolDialog(page);
    const editor = dlg.locator("textarea");
    await expect(editor).toHaveValue(/kind: NetworkPolicy/); // template is pre-filled
    await editor.fill(NETPOL_YAML(name));
    await reviewChanges(dlg);
    const applied = waitMutation(page, "POST", /\/apply$/);
    await dlg.getByRole("button", { name: "Apply", exact: true }).click();
    const res = await applied;
    expect(res.status()).toBe(200);
    expect(await res.json()).toMatchObject({ valid: true, kind: "NetworkPolicy", name });
    expect(t.find("POST", /\/apply$/)[0].body.manifest).toContain(`name: ${name}`);

    await toastText(page, new RegExp(`Applied NetworkPolicy ${name}`));
    await expect(dlg).toBeHidden();
    const r = row(page, name);
    await expect(r).toBeVisible(); // list reloaded
    await expect(r).toContainText("Ingress");
    expect((await policies()).map((p) => p.name)).toContain(name);

    const del = waitMutation(page, "DELETE", new RegExp(`/networkpolicies/default/${name}$`));
    await page.getByRole("button", { name: `Delete policy ${name}` }).click();
    await confirmDialog(page);
    expect((await del).status()).toBeLessThan(300);
    await toastText(page, new RegExp(`Deleted ${name}`));
    await expect(row(page, name)).toHaveCount(0);
    expect((await policies()).map((p) => p.name)).not.toContain(name);

    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("valid manifest on a backend that cannot apply says so honestly (demo) and creates nothing", async ({ page }) => {
    test.skip(!(await isDemo(page)), "only meaningful against the demo provider");
    const t = track(page);
    const name = `e2e-np-${uid()}`;
    const before = (await apiGet(page, "/networkpolicies")).data.map((p) => p.name);

    const dlg = await openNetpolDialog(page);
    await dlg.locator("textarea").fill(NETPOL_YAML(name));
    await reviewChanges(dlg);
    const applied = waitMutation(page, "POST", /\/apply$/);
    await dlg.getByRole("button", { name: "Apply", exact: true }).click();
    expect((await applied).status()).toBe(200);

    await expect(dlg.getByText(/not available in demo mode/)).toBeVisible();
    await expect(dlg.getByText(/^Applied /)).toHaveCount(0); // no false success
    await expect(page.locator("[data-sonner-toast]").filter({ hasText: /Applied/ })).toHaveCount(0);
    await expect(dlg).toBeVisible(); // stays open so the manifest is not lost
    await expect(dlg.getByRole("button", { name: "Apply", exact: true })).toBeEnabled();
    expect((await apiGet(page, "/networkpolicies")).data.map((p) => p.name)).toEqual(before);
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("negative: invalid YAML shows an error, does not crash, and the dialog stays usable", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/apply$/); // a real cluster may answer 400 here
    const dlg = await openNetpolDialog(page);
    await dlg.locator("textarea").fill("::: this is [not valid: yaml\n\t- {");
    // Apply is gated behind a successful review, so invalid text is rejected at the review step
    await expect(dlg.getByRole("button", { name: "Apply", exact: true })).toBeDisabled();
    await dlg.getByRole("button", { name: /^Review/ }).click();

    // A visible, non-empty, red error box and no success toast.
    const box = dlg.locator("div.rounded-lg.border");
    await expect(box).toBeVisible();
    await expect(box).toHaveClass(/tone-red/);
    expect(((await box.textContent()) || "").trim().length).toBeGreaterThan(0);
    await expect(page.locator("[data-sonner-toast]").filter({ hasText: /Applied/ })).toHaveCount(0);
    await expect(dlg.getByRole("button", { name: /^Review/ })).toBeEnabled(); // not stuck on "Reviewing…"
    await expect(dlg.getByRole("button", { name: "Apply", exact: true })).toBeDisabled(); // an invalid manifest can never be applied

    // Still usable: editing clears nothing, Escape closes, page beneath is alive.
    await dlg.locator("textarea").fill("still typing");
    await expect(dlg.locator("textarea")).toHaveValue("still typing");
    await page.keyboard.press("Escape");
    await expect(dlg).toBeHidden();
    await expect(page.getByRole("heading", { name: "NetworkPolicies" })).toBeVisible();
    expect(t.errors).toEqual([]);
  });

  test("negative: empty manifest cannot be applied; reopening resets to the template", async ({ page }) => {
    const t = track(page);
    const dlg = await openNetpolDialog(page);
    const apply = dlg.getByRole("button", { name: "Apply", exact: true });
    await dlg.locator("textarea").fill("");
    await expect(apply).toBeDisabled();
    await dlg.locator("textarea").fill("garbage");
    await expect(apply).toBeDisabled(); // not reviewed yet
    await dlg.getByRole("button", { name: /^Review/ }).click();
    await expect(dlg.locator("div.rounded-lg.border")).toBeVisible(); // a result box exists now
    await page.keyboard.press("Escape");
    await expect(dlg).toBeHidden();

    await page.getByRole("button", { name: "Create via YAML" }).click();
    await expect(dlg.locator("textarea")).toHaveValue(/kind: NetworkPolicy/); // template restored
    await expect(dlg.locator("div.rounded-lg.border")).toHaveCount(0); // stale error cleared
    expect(t.calls.length).toBeLessThanOrEqual(1);
    expect(t.errors).toEqual([]);
  });

  test("error shapes from POST /apply are all rendered (mocked backend): errors[], 400 error body, network failure", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/apply$/);
    const dlg = await openNetpolDialog(page);
    const apply = dlg.getByRole("button", { name: "Apply", exact: true });
    const box = dlg.locator("div.rounded-lg.border");
    let mode = "errors";
    await page.route((url) => new URL(url).pathname === "/api/v1/apply", async (route) => {
      if (mode === "errors") return route.fulfill({ json: { valid: false, errors: ["spec.podSelector: required", "kind: unknown"] } });
      if (mode === "400") return route.fulfill({ status: 400, json: { error: "yaml: line 3: mapping values are not allowed here" } });
      if (mode === "abort") return route.abort("failed");
      return route.fulfill({ json: { valid: true, kind: "NetworkPolicy", name: "recovered", namespace: "default" } });
    });

    await reviewChanges(dlg); // Apply enables once the (real) dry-run preview of the template succeeded
    await apply.click();
    await expect(box).toHaveText("spec.podSelector: required, kind: unknown");

    mode = "400";
    await apply.click();
    await expect(box).toHaveText(/mapping values are not allowed here/);

    mode = "abort";
    await apply.click();
    await expect(box).toBeVisible();
    await expect(box).not.toHaveText(/Applied/);
    expect(((await box.textContent()) || "").trim().length).toBeGreaterThan(0);

    // After all those failures the dialog still works: a good apply closes it.
    mode = "ok";
    await apply.click();
    await toastText(page, /Applied NetworkPolicy recovered/);
    await expect(dlg).toBeHidden();
    expect(t.errors.filter((e) => e.startsWith("pageerror"))).toEqual([]);
  });

  test("negative: cancelling the delete confirmation sends nothing (seeded policy untouched)", async ({ page }) => {
    const t = track(page);
    await go(page, "/networking", "Networking");
    const anyRow = page.getByRole("region", { name: "Network policies" }).getByRole("row").nth(1);
    await expect(anyRow).toBeVisible();
    const del = anyRow.getByRole("button", { name: /Delete policy/ });
    const label = (await del.getAttribute("aria-label")) || "";
    await del.click();
    await page.getByRole("alertdialog").getByRole("button", { name: "Cancel" }).click();
    await expect(page.getByRole("button", { name: label })).toBeVisible();
    expect(t.calls).toHaveLength(0);
  });

  test("negative: deleting a policy that already vanished reports the failure and refreshes", async ({ page }) => {
    const t = track(page);
    t.allow(/DELETE \/api\/v1\/networkpolicies\//);
    const demo = await isDemo(page);
    const name = `e2e-np-${uid()}`;
    // Show a policy the backend has never heard of; DELETE then really 404s.
    await page.route((url) => new URL(url).pathname === "/api/v1/networkpolicies", async (route) => {
      if (route.request().method() !== "GET") return route.continue();
      const real = await (await route.fetch()).json();
      return route.fulfill({ json: [...real, { namespace: "default", name, podSelector: "(all pods)", policyTypes: ["Ingress"], age: "1m" }] });
    });
    void demo;
    await go(page, "/networking", "Networking");
    await expect(row(page, name)).toBeVisible();
    const del = waitMutation(page, "DELETE", new RegExp(`/networkpolicies/default/${name}$`));
    await page.getByRole("button", { name: `Delete policy ${name}` }).click();
    await confirmDialog(page);
    expect((await del).status()).toBeGreaterThanOrEqual(400);
    await toastText(page, /Delete failed/);
    await expect(page.getByRole("heading", { name: "NetworkPolicies" })).toBeVisible();
    expect(t.errors).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// Atlas volumes (the only storage "create" flow in the UI)
// ---------------------------------------------------------------------------
test.describe("Atlas volumes", () => {
  async function openVolumeDialog(page) {
    await go(page, "/atlas", "Atlas");
    await page.getByRole("button", { name: "Create volume" }).click();
    const dlg = page.getByRole("dialog");
    await expect(dlg.getByText("Create volume").first()).toBeVisible();
    return dlg;
  }

  // GET /atlas serves a "demo Atlas fabric" as available and shows the Create
  // volume button, but POST /atlas/volumes answers 503 "atlas not configured"
  // (internal/api/atlas.go atlasCreateVolume has no demo branch).
  test("create volume against the demo Atlas fabric succeeds", async ({ page }) => {
    const t = track(page);
    const name = `e2e-vol-${uid()}`;
    const dlg = await openVolumeDialog(page);
    await dlg.locator("#atlas-name").fill(name);
    await dlg.locator("#atlas-size").fill("7");
    const posted = waitMutation(page, "POST", /\/atlas\/volumes$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await posted).status()).toBeLessThan(300);
    await toastText(page, new RegExp(`Volume ${name} requested`));
    await expect(dlg).toBeHidden();
    expect(t.failures()).toEqual([]);
  });

  test("create volume: request contract and UI outcome (mocked backend)", async ({ page }) => {
    const t = track(page);
    const name = `e2e-vol-${uid()}`;
    await page.route((url) => new URL(url).pathname === "/api/v1/atlas/volumes", (route) =>
      route.fulfill({ status: 202, json: { job_id: "job-1", state: "queued" } }));
    const dlg = await openVolumeDialog(page);
    await dlg.locator("#atlas-name").fill(name);
    await dlg.locator("#atlas-policy").click();
    await page.getByRole("option", { name: "production" }).click();
    await dlg.locator("#atlas-size").fill("7");
    await dlg.locator("#atlas-namespace").fill("data");
    const posted = waitMutation(page, "POST", /\/atlas\/volumes$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await posted).status()).toBe(202);
    expect(t.find("POST", /\/atlas\/volumes$/)[0].body).toEqual({ name, policy: "production", size_gi: 7, namespace: "data", create_pvc: true });
    await toastText(page, new RegExp(`Volume ${name} requested`));
    await expect(dlg).toBeHidden();
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("negative: a failing create shows the error and keeps the dialog usable; empty name blocks submit", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/atlas\/volumes$/);
    let fail = true;
    await page.route((url) => new URL(url).pathname === "/api/v1/atlas/volumes", (route) =>
      fail
        ? route.fulfill({ status: 502, json: { error: "atlas gateway unreachable" } })
        : route.fulfill({ status: 202, json: {} }));
    const dlg = await openVolumeDialog(page);
    const create = dlg.getByRole("button", { name: "Create", exact: true });
    await expect(create).toBeDisabled();
    await dlg.locator("#atlas-name").fill(`e2e-vol-${uid()}`);
    await create.click();
    await toastText(page, /Create volume failed/);
    await toastText(page, /atlas gateway unreachable/);
    await expect(dlg).toBeVisible();
    await expect(create).toBeEnabled();
    fail = false;
    await create.click();
    await toastText(page, /requested/);
    await expect(dlg).toBeHidden();
    expect(failed(t).map((f) => f.status)).toEqual([502]);
    expect(t.errors).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// Read-only pages: Quotas, Ingresses, Services, Storage. The UI offers no
// create/delete for these today (the API has createResourceQuota etc., but
// nothing calls it), so the flow under test is "what the API holds is what the
// page shows", including filters and the drill-down, with no mutating calls.
// ---------------------------------------------------------------------------
test.describe("read-only resource pages", () => {
  test("Quotas & Limits lists every ResourceQuota the API holds, with usage", async ({ page }) => {
    const t = track(page);
    await go(page, "/quotas", "Quotas & Limits");
    const quotas = (await apiGet(page, "/resourcequotas")).data;
    expect(quotas.length).toBeGreaterThan(0);
    const region = page.getByRole("region", { name: "Resource quotas" });
    for (const q of quotas) {
      const r = region.getByRole("row").filter({ hasText: q.name });
      await expect(r).toBeVisible();
      await expect(r).toContainText(q.namespace);
      for (const [k, v] of Object.entries(q.hard)) await expect(r).toContainText(`${k}=${v}`);
    }
    // No write affordances on this page.
    await expect(page.getByRole("button", { name: /^(Create|Delete|Add|New)/ })).toHaveCount(0);
    expect(t.calls).toHaveLength(0);
    expect(t.errors).toEqual([]);
  });

  test("Ingresses lists API data and the search filter narrows and recovers", async ({ page }) => {
    const t = track(page);
    await go(page, "/ingresses", "Ingresses");
    const ing = (await apiGet(page, "/ingresses")).data;
    expect(ing.length).toBeGreaterThan(1);
    for (const i of ing) {
      const r = row(page, i.name).filter({ hasText: i.namespace });
      await expect(r).toBeVisible();
      for (const h of i.hosts || []) await expect(r).toContainText(h);
    }
    const search = page.getByPlaceholder("Search by name or host");
    await search.fill(ing[0].hosts[0]); // filter by host
    await expect(row(page, ing[0].name).filter({ hasText: ing[0].namespace })).toBeVisible();
    await expect(row(page, ing[1].hosts[0])).toHaveCount(0);
    await search.fill(`nothing-${uid()}`);
    await expect(page.getByText("No ingresses match the current filters.")).toBeVisible(); // usable empty state
    await search.fill("");
    await expect(row(page, ing[1].hosts[0])).toBeVisible();
    expect(t.calls).toHaveLength(0);
    expect(t.errors).toEqual([]);
  });

  test("Services lists API data, filters, and drills into the detail page", async ({ page }) => {
    const t = track(page);
    await go(page, "/services", "Services");
    const svcs = (await apiGet(page, "/services")).data;
    expect(svcs.length).toBeGreaterThan(1);
    // (the demo provider reshuffles clusterIPs between calls, so match on name/namespace/type only)
    for (const s of svcs) await expect(row(page, s.name).filter({ hasText: s.namespace }).filter({ hasText: s.type }).first()).toBeVisible();
    const target = svcs[0];
    await page.getByPlaceholder("Search service name").fill(target.name);
    await expect(row(page, target.name).filter({ hasText: target.namespace }).first()).toBeVisible();
    await page.getByPlaceholder("Search service name").fill(`nothing-${uid()}`);
    await expect(page.getByText("No services match the current filters.")).toBeVisible();
    await page.getByPlaceholder("Search service name").fill(target.name);
    await page.getByRole("link", { name: target.name, exact: true }).first().click();
    await expect(page).toHaveURL(new RegExp(`/services/${target.namespace}/${target.name}$`));
    await expect(page.getByRole("heading", { name: target.name }).first()).toBeVisible();
    expect(t.calls).toHaveLength(0);
    expect(t.errors).toEqual([]);
  });

  test("Storage lists every PVC the API holds, grouped by class", async ({ page }) => {
    const t = track(page);
    await go(page, "/storage", "Storage");
    const vols = (await apiGet(page, "/volumes")).data;
    expect(vols.length).toBeGreaterThan(0);
    const region = page.getByRole("region", { name: "Persistent volume claims" });
    for (const v of vols) {
      const r = region.getByRole("row").filter({ hasText: v.name }).filter({ hasText: v.namespace });
      await expect(r).toBeVisible();
      await expect(r).toContainText(v.capacity);
      await expect(r).toContainText(v.status);
    }
    const classes = new Set(vols.map((v) => v.storageClass || "—"));
    for (const c of classes) await expect(page.getByTestId(`storage-class-${c}`).or(page.locator(".stat").filter({ hasText: c })).first()).toBeVisible();
    expect(t.calls).toHaveLength(0);
    expect(t.errors).toEqual([]);
  });
});
