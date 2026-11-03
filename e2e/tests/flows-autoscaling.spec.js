// @ts-check
// Autoscaling beyond CPU (multi-metric HPAs, VPA and KEDA views) and the cost
// tab's budgets and rightsizing, against the shared demo server.
const { test, expect } = require("@playwright/test");
const { signIn, apiGet, apiSend, waitMutation, confirmDialog } = require("./helpers");

test.describe("autoscaling", () => {
  test("HPAs list every metric with current and target, and say what the autoscaler reports", async ({ page }) => {
    await signIn(page);
    await page.goto("/autoscaling");
    // CPU and memory together, as a person would read them.
    const checkout = page.getByTestId("hpa-metrics-commerce-checkout");
    await expect(checkout).toContainText("cpu");
    await expect(checkout).toContainText("memory");
    // Non-resource metrics are labelled by kind.
    const otel = page.getByTestId("hpa-metrics-observability-otel-collector");
    await expect(otel).toContainText("external");
    await expect(otel).toContainText("pods");
    await expect(page.getByTestId("hpa-state-commerce-checkout")).toBeVisible();

    // Out of band: the API carries the same metrics, conditions and behaviour flag.
    const list = await apiGet(page, "/hpas");
    const h = list.data.find((x) => x.name === "checkout");
    expect(h.metrics.map((m) => m.name)).toEqual(expect.arrayContaining(["cpu", "memory"]));
    expect(h.conditions.length).toBeGreaterThan(0);
    expect(h.behavior).toBe(true);
  });

  test("create an HPA scaling on CPU and memory with a scale-down window", async ({ page }) => {
    await signIn(page);
    await page.goto("/autoscaling");
    await page.getByRole("button", { name: "Create autoscaler" }).click();
    await page.getByLabel("Target workload").click();
    await page.getByRole("option", { name: /redis/ }).first().click();
    await page.getByTestId("hpa-create-memory").fill("70");
    await page.getByTestId("hpa-create-stabilization").fill("120");
    const created = waitMutation(page, "POST", /\/hpas$/);
    await page.getByRole("button", { name: "Create", exact: true }).click();
    const res = await created;
    expect(res.status()).toBe(201);
    const posted = res.request().postDataJSON();
    expect(posted).toMatchObject({ targetMemoryPercent: 70, scaleDownStabilizationSeconds: 120 });
    try {
      const mine = (await apiGet(page, "/hpas")).data.find((x) => x.name === "redis-hpa");
      expect(mine.metrics.map((m) => m.name).sort()).toEqual(["cpu", "memory"]);
      await expect(page.getByTestId("hpa-metrics-data-redis-hpa")).toContainText("memory");
    } finally {
      await apiSend(page, "DELETE", "/hpas/data/redis-hpa");
    }
  });

  test("VPA and KEDA tabs show recommendations and triggers, never trigger settings", async ({ page }) => {
    await signIn(page);
    await page.goto("/autoscaling");

    await page.getByTestId("autoscale-tab-vpa").click();
    await expect(page.getByTestId("vpa-installed")).toBeVisible();
    await expect(page.getByTestId("vpa-row-commerce-checkout-vpa")).toContainText("Auto");
    await expect(page.getByTestId("vpa-rec-commerce-checkout-vpa")).toContainText("1200m");
    await expect(page.getByTestId("vpa-row-data-redis-vpa")).toContainText("Off");

    await page.getByTestId("autoscale-tab-keda").click();
    await expect(page.getByTestId("keda-row-payments-payments-api")).toContainText("Active");
    await expect(page.getByTestId("keda-triggers-payments-payments-api")).toContainText("kafka");
    await expect(page.getByTestId("keda-row-data-redis-consumer")).toContainText("Idle");

    // Out of band: only the trigger type and name are exposed.
    const raw = JSON.stringify((await apiGet(page, "/scaledobjects")).data);
    for (const leak of ["authenticationRef", "password", "serverAddress", "metadata"]) {
      expect(raw, leak).not.toContain(leak);
    }
  });

  test("namespace-restricted accounts only see their own namespace's autoscalers", async ({ page }) => {
    const name = `as-scoped-${Math.random().toString(36).slice(2, 7)}`;
    const created = await apiSend(page, "POST", "/users", { username: name, password: "pw-pw-pw-pw", role: "viewer", namespaces: ["payments"] });
    expect(created.status).toBe(201);
    try {
      const auth = { Authorization: `Basic ${Buffer.from(`${name}:pw-pw-pw-pw`).toString("base64")}` };
      const own = await page.request.get("/api/v1/scaledobjects?namespace=payments", { headers: auth });
      expect(own.status()).toBe(200);
      expect((await own.json()).items.every((s) => s.namespace === "payments")).toBe(true);
      expect((await page.request.get("/api/v1/scaledobjects?namespace=data", { headers: auth })).status()).toBe(403);
      expect((await page.request.get("/api/v1/vpas", { headers: auth })).status()).toBe(403);
      expect((await page.request.get("/api/v1/cost/budgets", { headers: auth })).status()).toBe(403);
      expect((await page.request.get("/api/v1/cost/rightsizing", { headers: auth })).status()).toBe(403);
    } finally {
      await apiSend(page, "DELETE", `/users/${name}`);
    }
  });
});

