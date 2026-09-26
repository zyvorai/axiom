// @ts-check
// Functional specs: submit the create/edit/delete flows and verify the result
// end to end (request status + UI + independent API read). Designed to run
// against a *demo-mode* server — every test creates its own uniquely-named
// resources and cleans up after itself. See e2e/README.md.
const { test, expect } = require("@playwright/test");
const { signIn, track, apiGet, waitMutation, confirmDialog, toastText } = require("./helpers");

const uid = () => Math.random().toString(36).slice(2, 8);

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

test("namespaces: create, appears in list and API, delete", async ({ page }) => {
  const t = track(page);
  const name = `e2e-ns-${uid()}`;

  await page.getByTestId("nav-link-namespaces").click();
  await page.getByRole("button", { name: "Create namespace" }).first().click();
  const dlg = page.getByRole("dialog");
  await dlg.getByPlaceholder("tenant-b").fill(name);
  const posted = waitMutation(page, "POST", /\/namespaces$/);
  await dlg.getByRole("button", { name: "Create", exact: true }).click();
  expect((await posted).status()).toBeLessThan(300);

  await toastText(page, new RegExp(`${name} created`));
  await expect(page.getByTestId(`namespace-card-${name}`)).toBeVisible();
  const list = await apiGet(page, "/namespaces");
  expect(list.data.map((n) => n.name)).toContain(name);

  const del = waitMutation(page, "DELETE", new RegExp(`/namespaces/${name}$`));
  await page.getByRole("button", { name: `Delete namespace ${name}` }).click();
  await confirmDialog(page);
  expect((await del).status()).toBeLessThan(300);
  await expect(page.getByTestId(`namespace-card-${name}`)).toHaveCount(0);
  const after = await apiGet(page, "/namespaces");
  expect(after.data.map((n) => n.name)).not.toContain(name);

  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});
