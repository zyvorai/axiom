// @ts-check
// Functional specs for the security views (RBAC viewer, NetworkPolicy analysis,
// /security reports) and the auth hardening that ships with them (stream
// tickets, deprecated URL credentials, audit JSON export).
//
// Runs against a *demo-mode* server whose RBAC graph, NetworkPolicies and
// scanner reports are fixed, deterministic fixtures (see
// internal/platform/{rbac_view_demo,netpol_analysis_demo,security_reports_demo}.go),
// so the assertions below are exact. Same contract as the other flow specs:
// drive the real UI, make `track` assert no mutating call failed, and confirm
// what the UI shows with an independent API read.
const http = require("http");
const { test, expect } = require("@playwright/test");
const { signIn, track, apiGet, apiSend, USERNAME, PASSWORD } = require("./helpers");

const uid = () => Math.random().toString(36).slice(2, 8);
const basic = (u, p) => ({ Authorization: `Basic ${Buffer.from(`${u}:${p}`).toString("base64")}` });

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

/** Creates a local account via the admin API and returns its credentials; the caller deletes it. */
async function makeUser(page, role, namespaces) {
  const username = `e2e-sec-${role}-${uid()}`;
  const password = `Passw0rd-${uid()}`;
  const data = { username, password, role };
  if (namespaces) data.namespaces = namespaces;
  const res = await apiSend(page, "POST", "/users", data);
  expect(res.status).toBe(201);
  return { username, password };
}

async function dropUser(page, username) {
  await apiSend(page, "DELETE", `/users/${username}`);
}

async function asUser(page, u, method, path, data) {
  const res = await page.request.fetch(`/api/v1${path}`, { method, headers: basic(u.username, u.password), data });
  return res.status();
}

