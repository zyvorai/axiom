// @ts-check
// Helm chart catalog and GitOps application creation, driven through the UI.
//
// Needs its OWN demo servers (the catalog fetches from a local mock chart
// repository, which Axiom refuses unless AXIOM_HELM_ALLOW_PRIVATE_REPOS /
// AXIOM_HELM_ALLOW_INSECURE_REPOS are set), started from ../../bin/zyvor-axiom
// like flows-integrations.spec.js.
const { test, expect } = require("@playwright/test");
const http = require("http");
const net = require("net");
const fs = require("fs");
const os = require("os");
const path = require("path");
const zlib = require("zlib");
const { spawn } = require("child_process");
const { USERNAME, PASSWORD } = require("./helpers");

const BIN_SRC = path.resolve(__dirname, "../../bin/zyvor-axiom");
/** @type {string} */
let BIN = "";
const running = new Set();
const basic = (u, p) => ({ Authorization: `Basic ${Buffer.from(`${u}:${p}`).toString("base64")}` });
const ADMIN = basic(USERNAME, PASSWORD);

test.beforeAll(() => {
  if (!fs.existsSync(BIN_SRC)) return;
  BIN = path.join(os.tmpdir(), `axiom-e2e-gitops-${process.pid}`);
  fs.copyFileSync(BIN_SRC, BIN);
  fs.chmodSync(BIN, 0o755);
});
test.afterAll(() => {
  for (const stop of running) stop();
  if (BIN) try { fs.unlinkSync(BIN); } catch { /* already gone */ }
});
test.beforeEach(() => {
  test.skip(!BIN, `binary not found at ${BIN_SRC} - run: go build -o bin/zyvor-axiom ./cmd/zyvor-axiom`);
});

function freePort() {
  return new Promise((resolve) => {
    const s = net.createServer();
    s.listen(0, "127.0.0.1", () => { const p = /** @type {any} */ (s.address()).port; s.close(() => resolve(p)); });
  });
}

async function startAxiom(env = {}) {
  const port = await freePort();
  const child = spawn(BIN, [], {
    env: { PATH: process.env.PATH, HOME: process.env.HOME, AXIOM_MODE: "demo", AXIOM_USERNAME: USERNAME, AXIOM_PASSWORD: PASSWORD, AXIOM_ADDR: `127.0.0.1:${port}`, ...env },
    stdio: "ignore",
  });
  const url = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 80; i++) {
    try { if ((await fetch(`${url}/api/v1/health`)).ok) break; } catch { /* not listening yet */ }
    await new Promise((r) => setTimeout(r, 150));
  }
  const stop = () => { running.delete(stop); child.kill("SIGTERM"); };
  running.add(stop);
  return { url, stop };
}

async function withApp(browser, env, fn) {
  const srv = await startAxiom(env);
  const ctx = await browser.newContext({ baseURL: srv.url, ignoreHTTPSErrors: true });
  const page = await ctx.newPage();
  try { await fn({ page, srv }); } finally { await ctx.close(); srv.stop(); }
}

async function signInAs(page, user, pass) {
  await page.goto("/");
  await page.getByPlaceholder("Username").fill(user);
  await page.getByRole("button", { name: "Continue", exact: true }).click();
  await page.getByPlaceholder("Password").fill(pass);
  await page.getByRole("button", { name: "Sign in to Axiom" }).click();
  await expect(page.getByTestId("sidebar-nav")).toBeVisible();
}

