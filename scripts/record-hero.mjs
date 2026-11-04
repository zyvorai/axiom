// Records the hero product clip from a running DEMO server.
// node record.mjs <baseUrl> <outDir> <path-to-playwright-core>
import { createRequire } from "node:module";
const [base, out, pw] = process.argv.slice(2);
const { chromium } = createRequire(import.meta.url)(pw);
const browser = await chromium.launch();
const ctx = await browser.newContext({
  viewport: { width: 1280, height: 800 }, colorScheme: "dark",
  recordVideo: { dir: out, size: { width: 1280, height: 800 } },
});
const page = await ctx.newPage();
const t0 = Date.now();
await page.goto(base + "/");
await page.getByPlaceholder("Username").fill("demo");
await page.getByRole("button", { name: "Continue", exact: true }).click();
await page.getByPlaceholder("Password").fill("demo");
await page.getByRole("button", { name: "Sign in to Axiom" }).click();
await page.getByTestId("sidebar-nav").waitFor();
await page.waitForTimeout(1200);
// hide the demo-instance setup nag so the clip shows the product, not the empty-config state
await page.evaluate(() => { for (const el of document.querySelectorAll("*")) { if (el.children.length < 4 && /Setup for this instance/.test(el.textContent || "") && el.textContent.length < 120) { let n = el; while (n.parentElement && n.parentElement.textContent.length < 120) n = n.parentElement; n.style.display = "none"; } } });
await page.waitForTimeout(4200);   // let the sign-in toast expire
const mark = Date.now();            // clip starts here
await page.waitForTimeout(1600);    // overview
await page.keyboard.press("Control+k");
await page.waitForTimeout(500);
await page.keyboard.type("checkout", { delay: 110 });
await page.waitForTimeout(1500);
await page.keyboard.press("Escape");
const nav = (n) => page.getByTestId("sidebar-nav").getByRole("link", { name: n }).first().click();
await nav("Workloads");  await page.waitForTimeout(1900);
await nav("Fleet");      await page.waitForTimeout(1900);
await nav("Timeline");   await page.waitForTimeout(1800);
await nav("Overview");   await page.waitForTimeout(1200);
console.log("CLIP_OFFSET_MS", mark - t0);
await ctx.close(); await browser.close();