test.describe("cost budgets and rightsizing", () => {
  test("set, see over-budget, edit and remove a namespace budget", async ({ page }) => {
    await signIn(page);
    await page.goto("/ops/cost");
    await expect(page.getByTestId("cost-budgets")).toBeVisible();
    try {
      // Unbudgeted namespaces are offered as one-click starting points.
      await page.getByTestId("cost-add-budget").click();
      const dlg = page.getByTestId("cost-budget-dialog");
      await dlg.getByTestId("cost-budget-namespace").fill("payments");
      await dlg.getByTestId("cost-budget-amount").fill("1");
      const saved = page.waitForResponse((r) => r.request().method() === "PUT" && /\/cost\/budgets\/payments$/.test(r.url()));
      await dlg.getByTestId("cost-budget-save").click();
      expect((await saved).status()).toBe(200);

      // A $1 budget is blown by any real namespace.
      await expect(page.getByTestId("cost-budget-state-payments")).toContainText("Over budget");
      await expect(page.getByTestId("cost-budget-payments")).toContainText("of $1");
      const api = (await apiGet(page, "/cost/budgets")).data;
      expect(api.budgets.find((b) => b.namespace === "payments")).toMatchObject({ monthlyUsd: 1, state: "over" });

      // Raise it far above the cost: back to on track.
      await page.getByTestId("cost-budget-edit-payments").click();
      await dlg.getByTestId("cost-budget-amount").fill("100000");
      await dlg.getByTestId("cost-budget-save").click();
      await expect(page.getByTestId("cost-budget-state-payments")).toContainText("On track");

      // Remove it.
      await page.getByTestId("cost-budget-remove-payments").click();
      const removed = page.waitForResponse((r) => r.request().method() === "DELETE" && /\/cost\/budgets\/payments$/.test(r.url()));
      await confirmDialog(page, "Remove");
      expect((await removed).status()).toBe(200);
      await expect(page.getByTestId("cost-budget-payments")).toHaveCount(0);
    } finally {
      await apiSend(page, "DELETE", "/cost/budgets/payments");
    }
  });

  test("invalid budgets are refused and operators cannot set them", async ({ page }) => {
    await signIn(page);
    expect((await apiSend(page, "PUT", "/cost/budgets/payments", { monthlyUsd: -5 })).status).toBe(400);
    expect((await apiSend(page, "PUT", "/cost/budgets/Not_A_Namespace", { monthlyUsd: 10 })).status).toBe(400);
    const name = `as-oper-${Math.random().toString(36).slice(2, 7)}`;
    expect((await apiSend(page, "POST", "/users", { username: name, password: "pw-pw-pw-pw", role: "operator" })).status).toBe(201);
    try {
      const auth = { Authorization: `Basic ${Buffer.from(`${name}:pw-pw-pw-pw`).toString("base64")}` };
      expect((await page.request.put("/api/v1/cost/budgets/payments", { headers: auth, data: { monthlyUsd: 10 } })).status()).toBe(403);
      expect((await page.request.get("/api/v1/cost/budgets", { headers: auth })).status()).toBe(200);
    } finally {
      await apiSend(page, "DELETE", `/users/${name}`);
    }
  });

  test("rightsizing says where its data comes from and flags over- and under-provisioned containers", async ({ page }) => {
    await signIn(page);
    await page.goto("/ops/cost");
    const rs = page.getByTestId("cost-rightsizing");
    await expect(rs).toBeVisible();
    // The source and confidence are stated, not implied.
    await expect(rs.getByTestId("cost-rightsizing-source")).toContainText(/Source: metrics-snapshot/);
    await expect(rs.getByTestId("cost-rightsizing-source")).toContainText(/Low confidence/);

    // Default view: only what needs attention (the right-sized checkout is hidden).
    await expect(rs.getByTestId("cost-rs-verdict-payments-payments-api-api")).toContainText("Over-provisioned");
    await expect(rs.getByTestId("cost-rs-verdict-payments-payments-api-envoy")).toContainText("Under-provisioned");
    await expect(rs.getByTestId("cost-rs-commerce-checkout-checkout")).toHaveCount(0);
    await expect(rs.getByTestId("cost-rightsizing-summary-savings")).not.toContainText("$0");

    // Showing everything includes it.
    await rs.getByLabel("Show").selectOption("all");
    await expect(rs.getByTestId("cost-rs-verdict-commerce-checkout-checkout")).toContainText("Right-sized");

    // Out of band: the API agrees, and a bad window is rejected.
    const report = (await apiGet(page, "/cost/rightsizing?window=7d")).data;
    expect(report.source).toBe("metrics-snapshot");
    expect(report.items.find((i) => i.workload === "payments-api" && i.container === "api").verdict).toBe("over");
    expect(report.summary.overProvisioned).toBeGreaterThan(0);
    expect((await apiGet(page, "/cost/rightsizing?window=2d")).status).toBe(400);
  });
});