// ---------------------------------------------------------------------------
// A mock chart repository: index.yaml + .tgz archives built in memory.
// ---------------------------------------------------------------------------
function tarEntry(name, content) {
  const body = Buffer.from(content);
  const header = Buffer.alloc(512);
  header.write(name, 0, 100);
  header.write("0000644\0", 100);
  header.write("0000000\0", 108);
  header.write("0000000\0", 116);
  header.write(body.length.toString(8).padStart(11, "0") + "\0", 124);
  header.write(Math.floor(Date.now() / 1000).toString(8).padStart(11, "0") + "\0", 136);
  header.write("        ", 148);
  header.write("0", 156);
  header.write("ustar\0", 257);
  header.write("00", 263);
  let sum = 0;
  for (const b of header) sum += b;
  header.write(sum.toString(8).padStart(6, "0") + "\0 ", 148);
  const pad = Buffer.alloc((512 - (body.length % 512)) % 512);
  return Buffer.concat([header, body, pad]);
}
function chartArchive(name, version, valuesYaml, readme) {
  const tar = Buffer.concat([
    tarEntry(`${name}/Chart.yaml`, `apiVersion: v2\nname: ${name}\nversion: ${version}\n`),
    tarEntry(`${name}/values.yaml`, valuesYaml),
    tarEntry(`${name}/README.md`, readme),
    // a subchart's values must never be mistaken for the chart's own
    tarEntry(`${name}/charts/sub/values.yaml`, "subchart: true\n"),
    Buffer.alloc(1024),
  ]);
  return zlib.gzipSync(tar);
}

const CHARTS = [
  { name: "nginx", versions: ["15.1.0", "15.0.2"], appVersion: "1.25.3", description: "A fast web server", keywords: ["web", "proxy"], values: "replicaCount: 2\nservice:\n  type: ClusterIP\n", readme: "# nginx\nA web server chart.\n" },
  { name: "redis", versions: ["19.0.1"], appVersion: "7.2.4", description: "An in-memory data store", keywords: ["database", "cache"], values: "architecture: standalone\n", readme: "# redis\n" },
  { name: "old-thing", versions: ["1.0.0"], appVersion: "0.1", description: "Retired", keywords: [], deprecated: true, values: "a: 1\n", readme: "" },
];

