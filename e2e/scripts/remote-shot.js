// Quick visual confirmation of a live remote deployment after `deploy-remote.sh`.
// Usage: AXIOM_E2E_BASE_URL=https://host:port node e2e/scripts/remote-shot.js
const { chromium } = require('playwright-core');

const BASE = process.env.AXIOM_E2E_BASE_URL || 'https://175.110.122.71:30590';
const OUT = process.env.OUT_DIR || '.';

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, ignoreHTTPSErrors: true });

  await page.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(600);
  await page.evaluate(() => document.getElementById('login-sign-in')?.scrollIntoView());
  await page.fill('form[aria-label="Account"] input[placeholder="Username"]', 'demo');
  await page.click('form[aria-label="Account"] button[type="submit"]');
  await page.waitForTimeout(400);
  await page.fill('form[aria-label="Password"] input[placeholder="Password"]', 'demo');
  await page.click('form[aria-label="Password"] button[type="submit"]');
  await page.waitForURL(/overview/, { timeout: 15000 });
  await page.waitForTimeout(1200);
  await page.screenshot({ path: `${OUT}/remote-overview.png` });

  await page.goto(`${BASE}/nodes`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(1000);
  await page.screenshot({ path: `${OUT}/remote-nodes.png` });

  await browser.close();
})();
