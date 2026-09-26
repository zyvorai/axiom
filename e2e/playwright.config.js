// @ts-check
const { defineConfig, devices } = require("@playwright/test");

// Runs against an already-running Axiom instance (demo mode by default) —
// see e2e/README.md for how to start one locally, and
// .github/workflows/ci.yml's `e2e` job for how CI starts one.
module.exports = defineConfig({
  testDir: "./tests",
  // Serial, not parallel: Axiom's login rate limiter (internal/auth/ratelimit.go)
  // counts every unauthenticated request — including the app's own
  // pre-login session probe — as a failed attempt per client IP. Several
  // browser contexts hitting the same server concurrently trip it.
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL: process.env.AXIOM_E2E_BASE_URL || "http://localhost:5090",
    ignoreHTTPSErrors: true, // self-signed lab/dev certs
    trace: "retain-on-failure",
  },
  // Visual regression is opt-in (VISUAL=1): screenshot baselines are per-OS, see tests/visual.spec.js.
  expect: { toHaveScreenshot: { maxDiffPixelRatio: 0.01, animations: "disabled", caret: "hide" } },
  projects: process.env.VISUAL
    ? [{ name: "visual", testMatch: /visual\.spec\.js/, use: { ...devices["Desktop Chrome"] } }]
    : [{ name: "chromium", testIgnore: /visual\.spec\.js/, use: { ...devices["Desktop Chrome"] } }],
});
