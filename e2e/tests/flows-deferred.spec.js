// @ts-check
// Deferred-roadmap features: fleet health history, OpenCost actual cost, OCI Helm
// registries, GitOps edit + write-only credentials, VPA/KEDA creation and the
// separate pod-proxy origin. Each runs on its
// OWN demo server (started from ../../bin/zyvor-axiom like flows-fleet.spec.js) so
// destructive actions and server-wide policy never disturb the shared demo state.
const { test, expect } = require("@playwright/test");
const net = require("net");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { spawn } = require("child_process");
const { USERNAME, PASSWORD, confirmDialog } = require("./helpers");

const BIN_SRC = path.resolve(__dirname, "../../bin/zyvor-axiom");
/** @type {string} */
let BIN = "";
const running = new Set();
const basic = (u, p) => ({ Authorization: `Basic ${Buffer.from(`${u}:${p}`).toString("base64")}` });

test.beforeAll(() => {
  if (!fs.existsSync(BIN_SRC)) return;
  BIN = path.join(os.tmpdir(), `axiom-e2e-deferred-${process.pid}`);
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



const crypto = require("crypto");
const http = require("http");
const zlib = require("zlib");
const ADMIN = basic(USERNAME, PASSWORD);

test.describe("fleet health history", () => {
  test.setTimeout(90_000);

  test("uptime, a state strip and incidents per cluster, over a chosen window", async ({ browser }) => {
    await withApp(browser, { AXIOM_DEMO_FLEET_HISTORY_SEED: "true" }, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/fleet");
      // The seeded 24 h history has one degraded run on ap-south-1 and none elsewhere.
      await expect(page.getByTestId("fleet-uptime-eu-west-1")).toContainText("100");
      await expect(page.getByTestId("fleet-no-incidents-eu-west-1")).toBeVisible();
      await expect(page.getByTestId("fleet-strip-eu-west-1")).toBeVisible();
      await expect(page.getByTestId("fleet-incident-ap-south-1-0")).toContainText("Degraded for");
      const pct = parseFloat(((await page.getByTestId("fleet-uptime-ap-south-1").textContent()) || "").replace("%", ""));
      expect(pct).toBeGreaterThan(90);
      expect(pct).toBeLessThanOrEqual(100);

      // A shorter window past the incident has none to show.
      const hist = page.waitForResponse((r) => r.url().includes("/fleet/history") && r.url().includes("window=1h"));
      await page.getByTestId("fleet-history-window").selectOption("1h");
      expect((await hist).status()).toBe(200);
      await expect(page.getByTestId("fleet-no-incidents-ap-south-1")).toBeVisible();

      // Out of band: the API agrees, the strip is downsampled, and bad input is refused.
      const api = await (await page.request.get("/api/v1/fleet/history?window=24h", { headers: ADMIN })).json();
      const ap = api.clusters.find((c) => c.cluster === "ap-south-1");
      expect(ap.incidents.length).toBe(1);
      expect(ap.incidents[0].state).toBe("degraded");
      expect(ap.samples.length).toBeLessThanOrEqual(240);
      expect((await page.request.get("/api/v1/fleet/history?window=3d", { headers: ADMIN })).status()).toBe(400);
      expect((await page.request.get("/api/v1/fleet/history?cluster=nope", { headers: ADMIN })).status()).toBe(404);
    });
  });
});

test.describe("OpenCost actual cost", () => {
  test.setTimeout(90_000);

  test("without OpenCost the page says it is an estimate", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/ops/cost");
      await expect(page.getByTestId("cost-source")).toContainText("Estimate from requests");
      await expect(page.getByTestId("cost-window")).toHaveCount(0);
      expect((await (await page.request.get("/api/v1/cost", { headers: ADMIN })).json()).source).toBe("estimate");
    });
  });

  test("with OpenCost it shows actual cost, the idle share, a breakdown and a window", async ({ browser }) => {
    await withApp(browser, { AXIOM_DEMO_OPENCOST: "true" }, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/ops/cost");
      await expect(page.getByTestId("cost-source")).toContainText("Actual cost");
      await expect(page.getByTestId("cost-idle")).toBeVisible();
      await expect(page.getByTestId("cost-breakdown-payments")).toContainText("cpu $");

      const week = page.waitForResponse((r) => r.url().includes("/api/v1/cost") && r.url().includes("window=7d"));
      await page.getByTestId("cost-window").selectOption("7d");
      expect((await week).status()).toBe(200);

      const actual = await (await page.request.get("/api/v1/cost?window=30d", { headers: ADMIN })).json();
      expect(actual.source).toBe("opencost");
      expect(actual.idleMonthlyUsd).toBeGreaterThan(0);
      const row = actual.namespaces.find((n) => n.namespace === "payments");
      expect(row.cpuUsd).toBeGreaterThan(0);
      expect((await page.request.get("/api/v1/cost?window=2d", { headers: ADMIN })).status()).toBe(400);
      // Budgets follow the same source.
      const budgets = await (await page.request.get("/api/v1/cost/budgets", { headers: ADMIN })).json();
      expect(budgets.source).toBe("opencost");
    });
  });
});

