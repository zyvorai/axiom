// @ts-check
// Experimental actual-bill reconciliation (Ops -> Cost invoice card and the
// Settings form). Demo mode serves deterministic canned invoice data, so this
// proves the UI <-> API contract only; the AWS / Azure / GCP clients are
// covered by Go tests against httptest fixtures and have not been verified
// against real cloud accounts.
const { test, expect } = require("@playwright/test");
const { signIn, track, apiGet } = require("./helpers");

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

test("the cost page compares the canned invoice with the allocated cost", async ({ page }) => {
  const t = track(page);
  const { status, data } = await apiGet(page, "/cost/invoice");
  expect(status).toBe(200);
  expect(data.experimental).toBe(true);
  expect(data.demo).toBe(true);
  expect(data.configured).toBe(true);
  expect(data.months).toHaveLength(2);
  const [prev, cur] = data.months;
  expect(prev.invoiceTotal).toBe(4812.4);
  expect(prev.unallocated).toBe(837.2);
  expect(cur.partial).toBe(true);

  await page.goto("/ops/cost");
  const card = page.getByTestId("cost-invoice-card");
  await expect(card).toBeVisible();
  await expect(page.getByTestId("cost-invoice-experimental")).toHaveText(/Experimental/);
  await expect(page.getByTestId("cost-invoice-provider")).toHaveText("AWS Cost Explorer");
  await expect(page.getByTestId(`cost-invoice-row-${prev.month}`)).toBeVisible();
  await expect(page.getByTestId(`cost-invoice-total-${prev.month}`)).toHaveText("$4,812.40");
  await expect(page.getByTestId(`cost-invoice-allocated-${prev.month}`)).toHaveText("$3,975.20");
  await expect(page.getByTestId(`cost-invoice-unallocated-${prev.month}`)).toHaveText("$837.20");
  await expect(page.getByTestId(`cost-invoice-row-${cur.month}`)).toContainText("(to date)");
  await expect(page.getByTestId("cost-invoice-note")).toContainText("Demo data");
  await expect(page.getByTestId("cost-invoice-error")).toHaveCount(0);
  expect(t.errors).toEqual([]);
});

test("settings shows the provider form with write-only credentials", async ({ page }) => {
  const t = track(page);
  await page.goto("/settings");
  const card = page.getByTestId("billing-settings-card");
  await expect(card).toBeVisible();
  await expect(card).toContainText("Experimental");

  const provider = page.getByTestId("billing-settings-provider");
  await provider.selectOption("aws");
  await expect(page.getByTestId("billing-settings-aws-secret-access-key")).toHaveAttribute("type", "password");
  await expect(page.getByTestId("billing-settings-aws-region")).toHaveValue("us-east-1");
  await provider.selectOption("azure");
  await expect(page.getByTestId("billing-settings-azure-subscription-id")).toBeVisible();
  await expect(page.getByTestId("billing-settings-aws-secret-access-key")).toHaveCount(0);
  await provider.selectOption("gcp");
  await expect(page.getByTestId("billing-settings-gcp-table")).toBeVisible();
  await expect(page.getByTestId("billing-settings-gcp-service-account-key")).toBeVisible();

  // A malformed table name is refused by the server and nothing is stored.
  t.allow(/PUT \/api\/v1\/integrations$/);
  await page.getByTestId("billing-settings-gcp-table").fill("proj.ds.tbl` OR 1=1 --");
  const put = page.waitForResponse((r) => r.request().method() === "PUT" && r.url().endsWith("/api/v1/integrations"));
  await page.getByTestId("billing-settings-save").click();
  expect((await put).status()).toBe(400);
  const integ = await apiGet(page, "/integrations");
  expect(integ.data.billingGcpTable).toBe("");
  expect(integ.data).toHaveProperty("billingGcpServiceAccountKeySet");
  expect(integ.data).not.toHaveProperty("billingGcpServiceAccountKey");

  // Demo mode answers the connection test without contacting anything.
  const test = page.waitForResponse((r) => r.url().endsWith("/api/v1/integrations/billing/test"));
  await page.getByTestId("billing-settings-test").click();
  const res = await test;
  expect(res.status()).toBe(200);
  expect((await res.json()).ok).toBe(true);
  expect(t.failures()).toEqual([]);
});
