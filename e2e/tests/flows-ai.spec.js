// @ts-check
// Functional specs for the opt-in AI assistant ("Explain with AI").
//
// Demo mode returns deterministic canned answers and never contacts a model, so
// these specs prove the UI -> API contract, the privacy notice, the redacted
// payload preview, the audit trail and that the feature is invisible while
// disabled. The provider HTTP client, redactor and namespace scoping are covered
// by Go tests (internal/platform/ai*_test.go, internal/auth/policy_ai_test.go).
//
// Same contract as flows-core.spec.js: `track` records every mutating call.
const { test, expect } = require("@playwright/test");
const { signIn, track, apiGet, apiSend, waitMutation } = require("./helpers");

test.describe.configure({ mode: "serial" });

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

test.afterAll(async ({ browser }) => {
  // Leave the shared demo server with AI off, as every other spec expects.
  const page = await browser.newPage();
  await signIn(page);
  await apiSend(page, "PUT", "/integrations", { aiEnabled: false });
  await page.close();
});

async function setAi(page, enabled) {
  const res = await apiSend(page, "PUT", "/integrations", { aiEnabled: enabled, aiModel: "demo-model", aiProvider: "openai-compatible" });
  expect(res.status).toBe(200);
}

async function crashLoopPod(page) {
  const pods = (await apiGet(page, "/pods")).data || [];
  const p = pods.find((x) => x.status === "CrashLoopBackOff");
  expect(p, "demo data should contain a crash-looping pod").toBeTruthy();
  return p;
}