async function startChartRepo() {
  const server = http.createServer((req, res) => {
    const u = new URL(req.url, "http://x");
    const base = `http://127.0.0.1:${/** @type {any} */ (server.address()).port}`;
    if (u.pathname === "/index.yaml") {
      let y = "apiVersion: v1\nentries:\n";
      for (const c of CHARTS) {
        y += `  ${c.name}:\n`;
        for (const v of c.versions) {
          y += `    - name: ${c.name}\n      version: ${v}\n      appVersion: "${c.appVersion}"\n      description: ${c.description}\n      created: "2026-10-01T00:00:00Z"\n`;
          if (c.deprecated) y += "      deprecated: true\n";
          if (c.keywords.length) y += `      keywords: [${c.keywords.join(", ")}]\n`;
          y += `      urls:\n        - ${base}/charts/${c.name}-${v}.tgz\n`;
        }
      }
      res.writeHead(200, { "Content-Type": "application/x-yaml" }).end(y);
      return;
    }
    const m = u.pathname.match(/^\/charts\/(.+)-(\d+\.\d+\.\d+)\.tgz$/);
    const chart = m && CHARTS.find((c) => c.name === m[1]);
    if (chart) {
      res.writeHead(200, { "Content-Type": "application/gzip" }).end(chartArchive(chart.name, m[2], chart.values, chart.readme));
      return;
    }
    res.writeHead(404).end("not found");
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", () => r(undefined)));
  const url = `http://127.0.0.1:${/** @type {any} */ (server.address()).port}`;
  return { url, close: () => new Promise((r) => server.close(() => r(undefined))) };
}

const PERMISSIVE = { AXIOM_HELM_ALLOW_PRIVATE_REPOS: "true", AXIOM_HELM_ALLOW_INSECURE_REPOS: "true" };

async function gotoOps(page, tab) {
  await page.goto(`/ops/${tab}`);
  await expect(page.getByRole("heading", { name: "Ops" })).toBeVisible();
}

test.describe("helm chart catalog", () => {
  test.setTimeout(90_000);

  test("add a repository, search, read default values and install a chart from it", async ({ browser }) => {
    const repo = await startChartRepo();
    try {
      await withApp(browser, PERMISSIVE, async ({ page }) => {
        await signInAs(page, USERNAME, PASSWORD);
        await gotoOps(page, "helm");
        await page.getByTestId("helm-view-catalog").click();
        await expect(page.getByTestId("helm-catalog")).toBeVisible();
        await expect(page.getByTestId("helm-catalog-empty")).toHaveCount(0);

        // Add the mock repository.
        await page.getByTestId("helm-repo-name").fill("mock");
        await page.getByTestId("helm-repo-url").fill(repo.url);
        const added = page.waitForResponse((r) => r.request().method() === "POST" && r.url().endsWith("/api/v1/helm/repos"));
        await page.getByTestId("helm-repo-add").click();
        expect((await added).status()).toBe(201);
        await expect(page.getByTestId("helm-repo-mock")).toContainText("3 charts");

        // The catalog lists one card per chart; deprecated ones are marked.
        await expect(page.getByTestId("helm-chart-mock-nginx")).toContainText("15.1.0");
        await expect(page.getByTestId("helm-chart-mock-redis")).toBeVisible();
        await expect(page.getByTestId("helm-chart-mock-old-thing")).toContainText("deprecated");

        // Search ranks and filters.
        await page.getByTestId("helm-catalog-search").fill("cache");
        await expect(page.getByTestId("helm-chart-mock-redis")).toBeVisible();
        await expect(page.getByTestId("helm-chart-mock-nginx")).toHaveCount(0);
        await page.getByTestId("helm-catalog-search").fill("zzz-nothing");
        await expect(page.getByTestId("helm-catalog-empty")).toBeVisible();
        await page.getByTestId("helm-catalog-search").fill("");

        // Open nginx: versions, default values (never the subchart's), install.
        await page.getByTestId("helm-chart-mock-nginx").click();
        const dialog = page.getByTestId("helm-chart-dialog");
        await expect(dialog).toBeVisible();
        await expect(dialog.getByTestId("helm-chart-version").locator("option")).toHaveCount(2);
        await dialog.getByTestId("helm-chart-show-values").click();
        await expect(dialog.getByTestId("helm-chart-values-editor")).toHaveValue(/replicaCount: 2/);
        await expect(dialog.getByTestId("helm-chart-values-editor")).not.toHaveValue(/subchart/);

        await dialog.getByTestId("helm-chart-version").selectOption("15.0.2");
        await dialog.getByTestId("helm-chart-namespace").fill("web");
        await dialog.getByTestId("helm-chart-release").fill("front");
        await dialog.getByTestId("helm-chart-values-editor").fill("replicaCount: 3\n");
        const install = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/v1/helm/install"));
        await dialog.getByTestId("helm-chart-install").click();
        const body = (await install).postDataJSON();
        expect(body).toMatchObject({ namespace: "web", releaseName: "front", repoUrl: repo.url, chartName: "nginx", chartVersion: "15.0.2", values: "replicaCount: 3\n" });
        await expect(page.getByTestId("helm-view-releases")).toHaveAttribute("aria-pressed", "true");

        // Out of band: the API agrees about what the catalog holds.
        const list = await (await page.request.get("/api/v1/helm/charts?repo=mock", { headers: ADMIN })).json();
        expect(list.charts.map((c) => c.name).sort()).toEqual(["nginx", "old-thing", "redis"]);
        expect(list.charts.find((c) => c.name === "nginx").version).toBe("15.1.0");

        // Remove the repository again.
        await page.getByTestId("helm-view-catalog").click();
        await page.getByTestId("helm-repo-remove-mock").click();
        const removed = page.waitForResponse((r) => r.request().method() === "DELETE" && /\/helm\/repos\/mock$/.test(r.url()));
        await page.getByRole("alertdialog").getByRole("button", { name: "Remove", exact: true }).click();
        expect((await removed).status()).toBe(200);
        await expect(page.getByTestId("helm-repo-mock")).toHaveCount(0);
      });
    } finally {
      await repo.close();
    }
  });

  test("private and metadata addresses are refused unless an administrator allows them", async ({ browser }) => {
    const repo = await startChartRepo();
    try {
      // Default policy: even a local repository is refused.
      await withApp(browser, {}, async ({ page }) => {
        const post = (url) => page.request.post("/api/v1/helm/repos", { headers: ADMIN, data: { name: "x", url } });
        for (const url of [repo.url, "http://169.254.169.254/latest", "https://localhost/charts", "https://10.0.0.5/charts", "file:///etc/passwd", "https://user:pw@charts.example.com"]) {
          const r = await post(url);
          expect(r.status(), url).toBe(400);
        }
        expect((await (await page.request.get("/api/v1/helm/repos", { headers: ADMIN })).json())).toEqual([]);
      });
    } finally {
      await repo.close();
    }
  });

  test("only administrators can change repositories; viewers can browse", async ({ browser }) => {
    const repo = await startChartRepo();
    try {
      await withApp(browser, PERMISSIVE, async ({ page }) => {
        const mk = async (username, body) => {
          const r = await page.request.post("/api/v1/users", { headers: ADMIN, data: { username, password: "pw-pw-pw-pw", ...body } });
          expect(r.status()).toBe(201);
          return basic(username, "pw-pw-pw-pw");
        };
        const operator = await mk("cat-oper", { role: "operator" });
        const viewer = await mk("cat-viewer", { role: "viewer" });
        const scoped = await mk("cat-scoped", { role: "viewer", namespaces: ["payments"] });

        const add = (h) => page.request.post("/api/v1/helm/repos", { headers: h, data: { name: "mock", url: repo.url } });
        expect((await add(operator)).status()).toBe(403);
        expect((await add(viewer)).status()).toBe(403);
        expect((await add(ADMIN)).status()).toBe(201);

        const get = (h) => page.request.get("/api/v1/helm/charts?repo=mock", { headers: h });
        expect((await get(viewer)).status()).toBe(200);
        expect((await get(operator)).status()).toBe(200);
        expect((await get(scoped)).status()).toBe(403);
        expect((await page.request.delete("/api/v1/helm/repos/mock", { headers: operator })).status()).toBe(403);
      });
    } finally {
      await repo.close();
    }
  });
});

test.describe("create GitOps applications", () => {
  test.setTimeout(90_000);

  test("Argo CD application: preview is required, then it is created and listed", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await gotoOps(page, "gitops");
      await page.getByTestId("gitops-new").click();
      const dlg = page.getByTestId("gitops-create-dialog");
      await expect(dlg).toBeVisible();

      // Nothing can be created before the YAML has been previewed.
      await expect(dlg.getByTestId("gitops-create")).toBeDisabled();
      await dlg.getByTestId("gitops-name").fill("shop");
      await dlg.getByTestId("gitops-repo-url").fill("https://github.com/acme/shop.git");
      await dlg.getByTestId("gitops-path").fill("deploy/prod");
      await dlg.getByTestId("gitops-dest-namespace").fill("shop-prod");
      await dlg.getByTestId("gitops-automated").check();
      await dlg.getByTestId("gitops-prune").check();
      await dlg.getByTestId("gitops-preview").click();
      const yaml = dlg.getByTestId("gitops-preview-yaml");
      await expect(yaml).toContainText("kind: Application");
      await expect(yaml).toContainText("repoURL: https://github.com/acme/shop.git");
      await expect(yaml).toContainText("path: deploy/prod");
      await expect(yaml).toContainText("namespace: shop-prod");
      await expect(dlg.getByTestId("gitops-warnings")).toContainText(/prune/i);
      await expect(dlg.getByTestId("gitops-create")).toBeEnabled();

      // Changing any field invalidates the preview.
      await dlg.getByTestId("gitops-path").fill("deploy/staging");
      await expect(dlg.getByTestId("gitops-create")).toBeDisabled();
      await dlg.getByTestId("gitops-preview").click();
      await expect(yaml).toContainText("path: deploy/staging");

      const created = page.waitForResponse((r) => r.request().method() === "POST" && r.url().endsWith("/api/v1/gitops"));
      await dlg.getByTestId("gitops-create").click();
      expect((await created).status()).toBe(201);
      await expect(page.getByRole("row", { name: /shop/ })).toContainText("Application");

      const list = await (await page.request.get("/api/v1/gitops", { headers: ADMIN })).json();
      expect(list.find((g) => g.name === "shop")).toMatchObject({ kind: "Application", namespace: "argocd" });

      // Creating the same application again is a conflict, shown in the dialog.
      await page.getByTestId("gitops-new").click();
      await dlg.getByTestId("gitops-name").fill("shop");
      await dlg.getByTestId("gitops-repo-url").fill("https://github.com/acme/shop.git");
      await dlg.getByTestId("gitops-dest-namespace").fill("shop-prod");
      await dlg.getByTestId("gitops-preview").click();
      await dlg.getByTestId("gitops-create").click();
      await expect(dlg.getByTestId("gitops-error")).toContainText(/already exists/i);
    });
  });

  test("Flux Git + Kustomize creates the source and the Kustomization; credentials in a URL are refused", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await gotoOps(page, "gitops");
      await page.getByTestId("gitops-new").click();
      const dlg = page.getByTestId("gitops-create-dialog");
      await dlg.getByTestId("gitops-engine-flux-kustomize").click();
      await dlg.getByTestId("gitops-name").fill("platform");
      await dlg.getByTestId("gitops-dest-namespace").fill("platform");

      // A password in the URL is rejected and the message says what to do instead.
      await dlg.getByTestId("gitops-repo-url").fill("https://ci:hunter2@git.example.com/org/platform.git");
      await dlg.getByTestId("gitops-preview").click();
      await expect(dlg.getByTestId("gitops-error")).toContainText(/secret|credential/i);
      await expect(dlg.getByTestId("gitops-error")).not.toContainText("hunter2");

      await dlg.getByTestId("gitops-repo-url").fill("https://git.example.com/org/platform.git");
      await dlg.getByTestId("gitops-secret-ref").fill("git-creds");
      await dlg.getByTestId("gitops-preview").click();
      const yaml = dlg.getByTestId("gitops-preview-yaml");
      await expect(yaml).toContainText("kind: GitRepository");
      await expect(yaml).toContainText("kind: Kustomization");
      await expect(yaml).toContainText("name: git-creds");
      await dlg.getByTestId("gitops-create").click();
      await expect(page.getByRole("row", { name: /platform/ }).first()).toBeVisible();
      const list = await (await page.request.get("/api/v1/gitops", { headers: ADMIN })).json();
      expect(list.some((g) => g.kind === "Kustomization" && g.name === "platform")).toBe(true);
    });
  });

  test("operators and viewers cannot create applications", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      const mk = async (username, role) => {
        const r = await page.request.post("/api/v1/users", { headers: ADMIN, data: { username, password: "pw-pw-pw-pw", role } });
        expect(r.status()).toBe(201);
        return basic(username, "pw-pw-pw-pw");
      };
      const operator = await mk("gitops-oper", "operator");
      const viewer = await mk("gitops-viewer", "viewer");
      const body = { engine: "argocd", name: "x", source: { repoUrl: "https://github.com/a/b.git", path: "." }, destination: { namespace: "x" } };
      for (const h of [operator, viewer]) {
        expect((await page.request.post("/api/v1/gitops", { headers: h, data: body })).status()).toBe(403);
        expect((await page.request.post("/api/v1/gitops/preview", { headers: h, data: body })).status()).toBe(403);
      }
      expect((await page.request.post("/api/v1/gitops/preview", { headers: ADMIN, data: body })).status()).toBe(200);
    });
  });
});