test.describe("RBAC viewer", () => {
  test("by subject: list, filter, effective permissions, URL-addressable", async ({ page }) => {
    const t = track(page);
    await page.goto("/ops/access");
    // The long-standing roles & bindings management stays the default view.
    await expect(page.getByRole("button", { name: "New role" })).toBeVisible();

    await page.getByTestId("rbac-view-subjects").click();
    await expect(page).toHaveURL(/\/ops\/access\?view=subjects/);
    const masters = page.getByTestId("rbac-subject-row-Group-system:masters");
    await expect(masters).toContainText("cluster-admin");

    await page.getByTestId("rbac-subject-search").fill("payments-api");
    await expect(page.getByTestId("rbac-subject-row-ServiceAccount-payments-api")).toBeVisible();
    await expect(masters).toHaveCount(0);
    await page.getByRole("button", { name: "View permissions of ServiceAccount payments-api" }).click();
    const detail = page.getByTestId("rbac-subject-detail");
    await expect(detail).toContainText("payments/payments-api");
    const grant = detail.getByTestId("rbac-grant-RoleBinding/payments/payments-operator-binding");
    await expect(grant).toContainText("Role/payments-operator");
    await expect(grant).toContainText("pods/exec");
    await expect(detail).toContainText("No ClusterRoleBindings reach this subject.");

    // out-of-band: the API agrees with the UI
    const api = await apiGet(page, "/rbac/subjects/ServiceAccount/payments-api?namespace=payments");
    expect(api.status).toBe(200);
    expect(api.data.namespaced.map((g) => g.binding)).toEqual(["RoleBinding/payments/payments-operator-binding"]);

    // cluster-admin subjects are called out
    await page.getByTestId("rbac-subject-search").fill("system:masters");
    await page.getByRole("button", { name: "View permissions of Group system:masters" }).click();
    await expect(page.getByTestId("rbac-subject-detail")).toContainText("cluster-admin-equivalent");

    // the selected view survives a reload
    await page.reload();
    await expect(page.getByTestId("rbac-subjects")).toBeVisible();
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("who can: verb/resource/namespace query, named-object rules", async ({ page }) => {
    const t = track(page);
    await page.goto("/ops/access?view=who-can");
    await page.getByTestId("rbac-who-verb").selectOption("get");
    await page.getByTestId("rbac-who-resource").fill("secrets");
    await page.getByTestId("rbac-who-namespace").fill("payments");
    await page.getByTestId("rbac-who-run").click();
    // cluster-admin matches everywhere; alice's Role only covers the named secret `payments-db`
    await expect(page.getByTestId("rbac-who-row-Group-system:masters-cluster")).toBeVisible();
    await expect(page.getByTestId("rbac-who-row-User-alice@zyvor.dev-namespace")).toHaveCount(0);

    await page.getByTestId("rbac-who-name").fill("payments-db");
    await page.getByTestId("rbac-who-run").click();
    await expect(page.getByTestId("rbac-who-row-User-alice@zyvor.dev-namespace")).toBeVisible();
    await expect(page.getByTestId("rbac-who-row-ServiceAccount-payments-api-namespace")).toBeVisible();

    const api = await apiGet(page, "/rbac/who-can?verb=get&resource=secrets&namespace=payments&name=payments-db");
    const who = api.data.allowed.map((a) => `${a.subject.kind}/${a.subject.name}@${a.scope}`);
    expect(who).toContain("User/alice@zyvor.dev@namespace");
    expect(who).toContain("Group/system:masters@cluster");
    expect(who).not.toContain("Group/devs@cluster"); // view role has no secrets access
    expect(t.failures()).toEqual([]);
  });

  test("server-side enforcement: viewers and namespace-restricted accounts are denied, operators may read", async ({ page }) => {
    const viewer = await makeUser(page, "viewer");
    const oper = await makeUser(page, "operator");
    const restricted = await makeUser(page, "viewer", ["commerce"]);
    try {
      for (const p of ["/rbac/subjects", "/rbac/subjects/User/alice@zyvor.dev", "/rbac/who-can?verb=get&resource=pods"]) {
        expect(await asUser(page, viewer, "GET", p)).toBe(403);
        expect(await asUser(page, restricted, "GET", p)).toBe(403);
        expect(await asUser(page, oper, "GET", p)).toBe(200);
      }
      // operators still cannot write RBAC through the viewer's prefix
      expect(await asUser(page, oper, "POST", "/rbac/subjects", {})).toBe(403);
      // the ServiceAccount list is an ordinary namespaced read
      expect(await asUser(page, viewer, "GET", "/serviceaccounts?namespace=payments")).toBe(200);
      expect(await asUser(page, restricted, "GET", "/serviceaccounts?namespace=commerce")).toBe(200);
      expect(await asUser(page, restricted, "GET", "/serviceaccounts?namespace=payments")).toBe(403);
      expect(await asUser(page, restricted, "GET", "/serviceaccounts")).toBe(403);
      // NetworkPolicy analysis + security reports honour namespace restriction
      expect(await asUser(page, restricted, "GET", "/networking/policies/commerce/analysis")).toBe(200);
      expect(await asUser(page, restricted, "GET", "/networking/policies/payments/analysis")).toBe(403);
      expect(await asUser(page, restricted, "GET", "/security/reports")).toBe(403);
      expect(await asUser(page, restricted, "GET", "/security/reports?namespace=commerce")).toBe(200);
      expect(await asUser(page, restricted, "GET", "/security/reports?namespace=payments")).toBe(403);
      expect(await asUser(page, restricted, "GET", "/security/reports/payments/Deployment/payments-api")).toBe(403);
      expect(await asUser(page, viewer, "GET", "/security/reports")).toBe(200);
    } finally {
      await dropUser(page, viewer.username);
      await dropUser(page, oper.username);
      await dropUser(page, restricted.username);
    }
  });
});

test.describe("NetworkPolicy analysis", () => {
  test("per-pod isolation, allowed peers/ports and uncovered-pod warning", async ({ page }) => {
    const t = track(page);
    await page.goto("/networking");
    const card = page.getByTestId("netpol-analysis");
    await expect(card).toBeVisible();

    // commerce: allow-checkout isolates the checkout pods in both directions
    await page.getByTestId("netpol-analysis-namespace").selectOption("commerce");
    const row = page.getByTestId("netpol-pod-checkout-01");
    await expect(row).toContainText("isolated");
    await expect(row).toContainText("allow-checkout");
    await row.getByRole("button", { name: "Show allowed traffic for checkout-01" }).click();
    const detail = page.getByTestId("netpol-pod-detail-checkout-01");
    await expect(detail).toContainText("kubernetes.io/metadata.name=ingress-nginx");
    await expect(detail).toContainText("TCP/8080, TCP/8443");
    await expect(detail).toContainText("CIDR 10.42.0.0/16 except 10.42.9.0/24");
    await expect(detail).toContainText("UDP/53, TCP/53");

    // payments: deny-all-ingress -> default-deny ingress, no allowed ingress, egress still open
    await page.getByTestId("netpol-analysis-namespace").selectOption("payments");
    await expect(page.getByTestId("netpol-analysis-summary")).toContainText("default-deny ingress");
    await page.getByRole("button", { name: "Show allowed traffic for payments-api-01" }).click();
    await expect(page.getByTestId("netpol-pod-detail-payments-api-01")).toContainText("nothing is allowed");

    // data: no policy at all -> every pod is flagged as not covered
    await page.getByTestId("netpol-analysis-namespace").selectOption("data");
    await expect(page.getByTestId("netpol-analysis-uncovered")).toContainText("not covered by any policy");
    await expect(page.getByTestId("netpol-analysis-uncovered")).toContainText("redis-01");

    const api = await apiGet(page, "/networking/policies/data/analysis");
    expect(api.data.uncovered).toContain("redis-01");
    expect(api.data.pods.every((p) => !p.ingressIsolated && !p.egressIsolated)).toBe(true);
    expect((await apiGet(page, "/networking/policies/all/analysis")).status).toBe(400);
    expect(t.failures()).toEqual([]);
  });
});

test.describe("Security reports", () => {
  test("severity counts, workload table and drill-down", async ({ page }) => {
    const t = track(page);
    await page.goto("/security");
    await expect(page.getByTestId("security-page")).toBeVisible();
    await expect(page.getByTestId("security-count-critical")).toContainText("1");
    await expect(page.getByTestId("security-count-high")).toContainText("5");
    await expect(page.getByTestId("security-count-policy-fail")).toContainText("1");
    await expect(page.getByTestId("security-empty")).toHaveCount(0);

    // riskiest first: the Deployment is shown, not its ReplicaSet
    const first = page.getByTestId("security-page").locator("tbody tr").first();
    await expect(first).toContainText("payments-api");
    await expect(first).toContainText("Deployment");

    await page.getByRole("button", { name: "Show findings for Deployment payments-api" }).click();
    const dlg = page.getByTestId("security-detail");
    await expect(dlg).toContainText("ghcr.io/zyvorai/payments");
    await expect(dlg.getByTestId("security-vuln-CVE-2024-45337")).toContainText("CRITICAL");
    await expect(dlg.getByTestId("security-vuln-CVE-2024-45337")).toContainText("v0.31.0");
    await dlg.getByTestId("security-detail-tab-config").click();
    await expect(dlg).toContainText("KSV017");
    await expect(dlg).not.toContainText("KSV001"); // passing checks are not listed
    await dlg.getByTestId("security-detail-tab-policy").click();
    await expect(dlg).toContainText("require-requests-limits");
    await page.keyboard.press("Escape");
    await expect(dlg).toBeHidden();

    // namespace filter scopes everything
    await page.getByTestId("security-namespace").selectOption("data");
    await expect(page.getByTestId("security-workload-redis")).toBeVisible();
    await expect(page.getByTestId("security-workload-payments-api")).toHaveCount(0);
    await expect(page.getByTestId("security-count-critical")).toContainText("0");

    const api = await apiGet(page, "/security/reports/payments/Deployment/payments-api");
    expect(api.data.vulnerabilities[0].id).toBe("CVE-2024-45337");
    expect(api.data.vulnerabilities).toHaveLength(7);
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("empty state with install hint when no scanner CRDs exist", async ({ page }) => {
    await page.route("**/api/v1/security/reports*", (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          available: false,
          sources: [
            { name: "Trivy VulnerabilityReports", group: "aquasecurity.github.io", available: false, reports: 0, message: "CRD not installed" },
            { name: "PolicyReports", group: "wgpolicyk8s.io", available: false, reports: 0, message: "CRD not installed" },
          ],
          vulnerabilities: {}, misconfigurations: {}, policy: {}, workloads: [],
          installHint: "Axiom reads scanner output but does not run scanners. Install Trivy Operator (helm install trivy-operator aqua/trivy-operator).",
        }),
      })
    );
    await page.goto("/security");
    await expect(page.getByTestId("security-empty")).toBeVisible();
    await expect(page.getByTestId("security-install-hint")).toContainText("Install Trivy Operator");
    await expect(page.getByTestId("security-empty")).toContainText("aquasecurity.github.io");
    await expect(page.getByTestId("security-count-critical")).toHaveCount(0);
  });

  test("sidebar links to /security", async ({ page }) => {
    await page.getByTestId("nav-link-security").click();
    await expect(page).toHaveURL(/\/security$/);
    await expect(page.getByRole("heading", { name: "Security", exact: true })).toBeVisible();
  });
});

/** GET `path` with node's http (the SSE route never ends); resolves with the status and first body chunk. */
function streamProbe(baseURL, path) {
  return new Promise((resolve, reject) => {
    const req = http.get(new URL(path, baseURL), (res) => {
      if (res.statusCode !== 200) {
        res.resume();
        return resolve({ status: res.statusCode, first: "" });
      }
      res.once("data", (chunk) => {
        resolve({ status: res.statusCode, first: chunk.toString() });
        req.destroy();
      });
    });
    req.on("error", (e) => (e.code === "ECONNRESET" ? undefined : reject(e)));
    req.setTimeout(8000, () => {
      req.destroy();
      reject(new Error("timeout waiting for stream"));
    });
  });
}

test.describe("Auth hardening", () => {
  test("stream tickets: single-use, path-bound, no credentials needed in the URL", async ({ page, baseURL }) => {
    const mint = async (path) => apiSend(page, "POST", "/ws-ticket", { path });
    const a = await mint("/api/v1/events/stream");
    expect(a.status).toBe(200);
    expect(a.data.expiresIn).toBe(30);
    expect(a.data.ticket.length).toBeGreaterThanOrEqual(40);

    const ok = await streamProbe(/** @type {string} */ (baseURL), `/api/v1/events/stream?ticket=${a.data.ticket}`);
    expect(ok.status).toBe(200);
    expect(ok.first).toContain("connected");
    const reuse = await streamProbe(/** @type {string} */ (baseURL), `/api/v1/events/stream?ticket=${a.data.ticket}`);
    expect(reuse.status).toBe(401);

    const b = await mint("/api/v1/events/stream");
    const wrongPath = await streamProbe(/** @type {string} */ (baseURL), `/api/v1/pods/payments/payments-api-01/logs?ticket=${b.data.ticket}`);
    expect(wrongPath.status).toBe(401);
    // a ticket cannot be minted for an arbitrary API route
    expect((await mint("/api/v1/secrets/default/x")).status).toBe(400);
    expect((await mint("")).status).toBe(400);
  });

  test("a viewer can mint tickets but a ticket never widens their access", async ({ page, baseURL }) => {
    const viewer = await makeUser(page, "viewer");
    try {
      const mint = async (path) => {
        const r = await page.request.post("/api/v1/ws-ticket", { headers: basic(viewer.username, viewer.password), data: { path } });
        return { status: r.status(), data: await r.json().catch(() => null) };
      };
      const term = await mint("/api/v1/pods/payments/payments-api-01/terminal");
      expect(term.status).toBe(200);
      const denied = await streamProbe(/** @type {string} */ (baseURL), `/api/v1/pods/payments/payments-api-01/terminal?ticket=${term.data.ticket}`);
      expect(denied.status).toBe(403);
      const ev = await mint("/api/v1/events/stream");
      expect((await streamProbe(/** @type {string} */ (baseURL), `/api/v1/events/stream?ticket=${ev.data.ticket}`)).status).toBe(200);
    } finally {
      await dropUser(page, viewer.username);
    }
  });

  test("credentials in the URL still work but are flagged deprecated", async ({ page }) => {
    const token = Buffer.from(`${USERNAME}:${PASSWORD}`).toString("base64");
    const res = await page.request.get(`/api/v1/overview?access_token=${encodeURIComponent(token)}`);
    expect(res.status()).toBe(200);
    expect(res.headers()["deprecation"]).toBe("true");
    const clean = await page.request.get("/api/v1/overview", { headers: basic(USERNAME, PASSWORD) });
    expect(clean.headers()["deprecation"]).toBeUndefined();
  });

  test("audit log exports as JSON as well as CSV", async ({ page }) => {
    // Make sure there is at least one entry to export, whatever ran before.
    const u = await makeUser(page, "viewer");
    await dropUser(page, u.username);
    await page.goto("/ops/audit");
    await expect(page.getByRole("button", { name: "Export CSV" })).toBeEnabled();
    const [download] = await Promise.all([page.waitForEvent("download"), page.getByRole("button", { name: "Export JSON" }).click()]);
    expect(download.suggestedFilename()).toMatch(/^axiom-audit-\d{4}-\d{2}-\d{2}\.json$/);
    const chunks = [];
    for await (const c of await download.createReadStream()) chunks.push(c);
    const parsed = JSON.parse(Buffer.concat(chunks).toString("utf8"));
    expect(Array.isArray(parsed)).toBe(true);
    expect(parsed.length).toBeGreaterThan(0);
    expect(parsed[0]).toHaveProperty("action");
  });
});