// ---------------------------------------------------------------------------
// A mock OCI registry (Distribution API): ping, tag list, manifest, blobs,
// with real digests so Axiom's integrity check passes.
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
  return Buffer.concat([header, body, Buffer.alloc((512 - (body.length % 512)) % 512)]);
}
const sha = (b) => "sha256:" + crypto.createHash("sha256").update(b).digest("hex");

async function startOciRegistry() {
  const charts = {
    "charts/podinfo": { versions: ["6.5.4", "6.5.3"], description: "Tiny web app", values: "replicaCount: 2\nui:\n  color: blue\n" },
    "charts/whoami": { versions: ["1.0.0"], description: "Echo request headers", values: "replicas: 1\n" },
  };
  const blobs = new Map(); // digest -> {type, data}
  const manifests = new Map(); // "name:tag" -> {json}
  for (const [name, c] of Object.entries(charts)) {
    for (const v of c.versions) {
      const short = name.split("/").pop();
      const cfg = Buffer.from(JSON.stringify({ name: short, version: v, appVersion: v, description: c.description, keywords: ["demo"] }));
      const layer = zlib.gzipSync(Buffer.concat([
        tarEntry(`${short}/Chart.yaml`, `apiVersion: v2\nname: ${short}\nversion: ${v}\n`),
        tarEntry(`${short}/values.yaml`, c.values),
        tarEntry(`${short}/README.md`, `# ${short}\n`),
        Buffer.alloc(1024),
      ]));
      const cd = sha(cfg), ld = sha(layer);
      blobs.set(cd, { type: "application/vnd.cncf.helm.config.v1+json", data: cfg });
      blobs.set(ld, { type: "application/vnd.cncf.helm.chart.content.v1.tar+gzip", data: layer });
      manifests.set(`${name}:${v}`, JSON.stringify({
        schemaVersion: 2, mediaType: "application/vnd.oci.image.manifest.v1+json",
        config: { mediaType: "application/vnd.cncf.helm.config.v1+json", digest: cd, size: cfg.length },
        layers: [{ mediaType: "application/vnd.cncf.helm.chart.content.v1.tar+gzip", digest: ld, size: layer.length }],
      }));
    }
  }
  const server = http.createServer((req, res) => {
    const u = new URL(req.url, "http://x");
    const p = u.pathname;
    if (p === "/v2/" || p === "/v2") return res.writeHead(200, { "Content-Type": "application/json" }).end("{}");
    let m = p.match(/^\/v2\/(.+)\/tags\/list$/);
    if (m) {
      const c = charts[m[1]];
      if (!c) return res.writeHead(404).end("{}");
      return res.writeHead(200, { "Content-Type": "application/json" }).end(JSON.stringify({ name: m[1], tags: c.versions }));
    }
    m = p.match(/^\/v2\/(.+)\/manifests\/(.+)$/);
    if (m) {
      const j = manifests.get(`${m[1]}:${m[2]}`);
      if (!j) return res.writeHead(404).end("{}");
      return res.writeHead(200, { "Content-Type": "application/vnd.oci.image.manifest.v1+json", "Docker-Content-Digest": sha(Buffer.from(j)) }).end(j);
    }
    m = p.match(/^\/v2\/(.+)\/blobs\/(sha256:[0-9a-f]+)$/);
    if (m) {
      const b = blobs.get(m[2]);
      if (!b) return res.writeHead(404).end("{}");
      return res.writeHead(200, { "Content-Type": b.type }).end(b.data);
    }
    res.writeHead(404).end("{}");
  });
  await new Promise((r) => server.listen(0, "127.0.0.1", () => r(undefined)));
  const host = `127.0.0.1:${/** @type {any} */ (server.address()).port}`;
  return { host, url: `oci://${host}/charts`, close: () => new Promise((r) => server.close(() => r(undefined))) };
}

