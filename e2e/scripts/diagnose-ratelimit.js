// One-off diagnostic: logs every request/response pair (URL, status, whether
// an Authorization header was sent) during the same login-then-rapid-
// navigation pattern that produced 401/429 console errors in
// full-walkthrough.js, to pin down exactly which endpoint(s) are involved.
const { chromium } = require('playwright-core');

const BASE = process.env.AXIOM_E2E_BASE_URL || 'http://localhost:5090';

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });

  page.on('response', async (res) => {
    const status = res.status();
    if (status === 401 || status === 429) {
      const req = res.request();
      const headers = await req.allHeaders();
      console.log(`${status} ${req.method()} ${req.url()} auth=${headers['authorization'] ? 'present' : 'MISSING'}`);
    }
  });

  await page.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(500);
  await page.evaluate(() => document.getElementById('login-sign-in')?.scrollIntoView());
  await page.fill('form[aria-label="Account"] input[placeholder="Username"]', 'demo');
  await page.click('form[aria-label="Account"] button[type="submit"]');
  await page.waitForTimeout(400);
  await page.fill('form[aria-label="Password"] input[placeholder="Password"]', 'demo');
  await page.click('form[aria-label="Password"] button[type="submit"]');
  await page.waitForURL(/overview/, { timeout: 10000 });
  console.log('--- logged in, now navigating rapidly ---');

  const ROUTES = ['workloads', 'atlas', 'backups', 'configmaps', 'nodes', 'pods', 'storage'];
  for (const route of ROUTES) {
    await page.goto(`${BASE}/${route}`, { waitUntil: 'networkidle', timeout: 15000 });
    await page.waitForTimeout(200);
  }

  console.log('--- done ---');
  await browser.close();
})();