test("AI is invisible while disabled: no buttons and the API refuses with 409", async ({ page }) => {
  const t = track(page);
  await setAi(page, false);
  const pod = await crashLoopPod(page);

  await page.goto(`/pods/${pod.namespace}/${pod.name}`);
  await expect(page.getByRole("heading", { name: pod.name })).toBeVisible();
  await expect(page.getByText(/CrashLoopBackOff/).first()).toBeVisible(); // diagnosis banner is there...
  await expect(page.getByTestId("ai-explain-pod")).toHaveCount(0); // ...but no AI entry point

  await page.goto("/events");
  await expect(page.getByTestId("events-page")).toBeVisible();
  await expect(page.locator('[data-testid^="ai-explain-event-"]')).toHaveCount(0);

  const status = await apiGet(page, "/ai/status");
  expect(status.data.enabled).toBe(false);
  t.allow(/POST \/api\/v1\/ai\/explain/);
  const refused = await apiSend(page, "POST", `/ai/explain?namespace=${pod.namespace}`, { kind: "pod", name: pod.name });
  expect(refused.status).toBe(409);
  expect(refused.data.error).toMatch(/disabled/i);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("Settings: configure the AI assistant (demo), key is write-only", async ({ page }) => {
  const t = track(page);
  await setAi(page, false);
  await page.goto("/settings");
  const card = page.getByTestId("ai-settings-card");
  await expect(card).toBeVisible();

  await page.getByTestId("ai-settings-enabled").click();
  await page.getByTestId("ai-settings-provider").selectOption("ollama");
  await page.getByTestId("ai-settings-base-url").fill("http://localhost:11434");
  await page.getByTestId("ai-settings-model").fill("llama3.1");
  await page.getByTestId("ai-settings-api-key").fill("sk-e2e-write-only-key-123456");
  const saved = waitMutation(page, "PUT", /\/integrations$/);
  await page.getByTestId("ai-settings-save").click();
  const res = await saved;
  expect(res.status()).toBe(200);
  expect(await res.text()).not.toContain("sk-e2e-write-only-key-123456");
  await expect(page.getByTestId("ai-settings-api-key")).toHaveValue("");
  await expect(card.getByText("API key (set — leave blank to keep)")).toBeVisible();

  const view = (await apiGet(page, "/integrations")).data;
  expect(view).toMatchObject({ aiEnabled: true, aiProvider: "ollama", aiModel: "llama3.1", aiApiKeySet: true, aiRedact: true });
  expect(JSON.stringify(view)).not.toContain("sk-e2e-write-only-key-123456");
  expect((await apiGet(page, "/ai/status")).data).toMatchObject({ enabled: true, demo: true });

  const tested = waitMutation(page, "POST", /\/integrations\/ai\/test$/);
  await page.getByTestId("ai-settings-test").click();
  expect((await tested).status()).toBe(200);

  // Invalid base URL schemes are refused by the server.
  t.allow(/PUT \/api\/v1\/integrations/);
  const bad = await apiSend(page, "PUT", "/integrations", { aiBaseUrl: "file:///etc/passwd" });
  expect(bad.status).toBe(400);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("Pod detail: Explain with AI shows the notice, the redacted preview, then the canned answer", async ({ page }) => {
  const t = track(page);
  await setAi(page, true);
  const pod = await crashLoopPod(page);

  await page.goto(`/pods/${pod.namespace}/${pod.name}`);
  await expect(page.getByRole("heading", { name: pod.name })).toBeVisible();
  const open = page.getByTestId("ai-explain-pod");
  await expect(open).toBeVisible();

  const previewReq = waitMutation(page, "POST", /\/ai\/explain$/);
  await open.click();
  const drawer = page.getByTestId("ai-drawer");
  await expect(drawer).toBeVisible();
  expect((await previewReq).status()).toBe(200);

  // Clear notice of where the data goes, and the exact payload before anything is sent.
  await expect(page.getByTestId("ai-send-notice")).toContainText("This data will be sent to");
  await expect(page.getByTestId("ai-send-notice")).toContainText("demo://canned-responses");
  const preview = page.getByTestId("ai-payload-preview");
  await expect(preview).toContainText("<cluster-data>");
  await expect(preview).toContainText(pod.name);
  await expect(preview).toContainText("Pod status");
  expect(await preview.textContent()).not.toContain(pod.ip); // pod IP is never part of the payload

  // Preview alone must not have been audited and must not have returned an answer.
  const audit0 = ((await apiGet(page, "/audit")).data || []).filter((e) => e.action === "ai.explain" && e.target.endsWith(`/pod/${pod.name}`));
  expect(audit0).toHaveLength(0);
  await expect(page.getByTestId("ai-answer")).toHaveCount(0);

  const sent = waitMutation(page, "POST", /\/ai\/explain$/);
  await page.getByTestId("ai-send").click();
  expect((await sent).status()).toBe(200);
  const answer = page.getByTestId("ai-answer");
  await expect(answer).toContainText("Demo assistant");
  await expect(answer).toContainText("kubectl logs");

  // Deterministic, plain text (no markup interpreted).
  expect(await answer.locator("*").count()).toBe(0);

  // Audited by target only: no prompt or answer text.
  const audit = ((await apiGet(page, "/audit")).data || []).filter((e) => e.action === "ai.explain" && e.target === `${pod.namespace}/pod/${pod.name}`);
  expect(audit.length).toBeGreaterThan(0);
  expect(audit[0].detail || "").toBe("");

  const posts = t.find("POST", /\/ai\/explain$/);
  expect(posts.length).toBeGreaterThanOrEqual(2);
  expect(posts[0].body).toMatchObject({ kind: "pod", name: pod.name, preview: true });
  expect(posts[0].body).not.toHaveProperty("namespace"); // namespace travels in the query only
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("Events: Explain with AI on a Warning event opens the drawer with the same notice", async ({ page }) => {
  const t = track(page);
  await setAi(page, true);
  await page.goto("/events");
  await expect(page.getByTestId("events-page")).toBeVisible();
  const buttons = page.locator('[data-testid^="ai-explain-event-"]');
  await expect(buttons.first()).toBeVisible();
  await buttons.first().click();
  await expect(page.getByTestId("ai-drawer")).toBeVisible();
  await expect(page.getByTestId("ai-send-notice")).toContainText("This data will be sent to");
  await expect(page.getByTestId("ai-payload-preview")).toContainText("Events");
  await page.getByTestId("ai-send").click();
  await expect(page.getByTestId("ai-answer")).toContainText("Demo assistant");
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("ai-drawer")).toBeHidden();
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("Health check: Explain with AI on a finding opens the drawer; hidden when disabled", async ({ page }) => {
  const t = track(page);
  await setAi(page, false);
  await page.goto("/health-check");
  await expect(page.getByTestId("health-check-page")).toBeVisible();
  await expect(page.getByTestId("health-finding-0")).toBeVisible();
  await expect(page.getByTestId("ai-explain-finding")).toHaveCount(0);

  await setAi(page, true);
  await page.goto("/health-check");
  await expect(page.getByTestId("health-check-page")).toBeVisible();
  const buttons = page.getByTestId("ai-explain-finding");
  await expect(buttons.first()).toBeVisible();
  await buttons.first().click();
  await expect(page.getByTestId("ai-drawer")).toBeVisible();
  await expect(page.getByTestId("ai-send-notice")).toContainText("This data will be sent to");
  await page.getByTestId("ai-send").click();
  await expect(page.getByTestId("ai-answer")).toContainText("Demo assistant");
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("ai-drawer")).toBeHidden();
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});