test.describe("OCI Helm registries", () => {
  test.setTimeout(90_000);
  const PERMISSIVE = { AXIOM_HELM_ALLOW_PRIVATE_REPOS: "true", AXIOM_HELM_ALLOW_INSECURE_REPOS: "true" };

  test("add a registry with its chart names, browse, read default values and install by oci:// reference", async ({ browser }) => {
    const reg = await startOciRegistry();
    try {
      await withApp(browser, PERMISSIVE, async ({ page }) => {
        await signInAs(page, USERNAME, PASSWORD);
        await page.goto("/ops/helm");
        await page.getByTestId("helm-view-catalog").click();
        await page.getByTestId("helm-repo-kind-oci").check();
        await page.getByTestId("helm-repo-name").fill("local-oci");
        await page.getByTestId("helm-repo-url").fill(reg.url);
        await page.getByTestId("helm-repo-oci-charts").fill("podinfo, whoami");
        const added = page.waitForResponse((r) => r.request().method() === "POST" && r.url().endsWith("/api/v1/helm/repos"));
        await page.getByTestId("helm-repo-add").click();
        expect((await added).status()).toBe(201);
        await expect(page.getByTestId("helm-repo-local-oci")).toContainText("OCI");

        await expect(page.getByTestId("helm-chart-oci-local-oci-podinfo")).toBeVisible();
        await page.getByTestId("helm-chart-local-oci-podinfo").click();
        const dialog = page.getByTestId("helm-chart-dialog");
        await expect(dialog.getByTestId("helm-chart-oci-ref")).toContainText(`oci://${reg.host}/charts/podinfo`);
        await expect(dialog.getByTestId("helm-chart-version").locator("option")).toHaveCount(2);
        await dialog.getByTestId("helm-chart-show-values").click();
        await expect(dialog.getByTestId("helm-chart-values-editor")).toHaveValue(/color: blue/);

        const install = page.waitForRequest((r) => r.method() === "POST" && r.url().endsWith("/api/v1/helm/install"));
        await dialog.getByTestId("helm-chart-namespace").fill("web");
        await dialog.getByTestId("helm-chart-release").fill("pod");
        await dialog.getByTestId("helm-chart-install").click();
        const body = (await install).postDataJSON();
        expect(body).toMatchObject({ namespace: "web", releaseName: "pod", ociRef: `oci://${reg.host}/charts/podinfo` });
        expect(body.repoUrl).toBeUndefined();
        expect(body.chartVersion).toBeTruthy();
      });
    } finally {
      await reg.close();
    }
  });

  test("a registry on a private address is refused by default, and a bad chart fails the add", async ({ browser }) => {
    const reg = await startOciRegistry();
    try {
      await withApp(browser, {}, async ({ page }) => {
        const r = await page.request.post("/api/v1/helm/repos", { headers: ADMIN, data: { name: "x", url: reg.url, charts: ["podinfo"] } });
        expect(r.status()).toBe(400);
      });
      await withApp(browser, PERMISSIVE, async ({ page }) => {
        const bad = await page.request.post("/api/v1/helm/repos", { headers: ADMIN, data: { name: "x", url: reg.url, charts: ["does-not-exist"] } });
        expect(bad.status()).toBe(400);
        const flag = await page.request.post("/api/v1/helm/install", { headers: ADMIN, data: { namespace: "a", releaseName: "b", ociRef: "oci://user:pw@host/x/y" } });
        expect(flag.status()).toBe(400);
      });
    } finally {
      await reg.close();
    }
  });
});

