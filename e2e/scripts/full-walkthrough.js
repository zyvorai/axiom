// Ad-hoc manual verification script (not a Playwright test — no assertions,
// just a console-error-collecting tour). Logs in against a running demo
// instance, visits every top-level route, opens a pod detail page, and
// opens the command palette, printing a summary plus any console/page
// errors seen along the way. Run against a local demo server:
//
//   AXIOM_MODE=demo AXIOM_USERNAME=demo AXIOM_PASSWORD=demo AXIOM_ADDR=:5090 ./bin/zyvor-axiom &
//   node e2e/scripts/full-walkthrough.js
//
// Requires playwright-core + a Chromium install (already present via the
// e2e/ Playwright devDependency).
const { chromium } = require('playwright-core');

const BASE = process.env.AXIOM_E2E_BASE_URL || 'http://localhost:5090';
const errors = [];
const results = [];

const ROUTES = [
  'overview', 'getting-started', 'nodes', 'workloads', 'jobs', 'pods',
  'namespaces', 'platform-intents', 'networking', 'services', 'ingresses',
  'vms', 'storage', 'atlas', 'backups', 'configmaps', 'secrets', 'quotas',
  'autoscaling', 'custom-resources', 'logs', 'events', 'alerts', 'ops',
  'users', 'settings',
];

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
  page.on('pageerror', (err) => errors.push(`[pageerror] ${err.message}`));
  page.on('console', (msg) => {
    if (msg.type() === 'error') errors.push(`[console.error] ${msg.text()}`);
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
  results.push('login: ok');

  for (const route of ROUTES) {
    const before = errors.length;
    try {
      await page.goto(`${BASE}/${route}`, { waitUntil: 'networkidle', timeout: 15000 });
      await page.waitForTimeout(600);
      const newErrors = errors.slice(before);
      results.push(`${route}: ok${newErrors.length ? ` (${newErrors.length} console error(s))` : ''}`);
    } catch (e) {
      results.push(`${route}: FAILED — ${e.message}`);
    }
  }

  // A couple of interactive checks
  await page.goto(`${BASE}/pods`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(600);
  const podLink = page.locator('table.tbl tbody tr a').first();
  if (await podLink.count()) {
    await podLink.click();
    await page.waitForTimeout(800);
    const hasTabs = await page.locator('[role="tab"]').count();
    results.push(`pod detail navigation: ok (${hasTabs} tabs found)`);
  } else {
    results.push('pod detail navigation: no pod link found');
  }

  await page.goto(`${BASE}/overview`, { waitUntil: 'networkidle' });
  await page.keyboard.press('Meta+k');
  await page.waitForTimeout(500);
  const paletteVisible = await page.getByTestId('command-palette').isVisible().catch(() => false);
  results.push(`command palette (cmd+k): ${paletteVisible ? 'ok' : 'FAILED'}`);

  console.log('\n=== FULL WALKTHROUGH RESULTS ===');
  results.forEach((r) => console.log(' -', r));
  console.log('\n=== CONSOLE/PAGE ERRORS (' + errors.length + ') ===');
  errors.slice(0, 40).forEach((e) => console.log(' !', e));

  await browser.close();
})();
