// @ts-check
// Functional specs for the troubleshooting features: change timeline, cluster
// health check, multi-pod workload logs, ephemeral debug containers, the node
// shell's disabled-by-default state and the kubectl port-forward helper.
//
// Same contract as flows-core.spec.js: drive the real UI, assert the mutating
// requests returned 2xx (track), assert the UI reflects it, and assert an
// independent API read agrees. Designed for a *demo-mode* server, where the
// seeded otel-collector Deployment has one crash-looping pod, the demo HPA
// "default/payments-api" targets a Deployment that is not in that namespace and
// the labs/qa-scratch PVC is waiting for its first consumer.
const { test, expect } = require("@playwright/test");
const { signIn, track, apiGet, apiSend, waitMutation, toastText, USERNAME, PASSWORD } = require("./helpers");

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

const AUTH = { Authorization: `Basic ${Buffer.from(`${USERNAME}:${PASSWORD}`).toString("base64")}` };

/** Raw GET that keeps the status and body of non-2xx responses (apiGet drops them). */
async function rawGet(page, path) {
  const res = await page.request.get(`/api/v1${path}`, { headers: AUTH });
  return { status: res.status(), text: await res.text() };
}

// ---------------------------------------------------------------- health check

test("Overview shows the health check panel and links to the full page", async ({ page }) => {
  const t = track(page);
  await page.goto("/overview");
  const panel = page.getByTestId("overview-health-panel");
  await expect(panel).toBeVisible();
  const first = page.getByTestId("overview-health-finding-0");
  await expect(first).toBeVisible();
  await expect(first).toContainText("crash-looping");

  const { data } = await apiGet(page, "/diagnose");
  expect(data.status).toBe("critical");
  expect(data.findings[0].severity).toBe("critical");
  await expect(first).toContainText(data.findings[0].name);

  await page.getByTestId("overview-health-link").click();
  await expect(page).toHaveURL(/\/health-check$/);
  await expect(page.getByTestId("health-check-page")).toBeVisible();
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("health check page lists findings with fixes, filters by severity and namespace", async ({ page }) => {
  const t = track(page);
  await page.getByTestId("nav-link-health-check").click();
  await expect(page.getByTestId("health-check-page")).toBeVisible();
  await expect(page.getByTestId("health-status")).toContainText("Critical");

  const { data } = await apiGet(page, "/diagnose");
  for (const sev of ["critical", "warning", "info"]) {
    await expect(page.getByTestId(`health-count-${sev}`)).toContainText(String(data.counts[sev]));
  }
  const crash = page.getByTestId("health-finding-0");
  await expect(crash).toContainText("otel-collector-04");
  await expect(crash).toContainText("Suggested fix");
  // Findings deep-link to the failing object.
  await crash.getByRole("link", { name: /otel-collector-04/ }).click();
  await expect(page).toHaveURL(/\/pods\/observability\/otel-collector-04$/);
  await page.goBack();

  // Severity filter: only the info finding(s) remain.
  await page.getByTestId("health-count-info").click();
  await expect(page.getByTestId("health-finding-0")).toContainText("first consumer");
  await expect(page.getByText("otel-collector-04")).toHaveCount(0);
  await page.getByTestId("health-count-info").click();

  // Namespace filter: payments is healthy.
  await page.getByTestId("health-namespace").selectOption("payments");
  await expect(page.getByTestId("health-clean")).toBeVisible();
  await expect(page.getByTestId("health-status")).toContainText("Healthy");
  const scoped = await apiGet(page, "/diagnose?namespace=payments");
  expect(scoped.data.findings).toEqual([]);

  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

// ---------------------------------------------------------------- timeline

test("timeline merges audit entries with events; filters and Events deep link work", async ({ page }) => {
  const t = track(page);
  // Create an audit entry so the feed has a user action to show.
  const restart = await apiSend(page, "POST", "/workloads/payments/Deployment/payments-api/restart");
  expect(restart.status).toBe(200);

  await page.goto("/events");
  await page.getByTestId("events-open-timeline").click();
  await expect(page).toHaveURL(/\/timeline$/);
  await expect(page.getByTestId("timeline-page")).toBeVisible();
  await expect(page.getByTestId("timeline-row-0")).toBeVisible();

  const api = await apiGet(page, "/timeline?limit=500");
  expect(api.status).toBe(200);
  const sources = new Set(api.data.map((e) => e.source));
  for (const s of ["event", "audit", "alert", "rollout"]) expect(sources.has(s)).toBe(true);

  const table = page.getByRole("region", { name: "Timeline entries" });
  await expect(table).toContainText("workload.restart payments/Deployment/payments-api");

  // Source filter: switching "audit" off removes the restart row.
  await page.getByTestId("timeline-source-audit").click();
  await expect(table).not.toContainText("workload.restart payments/Deployment/payments-api");
  await page.getByTestId("timeline-source-audit").click();
  await expect(table).toContainText("workload.restart");

  // Text search.
  await page.getByTestId("timeline-search").fill("BackOff");
  await expect(table).toContainText("BackOff");
  await expect(table).not.toContainText("workload.restart");
  await page.getByTestId("timeline-search").fill("zzz-no-such-text");
  await expect(page.getByTestId("timeline-empty")).toBeVisible();
  await page.getByTestId("timeline-search").fill("");

  // Namespace filter is reflected in the URL (shareable) and scopes the data.
  await page.getByTestId("timeline-namespace").selectOption("payments");
  await expect(page).toHaveURL(/namespace=payments/);
  await expect(table).not.toContainText("BackOff");
  await expect(table).toContainText("workload.restart");

  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("object-scoped timeline via URL shows a clearable filter chip", async ({ page }) => {
  await page.goto("/timeline?namespace=payments&object=Deployment%2Fpayments-api");
  await expect(page.getByTestId("timeline-object-chip")).toContainText("Deployment/payments-api");
  const { data } = await apiGet(page, "/timeline?namespace=payments&object=Deployment/payments-api");
  expect(data.length).toBeGreaterThan(0);
  await expect(page.getByTestId("timeline-row-0")).toBeVisible();
  await page.getByRole("button", { name: "Clear object filter" }).click();
  await expect(page.getByTestId("timeline-object-chip")).toHaveCount(0);
  expect(page.url()).not.toContain("object=");
});

test("WorkloadDetail 'What changed' tab lists the workload's rollouts and actions", async ({ page }) => {
  const t = track(page);
  const restart = await apiSend(page, "POST", "/workloads/payments/Deployment/payments-api/restart");
  expect(restart.status).toBe(200);

  await page.goto("/workloads/payments/Deployment/payments-api");
  await page.getByTestId("workload-tab-changes").click();
  const panel = page.getByTestId("workload-timeline");
  await expect(panel).toBeVisible();
  await expect(page.getByTestId("workload-timeline-row-0")).toBeVisible();
  await expect(panel).toContainText("Rollout revision");
  await expect(panel).toContainText("workload.restart");

  const { data } = await apiGet(page, "/timeline?namespace=payments&object=Deployment/payments-api&limit=100");
  await expect(panel.locator('[data-testid^="workload-timeline-row-"]')).toHaveCount(data.length);
  // Nothing from another workload leaks into the tab.
  await expect(panel).not.toContainText("checkout");

  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

// ---------------------------------------------------------------- workload logs

test("workload logs merge every pod, with regex filter, timestamps, previous and download", async ({ page }) => {
  const t = track(page);
  await page.getByTestId("nav-link-logs").click();
  await expect(page.getByTestId("logs-page")).toBeVisible();

  await page.getByTestId("logs-mode-workload").click();
  await page.getByTestId("logs-workload-select").selectOption("observability/Deployment/otel-collector");
  await page.getByTestId("logs-start").click();

  const stream = page.getByTestId("logs-stream");
  await expect(stream).toContainText("[otel-collector-01/otel-collector]");
  await expect(stream).toContainText("[otel-collector-04/otel-collector]");
  const raw = await page.request.get("/api/v1/workloads/observability/Deployment/otel-collector/logs?tail=100", { headers: AUTH });
  expect(raw.status()).toBe(200);
  const rawLines = (await raw.text()).trim().split("\n");
  await expect(page.getByTestId("logs-line-count")).toContainText(`${rawLines.length} lines`);
  expect(rawLines.every((l) => /^\[otel-collector-0[1-4]\/otel-collector\] \d{4}-/.test(l))).toBe(true);

  // Regex filter keeps only the crash-looping pod's error line; invalid regex is reported.
  await page.getByTestId("logs-filter").fill("ERROR|exited");
  await expect(page.getByTestId("logs-line-0")).toContainText("otel-collector-04");
  await expect(page.getByTestId("logs-line-count")).toContainText(`1/${rawLines.length} lines`);
  await page.getByTestId("logs-filter").fill("([unclosed");
  await expect(page.getByTestId("logs-filter-error")).toBeVisible();
  await page.getByTestId("logs-filter").fill("");
  await expect(page.getByTestId("logs-filter-error")).toHaveCount(0);

  // Timestamps toggle.
  await expect(stream).toContainText(/\d{4}-\d{2}-\d{2}T/);
  await page.getByTestId("logs-timestamps").uncheck();
  await expect(stream).not.toContainText(/\d{4}-\d{2}-\d{2}T/);
  await page.getByTestId("logs-timestamps").check();

  // Download carries the visible lines.
  const [download] = await Promise.all([page.waitForEvent("download"), page.getByTestId("logs-download").click()]);
  expect(download.suggestedFilename()).toBe("logs-observability_Deployment_otel-collector.txt");

  // Previous container: only the restarted pod has one.
  await page.getByTestId("logs-clear").click();
  await page.getByTestId("logs-previous").check();
  await page.getByTestId("logs-start").click();
  await expect(stream).toContainText("[otel-collector-04/otel-collector]");
  await expect(stream).not.toContainText("[otel-collector-01/otel-collector]");

  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("workload logs surface a server error when there are no previous logs", async ({ page }) => {
  await page.goto("/logs?workload=payments%2FDeployment%2Fpayments-api");
  await expect(page.getByTestId("logs-workload-select")).toHaveValue("payments/Deployment/payments-api");
  await page.getByTestId("logs-previous").check();
  await page.getByTestId("logs-start").click();
  await expect(page.getByRole("alert").filter({ hasText: "no previous terminated container" })).toBeVisible();
  const r = await rawGet(page, "/workloads/payments/Deployment/payments-api/logs?previous=true");
  expect(r.status).toBe(404);
});

// ---------------------------------------------------------------- debug container

test("ephemeral debug container: created via the pod page and audited", async ({ page }) => {
  const t = track(page);
  await page.goto("/pods/payments/payments-api-03");
  await page.getByRole("tab", { name: /Terminal/ }).click();
  const panel = page.getByTestId("debug-container-panel");
  await expect(panel).toBeVisible();
  await page.getByTestId("debug-container-target").selectOption("payments-api");

  const posted = waitMutation(page, "POST", /\/pods\/payments\/payments-api-03\/debug$/);
  await page.getByTestId("debug-container-start").click();
  const res = await posted;
  expect(res.status()).toBe(201);
  expect(t.find("POST", /\/debug$/)[0].body).toEqual({ target: "payments-api" });
  const created = await res.json();
  expect(created.container).toMatch(/^debugger-[0-9a-f]{6}$/);
  expect(created.image).toBe("busybox");

  const result = page.getByTestId("debug-container-result");
  await expect(result).toContainText(created.container);
  await expect(result).toContainText("target payments-api");
  await toastText(page, new RegExp(`Debug container ${created.container} added`));

  const audit = await apiGet(page, "/audit");
  expect(audit.data.some((a) => a.action === "pod.debug" && a.target === "payments/payments-api-03" && a.detail.includes(created.container))).toBe(true);

  // A client-supplied image is not accepted.
  t.allow(/POST .*\/debug$/);
  const bad = await apiSend(page, "POST", "/pods/payments/payments-api-03/debug", { image: "evil/root" });
  expect(bad.status).toBe(400);
  const missing = await apiSend(page, "POST", "/pods/payments/no-such-pod/debug", {});
  expect(missing.status).toBe(404);
  expect(t.errors).toEqual([]);
});

// ---------------------------------------------------------------- node shell

test("node shell is off by default and says why", async ({ page }) => {
  const t = track(page);
  const caps = await apiGet(page, "/debug/capabilities");
  expect(caps.data).toMatchObject({ nodeShell: false, ephemeralContainers: true });
  const refused = await rawGet(page, "/nodes/axiom-01/shell");
  expect(refused.status).toBe(403);
  expect(refused.text).toContain("AXIOM_ENABLE_NODE_SHELL");

  await page.goto("/nodes/axiom-01");
  await page.getByRole("tab", { name: /Shell/ }).click();
  await expect(page.getByTestId("node-shell-panel")).toBeVisible();
  await expect(page.getByTestId("node-shell-disabled")).toContainText("AXIOM_ENABLE_NODE_SHELL=true");
  await expect(page.getByRole("button", { name: /Open root shell/ })).toHaveCount(0);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

// ---------------------------------------------------------------- port-forward helper

test("pod ports offer a copyable kubectl port-forward command next to Open", async ({ page }) => {
  const t = track(page);
  try {
    await page.context().grantPermissions(["clipboard-read", "clipboard-write"]);
  } catch {
    // Firefox/WebKit have no such permission names; the toast assertion below still holds.
  }
  await page.goto("/pods/payments/payments-api-01");
  const copy = page.getByTestId("pod-port-forward-copy-8080");
  const open = page.getByTestId("pod-port-open-8080");
  await expect(copy).toBeVisible();
  await expect(open).toBeVisible();
  const expected = "kubectl port-forward -n payments pod/payments-api-01 8080:8080";
  await expect(copy).toHaveAttribute("title", expected);

  await copy.click();
  await toastText(page, /Copied/);
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: expected }).first()).toBeVisible();
  const clip = await page.evaluate(() => navigator.clipboard.readText().catch(() => null));
  if (clip !== null) expect(clip).toBe(expected);

  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});
