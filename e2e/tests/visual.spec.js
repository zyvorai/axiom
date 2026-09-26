// Visual regression: full-viewport screenshots of the key screens in both themes, compared with
// committed baselines (tests/visual.spec.js-snapshots/). OPT-IN — not part of `npm test`/CI:
// baselines are OS-specific (font rendering differs between macOS and Linux), so they are recorded
// on macOS. Run against a demo server:
//   VISUAL=1 AXIOM_E2E_BASE_URL=http://localhost:5091 npx playwright test --project=visual
// After an intentional UI change, review the diff images and refresh with `--update-snapshots`.
const { test, expect } = require("@playwright/test");

const PAGES = [
  ["overview", "/overview"],
  ["getting-started", "/getting-started"],
  ["nodes", "/nodes"],
  ["pods", "/pods"],
  ["workloads", "/workloads"],
  ["vm-detail", "/vms/default/edge-api-vm"],
  ["settings", "/settings"],
  ["not-found", "/this/page/does-not-exist"],
];

async function signIn(page) {
  await page.goto("/login");
  await page.getByRole("textbox").first().fill(process.env.AXIOM_USERNAME || "demo");
  await page.getByRole("button", { name: /continue/i }).click();
  await page.getByPlaceholder("Password").fill(process.env.AXIOM_PASSWORD || "demo");
  await page.getByRole("button", { name: /sign in to axiom/i }).click();
  await page.waitForURL(/overview/);
}

// anything that legitimately changes between runs (relative times, live counters) is masked
const dynamic = (page) => [page.locator("time, [data-volatile]")];

for (const theme of ["dark", "light"]) {
  test.describe(`visual · ${theme}`, () => {
    test.describe.configure({ mode: "serial" });
    let context;
    let page;

    test.beforeAll(async ({ browser }) => {
      context = await browser.newContext({ viewport: { width: 1280, height: 800 }, reducedMotion: "reduce", ignoreHTTPSErrors: true });
      await context.addInitScript((t) => localStorage.setItem("axiom-theme", t), theme);
      page = await context.newPage();
      await signIn(page);
    });
    test.afterAll(async () => context?.close());

    for (const [name, url] of PAGES) {
      test(name, async () => {
        await page.goto(url, { waitUntil: "networkidle" });
        await page.waitForTimeout(600); // let charts/skeletons settle
        await expect(page).toHaveScreenshot(`${theme}-${name}.png`, { mask: dynamic(page) });
      });
    }

    test("run-workload dialog", async () => {
      await page.goto("/workloads", { waitUntil: "networkidle" });
      await page.getByRole("button", { name: /run workload/i }).click();
      await page.getByRole("dialog").waitFor();
      await page.waitForTimeout(400);
      await expect(page).toHaveScreenshot(`${theme}-run-workload-dialog.png`, { mask: dynamic(page) });
    });
  });

  // signed-out screens need a fresh, unauthenticated context
  test.describe(`visual · ${theme} · signed out`, () => {
    test.use({ viewport: { width: 1280, height: 800 }, reducedMotion: "reduce" });
    test.beforeEach(async ({ page }) => {
      await page.addInitScript((t) => localStorage.setItem("axiom-theme", t), theme);
    });
    test("login hero", async ({ page }) => {
      await page.goto("/login", { waitUntil: "networkidle" });
      await page.evaluate(() => { const s = document.querySelector(".login-store-scroll"); if (s) s.scrollTop = 0; });
      await page.waitForTimeout(500);
      await expect(page).toHaveScreenshot(`${theme}-login-hero.png`);
    });
    test("login password step", async ({ page }) => {
      await page.goto("/login", { waitUntil: "networkidle" });
      await page.getByRole("textbox").first().fill("demo");
      await page.getByRole("button", { name: /continue/i }).click();
      await page.getByPlaceholder("Password").waitFor();
      await page.waitForTimeout(500);
      await expect(page).toHaveScreenshot(`${theme}-login-password.png`);
    });
  });
}