test.describe("GitOps edit and repository credentials", () => {
  test.setTimeout(90_000);

  test("edit an application's revision after previewing the exact change", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/ops/gitops");
      await page.getByTestId("gitops-edit-Application-argocd-platform-apps").click();
      const dlg = page.getByTestId("gitops-edit-dialog");
      await expect(dlg.getByTestId("gitops-edit-revision")).toHaveValue("main");
      await expect(dlg.getByTestId("gitops-edit-save")).toBeDisabled();

      await dlg.getByTestId("gitops-edit-revision").fill("release-2");
      await dlg.getByTestId("gitops-edit-preview").click();
      const changes = dlg.getByTestId("gitops-edit-changes");
      await expect(changes).toContainText("targetRevision");
      await expect(changes).toContainText("main → release-2");
      await expect(dlg.getByTestId("gitops-edit-save")).toBeEnabled();

      // Changing a field after previewing invalidates the preview.
      await dlg.getByTestId("gitops-edit-path").fill("apps-v2");
      await expect(dlg.getByTestId("gitops-edit-save")).toBeDisabled();
      await dlg.getByTestId("gitops-edit-preview").click();
      await expect(dlg.getByTestId("gitops-edit-changes")).toContainText("apps-v2");

      const saved = page.waitForResponse((r) => r.request().method() === "PUT" && /\/gitops\/Application\/argocd\/platform-apps$/.test(r.url()));
      await dlg.getByTestId("gitops-edit-save").click();
      expect((await saved).status()).toBe(200);

      const now = await (await page.request.get("/api/v1/gitops/Application/argocd/platform-apps/edit", { headers: ADMIN })).json();
      expect(now.source.revision).toBe("release-2");
      expect(now.source.path).toBe("apps-v2");
      // A stale edit (old resourceVersion) is refused.
      const stale = await page.request.put("/api/v1/gitops/Application/argocd/platform-apps", {
        headers: ADMIN,
        data: { source: now.source, destination: now.destination, project: now.project, syncPolicy: now.syncPolicy, resourceVersion: "0" },
      });
      expect(stale.status()).toBe(409);
    });
  });

  test("credentials are write-only: saved, listed without their secret, and removable", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/ops/gitops");
      await page.getByTestId("gitops-creds-open").click();
      const dlg = page.getByTestId("gitops-creds-dialog");
      await expect(dlg.getByTestId("gitops-cred-repo-platform")).toBeVisible();
      await expect(dlg.getByTestId("gitops-cred-remove-repo-platform")).toHaveCount(0); // not created by Axiom
      await expect(dlg.getByTestId("gitops-cred-writeonly")).toContainText("cannot be read back");

      const SECRET = "ghp_e2eDistinctiveSecret_9f3a7c";
      await dlg.getByTestId("gitops-cred-name").fill("ci-token");
      await dlg.getByTestId("gitops-cred-url").fill("https://github.com/acme/private.git");
      await dlg.getByTestId("gitops-cred-token").fill(SECRET);
      const created = page.waitForResponse((r) => r.request().method() === "POST" && r.url().endsWith("/api/v1/gitops/credentials"));
      await dlg.getByTestId("gitops-cred-save").click();
      expect((await created).status()).toBe(201);
      await expect(dlg.getByTestId("gitops-cred-ci-token")).toBeVisible();
      await expect(page.locator("body")).not.toContainText(SECRET);

      // Out of band: the secret is nowhere in what the API hands back.
      const list = await page.request.get("/api/v1/gitops/credentials?engine=argocd", { headers: ADMIN });
      expect(await list.text()).not.toContain(SECRET);

      // A second one with the same name is refused; removal works; non-admins see nothing.
      const dup = await page.request.post("/api/v1/gitops/credentials", { headers: ADMIN, data: { engine: "argocd", name: "ci-token", url: "https://github.com/acme/private.git", authType: "token", token: "x" } });
      expect(dup.status()).toBe(409);
      const mk = await page.request.post("/api/v1/users", { headers: ADMIN, data: { username: "cred-oper", password: "pw-pw-pw-pw", role: "operator" } });
      expect(mk.status()).toBe(201);
      expect((await page.request.get("/api/v1/gitops/credentials?engine=argocd", { headers: basic("cred-oper", "pw-pw-pw-pw") })).status()).toBe(403);

      await dlg.getByTestId("gitops-cred-remove-ci-token").click();
      const removed = page.waitForResponse((r) => r.request().method() === "DELETE" && /\/gitops\/credentials\/argocd\/argocd\/ci-token$/.test(r.url()));
      await confirmDialog(page, "Delete");
      expect((await removed).status()).toBe(200);
      await expect(dlg.getByTestId("gitops-cred-ci-token")).toHaveCount(0);
    });
  });
});

