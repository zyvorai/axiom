// Captures the marketing screenshots from a running DEMO server.
// Usage: node scripts/site-shots.mjs <baseUrl> <outDir> <path-to-playwright-module>
//   AXIOM_MODE=demo AXIOM_USERNAME=demo AXIOM_PASSWORD=demo AXIOM_ADDR=127.0.0.1:5091 ./zyvor-axiom
//   node scripts/site-shots.mjs http://127.0.0.1:5091 site/shots ./e2e/node_modules/@playwright/test
import { createRequire } from "node:module";
const [base, out, pw] = process.argv.slice(2);
const { chromium } = createRequire(import.meta.url)(pw);
const SHOTS = [
  // name, path, wait-for selector/text (optional), clip height from the top of the content area
  ["overview", "/overview", 1000],
  ["fleet", "/fleet", 760],
  ["gitops", "/ops/gitops", 560, 190],
  ["autoscaling", "/autoscaling", 520, 190],
  ["cost", "/ops/cost", 680, 190],
  ["helm", "/ops/helm", 400, 190],
  ["security", "/security", 700],
  ["apps", "/workloads", 700],
  ["timeline", "/timeline", 700],
];
const browser = await chromium.launch();
const page = await (await browser.newContext({ viewport: { width: 1600, height: 1000 }, colorScheme: "dark", deviceScaleFactor: 1 })).newPage();
await page.goto(base + "/");
await page.getByPlaceholder("Username").fill("demo");
await page.getByRole("button", { name: "Continue", exact: true }).click();
await page.getByPlaceholder("Password").fill("demo");
await page.getByRole("button", { name: "Sign in to Axiom" }).click();
await page.getByTestId("sidebar-nav").waitFor();
for (const [name, path, h, dy = 0] of SHOTS) {
  await page.goto(base + path);
  await page.waitForTimeout(2600);
  // Hide the first-run "finish setup" banner: marketing shots should show a configured instance.
  await page.evaluate(() => {
    for (const el of document.querySelectorAll("div,section")) {
      if (el.textContent && el.textContent.trim().startsWith("Setup for this instance") && el.textContent.length < 160) { el.style.display = "none"; break; }
    }
  });
  if (name === "overview") await page.screenshot({ path: `${out}/${name}.png` });
  else await page.screenshot({ path: `${out}/${name}.png`, clip: { x: 248, y: 48 + dy, width: 1352, height: h } });
  console.log("captured", name);
}
await browser.close();
