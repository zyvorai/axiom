// @ts-check
// Functional specs for the cross-page UX foundations: global namespace selector, URL-synced
// filters, saved views, keyboard shortcuts + help dialog, "system" theme, change review (YAML
// diff) before apply, the resource relationship graph and the notification inbox.
//
// Same contract as the other flows-*.spec.js: drive the real UI, `track` every mutating call and
// console error, and cross-check with an independent API read. Designed for a demo-mode server
// (a few assertions rely on seeded demo data and skip themselves elsewhere).
const { test, expect } = require("@playwright/test");
const { signIn, track, apiGet, apiSend, waitMutation, toastText } = require("./helpers");

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

const uid = () => Math.random().toString(36).slice(2, 8);
const globalNs = (page) => page.getByTestId("topbar-namespace");

let demoCache;
async function isDemo(page) {
  if (demoCache === undefined) {
    const r = await apiSend(page, "POST", "/apply", { manifest: "kind: Probe" });
    demoCache = /not available in demo mode/i.test(JSON.stringify(r.data || ""));
  }
  return demoCache;
}

/** Opens a sidebar page and waits for its table rows. */
async function go(page, path, heading) {
  await page.goto(path);
  if (heading) await expect(page.getByRole("heading", { name: heading, level: 1 })).toBeVisible();
  else await expect(page.getByTestId("overview-page")).toBeVisible(); // Overview's h1 is the cluster name
}
const bodyRows = (page) => page.locator("tbody tr");