test.describe("create VPAs and KEDA ScaledObjects", () => {
  test.setTimeout(90_000);

  test("a recommend-only VPA: preview the YAML, create, see it, delete it", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/autoscaling");
      await page.getByTestId("autoscale-tab-vpa").click();
      await page.getByTestId("vpa-new").click();
      const dlg = page.getByTestId("vpa-create-dialog");
      await dlg.getByTestId("vpa-target").selectOption("payments|Deployment|payments-api");
      await dlg.getByTestId("vpa-preview").click();
      await expect(dlg.getByTestId("vpa-preview-yaml")).toContainText("kind: VerticalPodAutoscaler");
      await expect(dlg.getByTestId("vpa-preview-yaml")).toContainText("updateMode: \"Off\"");

      const created = page.waitForResponse((r) => r.request().method() === "POST" && r.url().endsWith("/api/v1/vpas"));
      await dlg.getByTestId("vpa-create").click();
      expect((await created).status()).toBe(201);
      await expect(page.getByTestId("vpa-row-payments-payments-api-vpa")).toBeVisible();

      // Auto mode warns that pods may be restarted.
      await page.getByTestId("vpa-new").click();
      await dlg.getByTestId("vpa-target").selectOption("data|StatefulSet|redis");
      await dlg.getByTestId("vpa-name").fill("redis-auto");
      await dlg.getByTestId("vpa-mode").selectOption("Auto");
      await dlg.getByTestId("vpa-preview").click();
      await expect(dlg.getByTestId("vpa-warnings")).toContainText(/restart/i);
      await page.keyboard.press("Escape");

      const gone = page.waitForResponse((r) => r.request().method() === "DELETE" && /\/vpas\/payments\/payments-api-vpa$/.test(r.url()));
      await page.getByTestId("vpa-remove-payments-payments-api-vpa").click();
      await confirmDialog(page, "Delete");
      expect((await gone).status()).toBe(200);
      await expect(page.getByTestId("vpa-row-payments-payments-api-vpa")).toHaveCount(0);
    });
  });

  test("a KEDA ScaledObject from a cron trigger, then delete; credentials are never typed", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/autoscaling");
      await page.getByTestId("autoscale-tab-keda").click();
      await page.getByTestId("keda-new").click();
      const dlg = page.getByTestId("keda-create-dialog");
      await dlg.getByTestId("keda-target").selectOption("observability|Deployment|otel-collector");
      await dlg.getByTestId("keda-name").fill("otel-business-hours");
      await dlg.getByTestId("keda-trigger-0-timezone").fill("Europe/Berlin");
      await dlg.getByTestId("keda-trigger-0-start").fill("0 8 * * 1-5");
      await dlg.getByTestId("keda-trigger-0-end").fill("0 18 * * 1-5");
      await dlg.getByTestId("keda-trigger-0-desiredReplicas").fill("4");
      await dlg.getByTestId("keda-preview").click();
      const yaml = dlg.getByTestId("keda-preview-yaml");
      await expect(yaml).toContainText("kind: ScaledObject");
      await expect(yaml).toContainText("type: cron");
      await expect(yaml).toContainText("Europe/Berlin");

      // A trigger that needs credentials offers existing TriggerAuthentications by name, never a password field.
      await dlg.getByTestId("keda-add-trigger").click();
      await dlg.getByTestId("keda-trigger-type-1").selectOption("prometheus");
      await expect(dlg.getByTestId("keda-trigger-auth-1")).toBeVisible();
      await expect(dlg.locator('input[type="password"]')).toHaveCount(0);
      await dlg.getByTestId("keda-trigger-remove-1").click();
      await dlg.getByTestId("keda-preview").click();

      const created = page.waitForResponse((r) => r.request().method() === "POST" && r.url().endsWith("/api/v1/scaledobjects"));
      await dlg.getByTestId("keda-create").click();
      expect((await created).status()).toBe(201);
      await expect(page.getByTestId("keda-row-observability-otel-business-hours")).toContainText("cron");

      // The server rejects credential-looking settings even if a client tries.
      const sneaky = await page.request.post("/api/v1/scaledobjects", {
        headers: ADMIN,
        data: { namespace: "payments", name: "bad", targetKind: "Deployment", targetName: "payments-api", maxReplicas: 3, triggers: [{ type: "prometheus", settings: { serverAddress: "http://p:9090", query: "up", threshold: "1", token: "hunter2" } }] },
      });
      expect(sneaky.status()).toBe(400);
      expect(await sneaky.text()).not.toContain("hunter2");

      const gone = page.waitForResponse((r) => r.request().method() === "DELETE" && /\/scaledobjects\/observability\/otel-business-hours$/.test(r.url()));
      await page.getByTestId("keda-remove-observability-otel-business-hours").click();
      await confirmDialog(page, "Delete");
      expect((await gone).status()).toBe(200);
    });
  });
});

