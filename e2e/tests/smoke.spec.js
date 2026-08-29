// @ts-check
const { test, expect } = require("@playwright/test");

// Smoke tests replacing this project's previously-manual Chrome-based
// verification with something that runs unattended in CI. Deliberately
// narrow: these prove the app boots, authenticates, and its core
// navigation surfaces work — not a full regression suite for every page.
const USERNAME = process.env.AXIOM_E2E_USERNAME || "demo";
const PASSWORD = process.env.AXIOM_E2E_PASSWORD || "demo";

async function signIn(page) {
  await page.goto("/");
  await page.getByPlaceholder("Username").fill(USERNAME);
  await page.getByPlaceholder("Password").fill(PASSWORD);
  await page.getByRole("button", { name: "Sign in" }).click();
  await expect(page.getByTestId("sidebar-nav")).toBeVisible();
}

test("login page renders", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("login-page")).toBeVisible();
  await expect(page.getByPlaceholder("Username")).toBeVisible();
  await expect(page.getByPlaceholder("Password")).toBeVisible();
});

test("local sign-in reaches the Overview page", async ({ page }) => {
  await signIn(page);
  await expect(page.getByTestId("overview-page")).toBeVisible();
});

test("sidebar navigation reaches the Nodes page", async ({ page }) => {
  await signIn(page);
  await page.getByTestId("nav-link-nodes").click();
  await expect(page.getByTestId("nodes-page")).toBeVisible();
});

test("command palette opens and navigates", async ({ page }) => {
  await signIn(page);
  await page.keyboard.press(process.platform === "darwin" ? "Meta+k" : "Control+k");
  await expect(page.getByTestId("command-palette")).toBeVisible();
  await page.getByTestId("command-palette-input").fill("Workloads");
  await page.getByTestId("palette-item-workloads").click();
  await expect(page).toHaveURL(/\/workloads$/);
});

test("health and metrics endpoints respond", async ({ request, baseURL }) => {
  const health = await request.get(`${baseURL}/api/v1/health`);
  expect(health.ok()).toBeTruthy();
  const healthBody = await health.json();
  expect(healthBody.status).toBe("ok");

  const metrics = await request.get(`${baseURL}/api/v1/metrics`);
  expect(metrics.ok()).toBeTruthy();
  const metricsBody = await metrics.text();
  expect(metricsBody).toContain("axiom_build_info");
});