// ---------------------------------------------------------------------------
// Namespace selector + URL-synced filters
// ---------------------------------------------------------------------------
test.describe("namespace selector and URL filters", () => {
  test("the top-bar namespace scopes every page, survives navigation and reload", async ({ page }) => {
    const t = track(page);
    const { data: pods } = await apiGet(page, "/pods");
    const inPayments = pods.filter((p) => p.namespace === "payments").length;
    expect(inPayments).toBeGreaterThan(0);
    expect(pods.length).toBeGreaterThan(inPayments);

    await go(page, "/pods", "Pods");
    await expect(bodyRows(page)).toHaveCount(pods.length);

    await globalNs(page).selectOption("payments");
    await expect(bodyRows(page)).toHaveCount(inPayments);
    for (const cell of await page.locator("tbody tr td:nth-child(3)").allTextContents()) expect(cell).toBe("payments");

    // other pages follow the same selection (client-side navigation keeps the shell mounted)
    await page.getByTestId("nav-link-workloads").click();
    await expect(page.getByRole("heading", { name: "Workloads", level: 1 })).toBeVisible();
    await expect(page.getByRole("link", { name: "payments-api", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "checkout", exact: true })).toHaveCount(0);
    await expect(page.getByLabel("Filter by namespace")).toContainText("payments");

    // persisted in this browser
    await page.reload();
    await expect(globalNs(page)).toHaveValue("payments");
    await expect(page.getByRole("link", { name: "payments-api", exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "checkout", exact: true })).toHaveCount(0);

    // back to everything
    await globalNs(page).selectOption("all");
    await expect(page.getByRole("link", { name: "checkout", exact: true })).toBeVisible();
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("search and namespace filters live in the URL: reload and back/forward keep them", async ({ page }) => {
    const t = track(page);
    await go(page, "/pods", "Pods");
    await page.getByPlaceholder("Search pod name").fill("checkout");
    await expect(page).toHaveURL(/[?&]q=checkout/);
    const filtered = await bodyRows(page).count();
    expect(filtered).toBeGreaterThan(0);
    for (const cell of await page.locator("tbody tr td.name").allTextContents()) expect(cell).toContain("checkout");

    await page.reload();
    await expect(page.getByPlaceholder("Search pod name")).toHaveValue("checkout");
    await expect(bodyRows(page)).toHaveCount(filtered);

    // a shared link restores a page-local namespace override on top of the global scope
    await page.goto("/pods?ns=commerce&q=checkout");
    await expect(page.getByPlaceholder("Search pod name")).toHaveValue("checkout");
    await expect(page.getByLabel("Filter by namespace")).toContainText("commerce");
    await expect(globalNs(page)).toHaveValue("all");
    await expect(bodyRows(page)).toHaveCount(filtered);

    // picking the global namespace in the top bar clears the page-local override
    await globalNs(page).selectOption("payments");
    await expect(page).not.toHaveURL(/[?&]ns=/);
    await expect(page).toHaveURL(/[?&]q=checkout/); // other filters are untouched
    await expect(page.getByLabel("Filter by namespace")).toContainText("payments");

    // other filtered pages use the same mechanism
    await page.goto("/events?type=Warning&ns=all");
    await expect(page.getByLabel("Filter by event type")).toContainText("Warning");
    const types = await page.locator("tbody tr td:first-child").allTextContents();
    expect(types.length).toBeGreaterThan(0);
    for (const cell of types) expect(cell).toMatch(/Warning/i);
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// Saved views
// ---------------------------------------------------------------------------
test("saved views: save the current filters, apply them later, survive reload, delete", async ({ page }) => {
  const t = track(page);
  const name = `pay-${uid()}`;
  await go(page, "/workloads", "Workloads");
  await page.getByPlaceholder("Search name or image").fill("api");
  await page.getByLabel("Filter by namespace").click();
  await page.getByRole("option", { name: "payments", exact: true }).click();
  await expect(page).toHaveURL(/ns=payments/);

  await page.getByTestId("saved-views-trigger").click();
  await page.getByTestId("saved-views-name").fill(name);
  await page.getByTestId("saved-views-save").click();
  await expect(page.getByTestId(`saved-view-${name}`)).toContainText("q=api");
  await page.keyboard.press("Escape");

  // clear the filters, then apply the view
  await page.goto("/workloads");
  await expect(page.getByPlaceholder("Search name or image")).toHaveValue("");
  await page.getByTestId("saved-views-trigger").click();
  await page.getByTestId(`saved-view-${name}`).click();
  await expect(page.getByPlaceholder("Search name or image")).toHaveValue("api");
  await expect(page).toHaveURL(/q=api/);
  await expect(page).toHaveURL(/ns=payments/);
  await expect(page.getByRole("link", { name: "payments-api", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "checkout", exact: true })).toHaveCount(0);

  // stored in this browser: still there after a reload; views are per page
  await page.reload();
  await page.getByTestId("saved-views-trigger").click();
  await expect(page.getByTestId(`saved-view-${name}`)).toBeVisible();
  await page.keyboard.press("Escape");
  await go(page, "/pods", "Pods");
  await page.getByTestId("saved-views-trigger").click();
  await expect(page.getByTestId(`saved-view-${name}`)).toHaveCount(0);
  await page.keyboard.press("Escape");

  await go(page, "/workloads", "Workloads");
  await page.getByTestId("saved-views-trigger").click();
  await page.getByTestId(`saved-view-delete-${name}`).click();
  await expect(page.getByTestId(`saved-view-${name}`)).toHaveCount(0);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

// ---------------------------------------------------------------------------
// Keyboard shortcuts
// ---------------------------------------------------------------------------
test("shortcuts: ? opens the help dialog, g+letter navigates, / focuses search, typing is never hijacked", async ({ page }) => {
  const t = track(page);
  await go(page, "/overview");

  await page.keyboard.press("?");
  const dlg = page.getByTestId("shortcuts-dialog");
  await expect(dlg).toBeVisible();
  await expect(dlg.getByRole("heading", { name: "Keyboard shortcuts" })).toBeVisible();
  await expect(dlg).toContainText("Go to Pods");
  await expect(dlg).toContainText("Focus the search box");
  await page.keyboard.press("Escape");
  await expect(dlg).toBeHidden();

  await page.keyboard.press("g");
  await page.keyboard.press("p");
  await expect(page).toHaveURL(/\/pods$/);
  await page.keyboard.press("g");
  await page.keyboard.press("w");
  await expect(page).toHaveURL(/\/workloads/);
  await page.keyboard.press("g");
  await page.keyboard.press("e");
  await expect(page).toHaveURL(/\/events/);
  await page.keyboard.press("g");
  await page.keyboard.press("n");
  await expect(page).toHaveURL(/\/namespaces/);

  await page.goto("/pods");
  await expect(page.getByRole("heading", { name: "Pods", level: 1 })).toBeVisible();
  await page.keyboard.press("/");
  const search = page.getByPlaceholder("Search pod name");
  await expect(search).toBeFocused();
  // while typing in the field, shortcut keys are plain characters
  await search.pressSequentially("g?p/");
  await expect(search).toHaveValue("g?p/");
  await expect(page.getByTestId("shortcuts-dialog")).toBeHidden();
  await expect(page).toHaveURL(/\/pods/);

  // reachable from the command palette too
  await page.keyboard.press("Escape");
  await page.getByTestId("topbar-search").click();
  await page.getByTestId("palette-item-shortcuts").click();
  await expect(page.getByTestId("shortcuts-dialog")).toBeVisible();
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

// ---------------------------------------------------------------------------
// Theme: system
// ---------------------------------------------------------------------------
test("theme: light/dark/system, and system follows the OS colour scheme live", async ({ page }) => {
  const t = track(page);
  const html = page.locator("html");
  await page.goto("/settings");
  const group = page.getByRole("radiogroup", { name: "Theme" });
  await group.getByRole("radio", { name: "Light" }).click();
  await expect(html).toHaveAttribute("data-theme", "light");

  await page.emulateMedia({ colorScheme: "dark" });
  await group.getByRole("radio", { name: "System" }).click();
  await expect(group.getByRole("radio", { name: "System" })).toHaveAttribute("aria-checked", "true");
  await expect(html).toHaveAttribute("data-theme", "dark");
  await page.emulateMedia({ colorScheme: "light" });
  await expect(html).toHaveAttribute("data-theme", "light");

  // the choice (not the resolved value) is what is stored, and the pre-paint script honours it
  expect(await page.evaluate(() => localStorage.getItem("axiom-theme"))).toBe("system");
  await page.reload();
  await expect(html).toHaveAttribute("data-theme", "light");
  await expect(page.getByRole("radiogroup", { name: "Theme" }).getByRole("radio", { name: "System" })).toHaveAttribute("aria-checked", "true");

  // the top-bar button cycles light -> dark -> system
  await page.getByRole("radiogroup", { name: "Theme" }).getByRole("radio", { name: "Light" }).click();
  await page.getByRole("button", { name: /Toggle theme/ }).click();
  await expect(html).toHaveAttribute("data-theme", "dark");
  await page.getByRole("button", { name: /Toggle theme/ }).click();
  expect(await page.evaluate(() => localStorage.getItem("axiom-theme"))).toBe("system");
  expect(t.errors).toEqual([]);
});

// ---------------------------------------------------------------------------
// YAML diff gate before apply
// ---------------------------------------------------------------------------
const NETPOL = (name) => `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: ${name}
  namespace: default
spec:
  podSelector: {}
  policyTypes:
    - Ingress
`;

test("apply dialog: Apply stays disabled until the changes were reviewed, and an edit invalidates the review", async ({ page }) => {
  const t = track(page);
  const name = `e2e-diff-${uid()}`;
  await go(page, "/networking", "Networking");
  await page.getByRole("button", { name: "Create via YAML" }).click();
  const dlg = page.getByRole("dialog");
  const apply = dlg.getByRole("button", { name: "Apply", exact: true });
  const review = dlg.getByRole("button", { name: /^Review/ });

  await dlg.locator("textarea").fill(NETPOL(name));
  await expect(apply).toBeDisabled(); // not reviewed yet
  const diffCall = waitMutation(page, "POST", /\/apply\/diff$/);
  await review.click();
  expect((await diffCall).status()).toBe(200);
  expect(t.find("POST", /\/apply\/diff$/)[0].body.manifest).toContain(`name: ${name}`);

  // a new object: the whole manifest shows as additions, with the "will be created" heading
  await expect(dlg.getByRole("heading", { name: new RegExp(`NetworkPolicy default/${name} — will be created`) })).toBeVisible();
  const view = dlg.getByTestId("diff-view");
  await expect(view.locator('[data-diff="add"]').first()).toBeVisible();
  await expect(view.locator('[data-diff="del"]')).toHaveCount(0);
  await expect(dlg.getByTestId("diff-stats")).toContainText(/\+\d+/);
  await expect(apply).toBeEnabled();
  expect(t.find("POST", /\/apply$/)).toHaveLength(0); // reviewing never applies

  // editing the manifest after the review invalidates it
  await dlg.locator("textarea").fill(NETPOL(name) + "  # edited\n");
  await expect(apply).toBeDisabled();
  await expect(dlg.getByTestId("diff-view")).toHaveCount(0);
  await dlg.getByRole("button", { name: /^Review/ }).click();
  await expect(apply).toBeEnabled();

  // Apply is real: demo says it cannot apply, and the UI reports that rather than success
  const applied = waitMutation(page, "POST", /\/apply$/);
  await apply.click();
  expect((await applied).status()).toBe(200);
  if (await isDemo(page)) {
    await expect(dlg.getByText(/^apply is not available in demo mode/)).toBeVisible();
    expect((await apiGet(page, "/networkpolicies")).data.map((p) => p.name)).not.toContain(name);
  }
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("apply dialog: an invalid manifest is rejected at review time and can never be applied", async ({ page }) => {
  const t = track(page);
  await go(page, "/networking", "Networking");
  await page.getByRole("button", { name: "Create via YAML" }).click();
  const dlg = page.getByRole("dialog");
  await dlg.locator("textarea").fill("metadata:\n  name: nope\n");
  await dlg.getByRole("button", { name: /^Review/ }).click();
  await expect(dlg.getByRole("alert")).toContainText(/kind and apiVersion are required/);
  await expect(dlg.getByRole("button", { name: "Apply", exact: true })).toBeDisabled();
  expect(t.find("POST", /\/apply$/)).toHaveLength(0);
  expect(t.errors).toEqual([]);
});

test("workload YAML tab: Save opens a diff review showing the edit and the foreign-manager warning, then applies", async ({ page }) => {
  test.skip(!(await isDemo(page)), "relies on the seeded demo workload that Argo CD 'also manages'");
  const t = track(page);
  await page.goto("/workloads/payments/Deployment/payments-api");
  await expect(page.getByRole("heading", { name: "payments-api" })).toBeVisible();
  await page.getByRole("tab", { name: /YAML/ }).click();
  const editor = page.getByRole("textbox").first();
  const manifest = JSON.parse(await editor.inputValue());
  manifest.metadata.labels = { edited: "yes" };
  await editor.fill(JSON.stringify(manifest, null, 2));

  await page.getByRole("button", { name: "Save" }).click();
  const dlg = page.getByTestId("diff-review-dialog");
  await expect(dlg).toBeVisible();
  await expect(dlg.getByTestId("diff-warning")).toContainText("argocd-application-controller");
  await expect(dlg.locator('[data-diff="add"]').filter({ hasText: '"edited": "yes"' })).toBeVisible();
  await expect(dlg.getByTestId("diff-stats")).toContainText("+");
  const apply = dlg.getByRole("button", { name: "Apply", exact: true });
  await expect(apply).toBeEnabled();
  expect(t.find("POST", /\/apply$/)).toHaveLength(0);

  const applied = waitMutation(page, "POST", /\/apply$/);
  await apply.click();
  expect((await applied).status()).toBe(200);
  await toastText(page, /Apply failed/); // demo must not claim success
  expect(t.find("POST", /\/apply\/diff$/).length).toBeGreaterThan(0);

  // cancelling a review applies nothing (the failed apply left the dialog open so the edit is not lost)
  await expect(dlg).toBeVisible();
  await dlg.getByRole("button", { name: "Cancel" }).click();
  await expect(dlg).toBeHidden();
  await page.getByRole("button", { name: "Save" }).click();
  await expect(dlg).toBeVisible();
  const before = t.find("POST", /\/apply$/).length;
  await dlg.getByRole("button", { name: "Cancel" }).click();
  await expect(dlg).toBeHidden();
  expect(t.find("POST", /\/apply$/).length).toBe(before);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("apply/diff API: dry-run preview never changes anything and masks Secret values", async ({ page }) => {
  const before = (await apiGet(page, "/workloads")).data.length;
  const r = await apiSend(page, "POST", "/apply/diff", {
    manifest: JSON.stringify({ apiVersion: "v1", kind: "Secret", metadata: { name: "s", namespace: "default" }, stringData: { token: "hunter2-e2e" } }),
  });
  expect(r.status).toBe(200);
  expect(JSON.stringify(r.data)).not.toContain("hunter2-e2e");
  expect((await apiGet(page, "/workloads")).data.length).toBe(before);
  expect((await apiSend(page, "POST", "/apply/diff", "not-json")).status).toBe(400);
});

// ---------------------------------------------------------------------------
// Relationship graph
// ---------------------------------------------------------------------------
test("graph tab: workload relationships render as an accessible graph with working links", async ({ page }) => {
  const t = track(page);
  const { status, data: g } = await apiGet(page, "/graph/payments/Deployment/payments-api");
  expect(status).toBe(200);
  expect(g.root).toBe("deployment/payments/payments-api");
  const kinds = new Set(g.nodes.map((n) => n.kind));
  for (const k of ["Deployment", "Pod", "Service", "Ingress"]) expect(kinds.has(k)).toBe(true);

  await page.goto("/workloads/payments/Deployment/payments-api");
  await expect(page.getByRole("heading", { name: "payments-api" })).toBeVisible();
  await page.getByRole("tab", { name: "Graph" }).click();
  const root = page.getByTestId("resource-graph");
  await expect(root.getByRole("region", { name: "Relationship graph" })).toBeVisible();
  await expect(page.getByTestId("graph-node-deployment/payments/payments-api")).toHaveAttribute("aria-label", /this resource/);
  await expect(page.getByTestId("graph-node-service/payments/payments-api")).toBeVisible();
  await expect(page.getByTestId("graph-node-ingress/payments/payments-api")).toBeVisible();
  await expect(root.locator('[data-kind="Pod"]').first()).toBeVisible();

  // text alternative for screen readers / keyboard users
  await root.getByText("Show relationships as a list").click();
  const list = page.getByTestId("resource-graph-list");
  await expect(list).toContainText("Ingress payments-api routes Service payments-api");
  await expect(list).toContainText("Service payments-api selects Deployment payments-api");

  // nodes are keyboard-focusable links
  const svc = page.getByTestId("graph-node-service/payments/payments-api");
  await svc.focus();
  await expect(svc).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(page).toHaveURL(/\/services\/payments\/payments-api/);

  // pods link to their own page
  await page.goBack();
  await page.getByRole("tab", { name: "Graph" }).click();
  await root.locator('[data-kind="Pod"]').first().click();
  await expect(page).toHaveURL(/\/pods\/payments\/payments-api-/);

  // unknown workload: honest error, no crash
  const missing = await apiGet(page, "/graph/payments/Deployment/does-not-exist");
  expect(missing.status).toBe(404);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

// ---------------------------------------------------------------------------
// Notification inbox
// ---------------------------------------------------------------------------
test("notification inbox: lists cluster warnings, read and dismiss persist locally", async ({ page }) => {
  const t = track(page);
  const { data: alerts } = await apiGet(page, "/alerts");
  const warnings = alerts.warnings || [];
  test.skip(warnings.length < 1, "needs at least one active warning");
  const n = warnings.length;

  await go(page, "/overview");
  const bell = page.getByTestId("inbox-bell");
  await expect(bell).toHaveAccessibleName(new RegExp(`Notifications, ${n} unread`));
  await bell.click();
  const panel = page.getByTestId("inbox-panel");
  await expect(panel).toBeVisible();
  const items = panel.locator('[data-testid^="inbox-item-"]');
  await expect(items).toHaveCount(n);
  await expect(items.first()).toContainText(warnings[0].reason);
  await expect(items.first()).toContainText(warnings[0].message);

  // mark all read: no unread badge any more, the items are still listed, and it is remembered
  await panel.getByTestId("inbox-mark-all-read").click();
  await expect(panel.locator('[data-read="false"]')).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("inbox-bell")).toHaveAccessibleName("Notifications");
  await page.reload();
  await expect(page.getByTestId("inbox-bell")).toHaveAccessibleName("Notifications");
  await page.getByTestId("inbox-bell").click();
  await expect(page.getByTestId("inbox-panel").locator('[data-read="true"]')).toHaveCount(n);

  // dismiss: gone from the panel (empty state once all are), and stays gone after a reload
  for (let i = n - 1; i >= 0; i--) await page.getByTestId(`inbox-dismiss-${i}`).click();
  await expect(page.getByTestId("inbox-empty")).toBeVisible();
  await page.keyboard.press("Escape");
  await page.reload();
  await page.getByTestId("inbox-bell").click();
  await expect(page.getByTestId("inbox-empty")).toBeVisible();

  // the full list is one click away; dismissing is local, the server still reports every warning
  await page.getByRole("link", { name: "View all alerts" }).click();
  await expect(page).toHaveURL(/\/alerts/);
  expect(((await apiGet(page, "/alerts")).data.warnings || []).length).toBe(n);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});