test.describe("pod proxy on a separate origin", () => {
  test.setTimeout(90_000);

  test("registry logins are write-only and a repository can use one", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      const mk = await page.request.post("/api/v1/helm/registry-credentials", { headers: ADMIN, data: { name: "ci-reg", registry: "registry.example.com", username: "bot", password: "s3cret-pass-value" } });
      expect(mk.status()).toBe(201);
      const list = await page.request.get("/api/v1/helm/registry-credentials", { headers: ADMIN });
      const text = await list.text();
      expect(text).toContain("ci-reg");
      expect(text).not.toContain("s3cret-pass-value");
      const viewer = await page.request.post("/api/v1/users", { headers: ADMIN, data: { username: "rc-viewer", password: "pw-pw-pw-pw", role: "viewer" } });
      expect(viewer.status()).toBe(201);
      expect((await page.request.get("/api/v1/helm/registry-credentials", { headers: basic("rc-viewer", "pw-pw-pw-pw") })).status()).toBe(403);
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/ops/helm");
      await page.getByTestId("helm-view-catalog").click();
      await page.getByTestId("helm-repo-kind-oci").check();
      await page.getByTestId("helm-regcred-manage").click();
      await expect(page.getByTestId("helm-regcred-ci-reg")).toBeVisible();
      await expect(page.getByTestId("helm-regcred-dialog")).not.toContainText("s3cret-pass-value");
      await page.getByTestId("helm-regcred-remove-ci-reg").click();
      await confirmDialog(page, "Delete");
      await expect(page.getByTestId("helm-regcred-ci-reg")).toHaveCount(0);
    });
  });

  test("edit a VPA in place after previewing the change", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      await signInAs(page, USERNAME, PASSWORD);
      await page.goto("/autoscaling");
      await page.getByTestId("autoscale-tab-vpa").click();
      const list = await (await page.request.get("/api/v1/vpas", { headers: ADMIN })).json();
      const v = list.items[0];
      await page.getByTestId(`vpa-edit-${v.namespace}-${v.name}`).click();
      const dlg = page.getByTestId("autoscale-edit-dialog");
      await expect(dlg.getByTestId("vpa-edit-mode")).toBeVisible();
      await dlg.getByTestId("vpa-edit-mode").selectOption("Initial");
      await dlg.getByTestId("autoscale-edit-preview").click();
      await expect(dlg.getByTestId("autoscale-edit-changes")).toBeVisible();
      const saved = page.waitForResponse((r) => r.request().method() === "PUT" && r.url().includes(`/vpas/${v.namespace}/${v.name}`));
      await dlg.getByTestId("autoscale-edit-save").click();
      expect((await saved).status()).toBe(200);
    });
  });

  test("fleet history reports whether it is durable", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      const h = await (await page.request.get("/api/v1/fleet/history", { headers: ADMIN })).json();
      expect(h.durable).toBe(false);
    });
  });

  test("per-pod host mode hands out a distinct hostname per link", async ({ browser }) => {
    const proxyPort = await freePort();
    await withApp(browser, { AXIOM_POD_PROXY_ADDR: `127.0.0.1:${proxyPort}`, AXIOM_POD_PROXY_ORIGIN: `http://{id}.localhost:${proxyPort}`, AXIOM_ALLOW_ORIGIN: "http://127.0.0.1" }, async ({ page }) => {
      const caps = await (await page.request.get("/api/v1/realtime/capabilities", { headers: ADMIN })).json();
      expect(caps.podProxyMode).toBe("host");
      const pods = await (await page.request.get("/api/v1/pods?namespace=payments", { headers: ADMIN })).json();
      const link = async () => (await (await page.request.post(`/api/v1/pods/payments/${pods[0].name}/window-link`, { headers: ADMIN, data: { port: 8080, path: "/" } })).json()).url;
      const [a, b] = [await link(), await link()];
      const re = new RegExp(`^http://[0-9a-f]{32}\\.localhost:${proxyPort}/p/`);
      expect(a).toMatch(re);
      expect(b).toMatch(re);
      expect(a.split(".")[0]).not.toBe(b.split(".")[0]);
    });
  });

  test("window links point at the isolated origin and the in-origin route is retired", async ({ browser }) => {
    const proxyPort = await freePort();
    await withApp(browser, { AXIOM_POD_PROXY_ADDR: `127.0.0.1:${proxyPort}`, AXIOM_POD_PROXY_ORIGIN: `http://localhost:${proxyPort}` }, async ({ page }) => {
      const caps = await (await page.request.get("/api/v1/realtime/capabilities", { headers: ADMIN })).json();
      expect(caps.podProxy).toBe(true);
      const pods = await (await page.request.get("/api/v1/pods?namespace=payments", { headers: ADMIN })).json();
      const pod = pods[0].name;
      const link = await page.request.post(`/api/v1/pods/payments/${pod}/window-link`, { headers: ADMIN, data: { port: 8080, path: "/" } });
      expect(link.status()).toBe(200);
      const { url, expiresIn } = await link.json();
      expect(url).toMatch(new RegExp(`^http://localhost:${proxyPort}/p/[A-Za-z0-9_-]{20,}/`));
      expect(expiresIn).toBe(60);
      // The in-origin route no longer serves pod content.
      expect((await page.request.get(`/api/v1/pods/payments/${pod}/window/8080/`, { headers: ADMIN })).status()).toBe(410);
      // The proxy listener serves nothing but tickets and sessions, and never the console's API.
      expect((await fetch(`http://127.0.0.1:${proxyPort}/api/v1/health`)).status).toBe(404);
      expect((await fetch(`http://127.0.0.1:${proxyPort}/api/v1/pods`, { headers: ADMIN })).status).toBe(404);
      // A viewer cannot mint a link.
      const mk = await page.request.post("/api/v1/users", { headers: ADMIN, data: { username: "pp-viewer", password: "pw-pw-pw-pw", role: "viewer" } });
      expect(mk.status()).toBe(201);
      const denied = await page.request.post(`/api/v1/pods/payments/${pod}/window-link`, { headers: basic("pp-viewer", "pw-pw-pw-pw"), data: { port: 8080, path: "/" } });
      expect(denied.status()).toBe(403);
    });
  });

  test("without the feature nothing changes", async ({ browser }) => {
    await withApp(browser, {}, async ({ page }) => {
      const caps = await (await page.request.get("/api/v1/realtime/capabilities", { headers: ADMIN })).json();
      expect(caps.podProxy).toBe(false);
    });
  });
});
