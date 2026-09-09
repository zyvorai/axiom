// Drive a real browser through Axiom's "Sign in with SSO" link end-to-end
// against a live OIDC provider (Keycloak), confirming the redirect, the IdP
// login form, and that Axiom mints a real session afterward.
//
// Usage:
//   AXIOM_E2E_BASE_URL=https://host:30590 \
//   OIDC_USERNAME=<idp-username> OIDC_PASSWORD=<idp-password> \
//   node scripts/verify-sso.js
//
// Requires an Axiom deployment with OIDC already configured (Settings →
// Integrations, or AXIOM_OIDC_ISSUER/AXIOM_OIDC_CLIENT_ID) and a user that
// exists in the IdP. Run from inside e2e/ so playwright-core resolves to
// this project's own node_modules, not a stray copy elsewhere on PATH.
const { chromium } = require('playwright-core');

const BASE = process.env.AXIOM_E2E_BASE_URL || 'https://175.110.122.71:30590';
const USERNAME = process.env.OIDC_USERNAME || 'demo';
const PASSWORD = process.env.OIDC_PASSWORD || 'demo';
const LOCAL_USERNAME = process.env.AXIOM_TWO_STEP_USERNAME || 'nobody';

(async () => {
  const browser = await chromium.launch();
  const page = await browser.newPage({ viewport: { width: 1280, height: 900 }, ignoreHTTPSErrors: true });
  page.on('response', (res) => { if (res.status() >= 400) console.log('HTTP', res.status(), res.url()); });

  await page.goto(`${BASE}/login`, { waitUntil: 'networkidle' });
  await page.waitForTimeout(1000);

  // The SSO link only renders on the password step of Axiom's two-step
  // local-account form, even when only SSO is intended to be used.
  await page.fill('form[aria-label="Account"] input[placeholder="Username"]', LOCAL_USERNAME);
  await page.click('form[aria-label="Account"] button[type="submit"]');
  await page.waitForTimeout(500);

  const ssoLink = page.getByText(/Sign in with SSO/i);
  await ssoLink.click();
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(800);
  console.log('Landed on IdP:', page.url());

  await page.fill('input[name="username"]', USERNAME);
  await page.fill('input[name="password"]', PASSWORD);
  await page.click('input[type="submit"], button[type="submit"]');
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(1000);
  console.log('After IdP login, landed on:', page.url());

  // Keycloak may ask new users to complete their profile before issuing a code.
  if (page.url().includes('VERIFY_PROFILE')) {
    await page.fill('#firstName', 'Axiom').catch(() => {});
    await page.fill('#lastName', 'SSOTest').catch(() => {});
    await page.click('input[type="submit"], button[type="submit"]');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(1000);
    console.log('After profile step, landed on:', page.url());
  }

  const sessionResp = await page.evaluate(async () => {
    const r = await fetch('/api/v1/session', { credentials: 'include' });
    return { status: r.status, body: await r.text() };
  });
  console.log('--- /api/v1/session after SSO login ---');
  console.log(sessionResp);

  await browser.close();
})();
