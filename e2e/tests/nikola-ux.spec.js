// @ts-check
/**
 * UX exercise against the lab: ensure workload "nikola" exists (create if needed),
 * keep it, and run several console operations.
 *
 *   AXIOM_E2E_BASE_URL=https://<host>:30590 npx playwright test tests/nikola-ux.spec.js
 */
const { test, expect } = require("@playwright/test");

const USERNAME = process.env.AXIOM_E2E_USERNAME || "demo";
const PASSWORD = process.env.AXIOM_E2E_PASSWORD || "demo";
const NAME = "nikola";

async function signIn(page) {
  await page.goto("/");
  // Already authenticated sessions land on overview
  if (await page.getByTestId("sidebar-nav").isVisible().catch(() => false)) return;
  await page.getByPlaceholder("Username").fill(USERNAME);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.getByPlaceholder("Password").fill(PASSWORD);
  await page.getByRole("button", { name: /Sign in/ }).click();
  await expect(page.getByTestId("sidebar-nav")).toBeVisible({ timeout: 30000 });
}

async function openWorkloadMenu(page, name) {
  const row = page.locator("tr").filter({ hasText: name }).first();
  await expect(row).toBeVisible({ timeout: 60000 });
  await row.getByRole("button").last().click();
}

test("create nikola from UX and exercise ops (keep it)", async ({ page }) => {
  test.setTimeout(240000);
  const results = [];

  await signIn(page);
  results.push("login ok");

  await page.getByTestId("nav-link-workloads").click();
  await expect(page.getByRole("heading", { name: "Workloads" })).toBeVisible();

  const existing = page.locator("tr").filter({ hasText: NAME }).first();
  if (!(await existing.isVisible().catch(() => false))) {
    await page.getByRole("button", { name: /Run workload/ }).click();
    await expect(page.getByRole("heading", { name: "Run workload" })).toBeVisible();
    const dialog = page.getByRole("dialog");
    await dialog.locator('input[placeholder="checkout-worker"]').fill(NAME);
    await dialog.locator('input[placeholder="ghcr.io/zyvorai/checkout:v1"]').fill("nginx:alpine");
    await dialog.getByRole("button", { name: "Launch" }).click();
    await expect(page.locator("tr").filter({ hasText: NAME }).first()).toBeVisible({ timeout: 60000 });
    results.push("launch nikola ok");
  } else {
    results.push("nikola already present — skip launch");
  }

  // --- Restart ---
  await openWorkloadMenu(page, NAME);
  await page.getByRole("menuitem", { name: /Restart/ }).click();
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: /restart requested/i }).first()).toBeVisible({ timeout: 15000 });
  results.push("restart ok");
  await page.waitForTimeout(1500);

  // --- Scale to 2 ---
  await openWorkloadMenu(page, NAME);
  await page.getByRole("menuitem", { name: /Scale/ }).click();
  let scaleDialog = page.getByRole("dialog").filter({ hasText: /Scale/ });
  await expect(scaleDialog).toBeVisible();
  await scaleDialog.locator('input[type="number"]').fill("2");
  await scaleDialog.getByRole("button", { name: "Scale" }).click();
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: /scaled to 2/i }).first()).toBeVisible({ timeout: 15000 });
  results.push("scale→2 ok");
  await page.waitForTimeout(1500);

  // --- Rollout ---
  await openWorkloadMenu(page, NAME);
  await page.getByRole("menuitem", { name: /Rollout/ }).click();
  const rollout = page.getByRole("dialog").filter({ hasText: /Rollout/ });
  await expect(rollout).toBeVisible({ timeout: 15000 });
  await rollout.getByRole("button", { name: "restart", exact: true }).click();
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: /restart requested/i }).first()).toBeVisible({ timeout: 15000 });
  await page.keyboard.press("Escape");
  results.push("rollout restart ok");
  await page.waitForTimeout(1000);

  // --- Scale back to 1 ---
  await openWorkloadMenu(page, NAME);
  await page.getByRole("menuitem", { name: /Scale/ }).click();
  scaleDialog = page.getByRole("dialog").filter({ hasText: /Scale/ });
  await scaleDialog.locator('input[type="number"]').fill("1");
  await scaleDialog.getByRole("button", { name: "Scale" }).click();
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: /scaled to 1/i }).first()).toBeVisible({ timeout: 15000 });
  results.push("scale→1 ok");
  await page.waitForTimeout(1500);

  // --- Stop / Start ---
  await openWorkloadMenu(page, NAME);
  await page.getByRole("menuitem", { name: /^Stop$/ }).click();
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: /stop requested/i }).first()).toBeVisible({ timeout: 15000 });
  results.push("stop ok");
  await page.waitForTimeout(2500);
  await openWorkloadMenu(page, NAME);
  await page.getByRole("menuitem", { name: /^Start$/ }).click();
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: /start requested/i }).first()).toBeVisible({ timeout: 15000 });
  results.push("start ok");
  await page.waitForTimeout(3000);

  // --- Workload detail ---
  await page.locator("tr").filter({ hasText: NAME }).first().getByRole("link", { name: NAME }).click();
  await expect(page.getByText(NAME).first()).toBeVisible({ timeout: 15000 });
  results.push("workload detail ok");

  // --- Pods ---
  await page.getByTestId("nav-link-pods").click();
  await expect(page.getByRole("heading", { name: /Pods/i })).toBeVisible({ timeout: 15000 });
  const search = page.getByPlaceholder(/Search/i).first();
  if (await search.isVisible().catch(() => false)) await search.fill(NAME);

  const podLink = page.getByRole("link").filter({ hasText: new RegExp(`^${NAME}-`) }).first();
  await expect(podLink).toBeVisible({ timeout: 120000 });
  const podName = (await podLink.innerText()).trim();
  await podLink.click();
  results.push(`pod detail: ${podName}`);

  await expect(page.getByText(/Pod ·|Containers|Ready/i).first()).toBeVisible({ timeout: 20000 });
  results.push("details ok");

  await page.getByRole("tab", { name: /Metrics/ }).click();
  await page.waitForTimeout(1500);
  results.push("metrics tab ok");

  await page.getByRole("tab", { name: /Events/ }).click();
  await page.waitForTimeout(1000);
  results.push("events tab ok");

  await page.getByRole("tab", { name: /Labels/ }).click();
  await page.waitForTimeout(1000);
  results.push("labels tab ok");

  await page.getByRole("tab", { name: /YAML/ }).click();
  await expect(page.locator("textarea").first()).toBeVisible({ timeout: 10000 });
  results.push("yaml tab ok");

  // Terminal / one-shot exec
  await page.getByRole("tab", { name: /Terminal/ }).click();
  await page.waitForTimeout(1500);
  const lsInput = page.locator('input[value="ls -la"]').first();
  if (await lsInput.count()) {
    await lsInput.fill("hostname");
    await page.getByRole("button", { name: /^Run$/ }).click();
    await page.waitForTimeout(4000);
    results.push("exec hostname ok");
  } else {
    // Live interactive terminal may be available instead of one-shot exec
    results.push("terminal tab ok (interactive or empty)");
  }

  // Logs
  await page.getByRole("link", { name: /Logs/ }).first().click();
  // .first() because the redesigned Logs page legitimately has both a
  // heading and the testid at once — plain .or() hits Playwright's strict
  // mode (multiple matches) instead of "either is fine".
  await expect(page.getByRole("heading", { name: /Logs|Pod Logs/i }).or(page.getByTestId("logs-page")).first()).toBeVisible({
    timeout: 20000,
  });
  const startBtn = page.getByTestId("logs-start").or(page.getByRole("button", { name: /^Start$/i }));
  if (await startBtn.first().isVisible().catch(() => false)) {
    await startBtn.first().click();
    await page.waitForTimeout(3000);
    const stopBtn = page.getByTestId("logs-stop").or(page.getByRole("button", { name: /^Stop$/i }));
    if (await stopBtn.first().isVisible().catch(() => false)) await stopBtn.first().click();
    results.push("logs stream ok");
  } else {
    results.push("logs page ok");
  }

  // Keep — verify still present
  await page.getByTestId("nav-link-pods").click();
  if (await search.isVisible().catch(() => false)) await search.fill(NAME);
  await expect(page.getByRole("link").filter({ hasText: new RegExp(NAME) }).first()).toBeVisible({ timeout: 60000 });
  results.push("pod kept");

  await page.getByTestId("nav-link-workloads").click();
  await expect(page.locator("tr").filter({ hasText: NAME }).first()).toBeVisible({ timeout: 30000 });
  results.push("workload kept");

  console.log("\n=== nikola UX results ===");
  for (const r of results) console.log(" ✓", r);
  console.log("=========================\n");
});
