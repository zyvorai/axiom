// @ts-check
// Functional specs for the workload-facing state-changing flows: Workloads
// (Run-workload wizard, scale, rollout, start/stop/restart/seal, delete, bulk),
// WorkloadDetail (apply), Pods (evict, bulk evict, exec, PodDetail), Jobs &
// CronJobs and Nodes (cordon/uncordon/drain/taints).
//
// Same contract as flows-core.spec.js: drive the real UI, assert the mutating
// request returned 2xx (track), assert the UI reflects it, and assert an
// independent API read agrees. Designed for a *demo-mode* server; every test
// that mutates creates its own uniquely named workload so it never disturbs
// the seeded data, and node tests always restore the node they touched.
//
// Demo-mode notes that shape a few assertions:
//   * POST /apply answers 200 {valid:false, message:"apply is not available in
//     demo mode"} — the UI must surface that message rather than claim success.
//   * There is no API or UI to create a CronJob, and no API to delete a Node,
//     so the two cronjob-delete flows act on seeded data (skipped once gone —
//     restart the demo server to restore them).
const { test, expect } = require("@playwright/test");
const { signIn, track, apiGet, apiSend, waitMutation, confirmDialog, toastText } = require("./helpers");

const uid = () => Math.random().toString(36).slice(2, 8);
const IMAGE = "ghcr.io/zyvorai/e2e:v1";

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

// ---- helpers ---------------------------------------------------------------

/** Creates a Deployment straight through the API (setup only, not the flow under test). */
async function seedWorkload(page, name, replicas = 2, namespace = "default") {
  const r = await apiSend(page, "POST", "/workloads", { namespace, name, image: IMAGE, replicas, port: 8080, cpu: "100m", memory: "64Mi" });
  expect(r.status).toBe(201);
}
async function dropWorkload(page, name, kind = "Deployment", namespace = "default") {
  await apiSend(page, "DELETE", `/workloads/${namespace}/${kind}/${name}`);
}
async function findWorkload(page, name) {
  const { data } = await apiGet(page, "/workloads");
  return (data || []).find((w) => w.name === name);
}
async function findPods(page, prefix) {
  const { data } = await apiGet(page, "/pods");
  return (data || []).filter((p) => p.name.startsWith(prefix));
}
async function findNode(page, name) {
  const { data } = await apiGet(page, "/nodes");
  return (data || []).find((n) => n.name === name);
}

/** Workloads page row for `name` (the row holding the workload's link). */
const wlRow = (page, name) => page.getByRole("row").filter({ has: page.getByRole("link", { name, exact: true }) });
/** Any table row that mentions `text` (pods, jobs). */
const rowWith = (page, text) => page.locator("tbody tr").filter({ hasText: text });

async function gotoWorkloads(page) {
  await page.getByTestId("nav-link-workloads").click();
  await expect(page.getByRole("heading", { name: "Workloads", level: 1 })).toBeVisible();
}
async function openWorkloadMenu(page, name, item) {
  await page.getByRole("button", { name: `Actions for ${name}`, exact: true }).click();
  await page.getByRole("menuitem", { name: item, exact: true }).click();
}

async function openWizard(page, tab) {
  await gotoWorkloads(page);
  await page.getByRole("button", { name: "Run workload" }).first().click();
  const dlg = page.getByRole("dialog", { name: "Run workload" });
  await expect(dlg).toBeVisible();
  if (tab) await dlg.getByRole("button", { name: tab }).click();
  return dlg;
}
async function fillForm(dlg, { name, image, replicas, namespace }) {
  if (namespace !== undefined) await dlg.getByLabel("Namespace", { exact: true }).fill(namespace);
  if (name !== undefined) await dlg.getByLabel("Name", { exact: true }).fill(name);
  if (image !== undefined) await dlg.getByLabel("Image", { exact: true }).fill(image);
  if (replicas !== undefined) await dlg.getByLabel("Replicas", { exact: true }).fill(String(replicas));
}
const deploymentYaml = (name) =>
  `apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: ${name}\n  namespace: default\nspec:\n  replicas: 1\n`;

// ============================================================================
// Workloads: Run-workload wizard
// ============================================================================

test("wizard/form: launch creates a Deployment (POST /workloads), row + API agree", async ({ page }) => {
  const t = track(page);
  const name = `e2e-form-${uid()}`;
  const dlg = await openWizard(page);
  await fillForm(dlg, { name, image: IMAGE, replicas: 3 });
  const posted = waitMutation(page, "POST", /\/workloads$/);
  await dlg.getByRole("button", { name: "Launch" }).click();
  expect((await posted).status()).toBe(201);
  expect(t.find("POST", /\/workloads$/)[0].body).toMatchObject({ name, image: IMAGE, replicas: 3, namespace: "default" });

  await toastText(page, new RegExp(`${name} launched`));
  await expect(dlg).toBeHidden();
  const row = wlRow(page, name);
  await expect(row).toBeVisible();
  await expect(row).toContainText(IMAGE);
  await expect(row).toContainText("Deployment");

  const w = await findWorkload(page, name);
  expect(w).toMatchObject({ kind: "Deployment", desired: 3, image: IMAGE });
  expect(await findPods(page, name)).toHaveLength(3);

  await dropWorkload(page, name);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("wizard/form negative: Launch is blocked until name and image are given", async ({ page }) => {
  const t = track(page);
  const dlg = await openWizard(page);
  const launch = dlg.getByRole("button", { name: "Launch" });
  await expect(launch).toBeDisabled();
  await dlg.getByLabel("Name", { exact: true }).fill(`e2e-neg-${uid()}`);
  await expect(launch).toBeDisabled(); // image still empty
  await dlg.getByLabel("Image", { exact: true }).fill(IMAGE);
  await expect(launch).toBeEnabled();
  await dlg.getByLabel("Name", { exact: true }).fill("");
  await expect(launch).toBeDisabled();
  expect(t.calls).toEqual([]); // nothing was submitted
});

test("wizard/form negative: duplicate name -> 409 shown to the user, wizard stays open, no second workload", async ({ page }) => {
  const t = track(page);
  t.allow(/POST \/api\/v1\/workloads$/);
  const name = `e2e-dup-${uid()}`;
  await seedWorkload(page, name, 1);
  const dlg = await openWizard(page);
  await fillForm(dlg, { name, image: "other/image:2" });
  const posted = waitMutation(page, "POST", /\/workloads$/);
  await dlg.getByRole("button", { name: "Launch" }).click();
  expect((await posted).status()).toBe(409);

  await toastText(page, /Launch failed/);
  await toastText(page, /already exists/);
  await expect(dlg).toBeVisible();
  const w = await findWorkload(page, name);
  expect(w.image).toBe(IMAGE); // the original was not overwritten

  await dropWorkload(page, name);
  expect(t.errors).toEqual([]);
});

test("wizard/form negative: replicas beyond the demo cap -> 400 shown to the user", async ({ page }) => {
  const t = track(page);
  t.allow(/POST \/api\/v1\/workloads$/);
  const name = `e2e-big-${uid()}`;
  const dlg = await openWizard(page);
  await fillForm(dlg, { name, image: IMAGE, replicas: 5000 });
  const posted = waitMutation(page, "POST", /\/workloads$/);
  await dlg.getByRole("button", { name: "Launch" }).click();
  expect((await posted).status()).toBe(400);
  await toastText(page, /replicas must be/);
  await expect(dlg).toBeVisible();
  expect(await findWorkload(page, name)).toBeUndefined();
});

// A Kubernetes object name must be a DNS-1123 label: the form blocks submission with an
// inline reason, and the API (demo included) rejects it if a client sends it anyway.
test("wizard/form negative: an invalid Kubernetes name is rejected with a visible error", async ({ page }) => {
  const t = track(page);
  t.allow(/POST \/api\/v1\/workloads$/);
  const name = "Bad Name!";
  try {
    const dlg = await openWizard(page);
    await fillForm(dlg, { name, image: IMAGE });
    await expect(dlg.getByText(/lowercase letters, numbers/i)).toBeVisible();
    await expect(dlg.getByRole("button", { name: "Launch" })).toBeDisabled();
    expect(await findWorkload(page, name)).toBeUndefined();
    expect(t.calls.filter((c) => c.method === "POST" && /\/workloads$/.test(c.url))).toHaveLength(0);
    // ...and the API refuses it for clients that skip the UI.
    const direct = await apiSend(page, "POST", "/workloads", { name, image: IMAGE, namespace: "default" });
    expect(direct.status).toBe(400);
    expect(await findWorkload(page, name)).toBeUndefined();
  } finally {
    await dropWorkload(page, name); // cleanup if the bug let it through
  }
});

test("wizard/intent: Preview (POST /intent/parse) shows the receipt, Place (POST /intent/place) creates it", async ({ page }) => {
  const t = track(page);
  const name = `e2e-intent-${uid()}`;
  const dlg = await openWizard(page, "Intent");
  const preview = dlg.getByRole("button", { name: "Preview" });
  const place = dlg.getByRole("button", { name: "Place" });
  await expect(preview).toBeDisabled();
  await expect(place).toBeDisabled();

  await dlg.getByPlaceholder(/small stateless api/).fill(`run ${name}, 2 copies, on staging, image ghcr.io/zyvorai/checkout:v1`);
  const parsed = waitMutation(page, "POST", /\/intent\/parse$/);
  await preview.click();
  expect((await parsed).status()).toBe(200);
  await expect(dlg.getByText(`name: ${name}`)).toBeVisible();
  await expect(dlg.getByText("replicas: 2")).toBeVisible();
  await expect(dlg.getByText("namespace: staging")).toBeVisible();
  // Preview is read-only: nothing was created yet.
  expect(await findWorkload(page, name)).toBeUndefined();

  const placed = waitMutation(page, "POST", /\/intent\/place$/);
  await place.click();
  expect((await placed).status()).toBe(201);
  await toastText(page, /Workload placed from intent/);
  await expect(dlg).toBeHidden();
  await expect(wlRow(page, name)).toBeVisible();

  const w = await findWorkload(page, name);
  expect(w).toMatchObject({ namespace: "staging", desired: 2, image: "ghcr.io/zyvorai/checkout:v1" });

  await dropWorkload(page, name, "Deployment", "staging");
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("wizard/yaml: Validate (POST /validate) gates Apply; Apply (POST /apply) outcome is surfaced honestly", async ({ page }) => {
  const t = track(page);
  const name = `e2e-yaml-${uid()}`;
  const dlg = await openWizard(page, "Validate YAML");
  const validate = dlg.getByRole("button", { name: "Validate", exact: true });
  const apply = dlg.getByRole("button", { name: "Apply", exact: true });
  await expect(validate).toBeDisabled();
  await expect(apply).toBeDisabled();

  await dlg.getByPlaceholder(/apiVersion: apps\/v1/).fill(deploymentYaml(name));
  const validated = waitMutation(page, "POST", /\/validate$/);
  await validate.click();
  expect((await validated).status()).toBe(200);
  await expect(dlg.getByText("Valid Deployment")).toBeVisible();
  await expect(apply).toBeEnabled();
  expect(await findWorkload(page, name)).toBeUndefined(); // validate never creates anything

  await apply.click();
  const applyDlg = page.getByRole("dialog", { name: "Apply workload manifest" });
  await expect(applyDlg.getByPlaceholder(/apiVersion: apps\/v1/)).toHaveValue(deploymentYaml(name));
  await applyDlg.getByRole("button", { name: /^Review/ }).click(); // Apply enables after the change preview
  const applied = waitMutation(page, "POST", /\/apply$/);
  await applyDlg.getByRole("button", { name: "Apply", exact: true }).click();
  const res = await applied;
  expect(res.status()).toBe(200);
  const body = await res.json();
  if (body.valid) {
    await toastText(page, /Applied/);
    await expect(applyDlg).toBeHidden();
    expect(await findWorkload(page, name)).toBeTruthy();
    await dropWorkload(page, name);
  } else {
    // demo mode: the failure reason is shown in the dialog and nothing is created
    await expect(applyDlg.getByText(/apply is not available in demo mode/)).toBeVisible();
    await expect(applyDlg).toBeVisible();
    expect(await findWorkload(page, name)).toBeUndefined();
  }
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("wizard/yaml negative: an invalid manifest shows the validation error and keeps Apply disabled", async ({ page }) => {
  const t = track(page);
  const dlg = await openWizard(page, "Validate YAML");
  await dlg.getByPlaceholder(/apiVersion: apps\/v1/).fill("foo: bar\nbaz: 1\n");
  const validated = waitMutation(page, "POST", /\/validate$/);
  await dlg.getByRole("button", { name: "Validate", exact: true }).click();
  expect((await validated).status()).toBe(200);
  await expect(dlg.getByText(/kind and apiVersion are required/)).toBeVisible();
  await expect(dlg.getByRole("button", { name: "Apply", exact: true })).toBeDisabled();

  // metadata.name missing is a distinct error
  await dlg.getByPlaceholder(/apiVersion: apps\/v1/).fill("apiVersion: apps/v1\nkind: Deployment\n");
  await dlg.getByRole("button", { name: "Validate", exact: true }).click();
  await expect(dlg.getByText(/metadata\.name is required/)).toBeVisible();
  await expect(dlg.getByRole("button", { name: "Apply", exact: true })).toBeDisabled();
  expect(t.find("POST", /\/apply$/)).toHaveLength(0);
  expect(t.failures()).toEqual([]);
});

// The last validation result outlives edits to the manifest, so Apply stays
// enabled for text that was never validated.
test("wizard/yaml negative: editing a validated manifest invalidates the result (Apply disabled again)", async ({ page }) => {
  const dlg = await openWizard(page, "Validate YAML");
  const box = dlg.getByPlaceholder(/apiVersion: apps\/v1/);
  await box.fill(deploymentYaml(`e2e-stale-${uid()}`));
  await dlg.getByRole("button", { name: "Validate", exact: true }).click();
  await expect(dlg.getByText("Valid Deployment")).toBeVisible();
  await box.fill("this is no longer a manifest");
  await expect(dlg.getByRole("button", { name: "Apply", exact: true })).toBeDisabled();
});

// ============================================================================
// Workloads: scale / rollout / start / stop / restart / seal / delete
// ============================================================================

test("scale: dialog posts replicas, list Desired and API agree, pods follow", async ({ page }) => {
  const t = track(page);
  const name = `e2e-scale-${uid()}`;
  await seedWorkload(page, name, 2);
  await gotoWorkloads(page);
  await expect(wlRow(page, name)).toBeVisible();

  await openWorkloadMenu(page, name, "Scale…");
  const dlg = page.getByRole("dialog", { name: `Scale · ${name}` });
  await expect(dlg.getByRole("spinbutton")).toHaveValue("2");
  await dlg.getByRole("spinbutton").fill("5");
  const posted = waitMutation(page, "POST", new RegExp(`/workloads/default/Deployment/${name}/scale$`));
  await dlg.getByRole("button", { name: "Scale", exact: true }).click();
  expect((await posted).status()).toBeLessThan(300);
  expect(t.find("POST", /\/scale$/)[0].body).toEqual({ replicas: 5 });

  await toastText(page, new RegExp(`${name} scaled to 5`));
  await expect(dlg).toBeHidden();
  await expect(wlRow(page, name).getByRole("cell").nth(4)).toHaveText("5"); // Desired
  expect((await findWorkload(page, name)).desired).toBe(5);
  expect(await findPods(page, name)).toHaveLength(5);

  // scale to zero => Stopped, no pods
  await openWorkloadMenu(page, name, "Scale…");
  await dlg.getByRole("spinbutton").fill("0");
  await dlg.getByRole("button", { name: "Scale", exact: true }).click();
  await expect(dlg).toBeHidden();
  await expect(wlRow(page, name)).toContainText("Stopped");
  expect((await findWorkload(page, name)).status).toBe("Stopped");
  expect(await findPods(page, name)).toHaveLength(0);

  await dropWorkload(page, name);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("scale negative: -1 replicas is blocked in the UI and refused (400) by the API, workload unchanged", async ({ page }) => {
  const t = track(page);
  const name = `e2e-scaleneg-${uid()}`;
  await seedWorkload(page, name, 2);
  await gotoWorkloads(page);
  await openWorkloadMenu(page, name, "Scale…");
  const dlg = page.getByRole("dialog", { name: `Scale · ${name}` });
  await dlg.getByRole("spinbutton").fill("-1");
  await expect(dlg.getByRole("button", { name: "Scale", exact: true })).toBeDisabled();
  expect(t.calls).toHaveLength(0);
  // A client that skips the UI still gets a clean 400 with the reason.
  const direct = await apiSend(page, "POST", `/workloads/default/Deployment/${name}/scale`, { replicas: -1 });
  expect(direct.status).toBe(400);
  expect(JSON.stringify(direct.data)).toMatch(/replicas must be >= 0/);
  expect((await findWorkload(page, name)).desired).toBe(2);
  await dropWorkload(page, name);
  expect(t.errors).toEqual([]);
});

// Number("") === 0, so clearing the field and pressing Scale silently scales the
// workload to zero (i.e. takes it down) instead of being rejected.
test("scale negative: an empty replicas field is rejected, not treated as 0", async ({ page }) => {
  const name = `e2e-scaleempty-${uid()}`;
  await seedWorkload(page, name, 2);
  try {
    await gotoWorkloads(page);
    await openWorkloadMenu(page, name, "Scale…");
    const dlg = page.getByRole("dialog", { name: `Scale · ${name}` });
    await dlg.getByRole("spinbutton").fill("");
    await expect(dlg.getByRole("button", { name: "Scale", exact: true })).toBeDisabled();
    expect((await findWorkload(page, name)).desired).toBe(2);
    await dlg.getByRole("spinbutton").fill("0"); // an explicit 0 is still allowed
    await expect(dlg.getByRole("button", { name: "Scale", exact: true })).toBeEnabled();
  } finally {
    await dropWorkload(page, name);
  }
});

test("rollout: restart adds a revision, pause/resume toggle status, undo rolls back", async ({ page }) => {
  const t = track(page);
  const name = `e2e-roll-${uid()}`;
  await seedWorkload(page, name, 2);
  const rolloutUrl = `/workloads/default/Deployment/${name}/rollout`;
  await gotoWorkloads(page);
  await openWorkloadMenu(page, name, "Rollout…");
  const dlg = page.getByRole("dialog", { name: `Rollout · ${name}` });
  await expect(dlg.getByText("rev 1")).toBeVisible();
  await expect(dlg.getByText("Available").first()).toBeVisible();

  const act = async (action) => {
    const p = waitMutation(page, "POST", new RegExp(`${rolloutUrl}/${action}$`));
    await dlg.getByRole("button", { name: action, exact: true }).click();
    expect((await p).status()).toBeLessThan(300);
    await toastText(page, new RegExp(`${action} requested`));
  };

  await act("restart");
  await expect(dlg.getByText("rev 2")).toBeVisible();
  let st = (await apiGet(page, rolloutUrl)).data;
  expect(st.history.map((h) => h.revision)).toEqual([2, 1]);
  expect(st.history[0].status).toBe("active");

  await act("pause");
  await expect(dlg.getByText("Paused").first()).toBeVisible();
  expect((await apiGet(page, rolloutUrl)).data.paused).toBe(true);
  await act("resume");
  await expect(dlg.getByText("Paused")).toHaveCount(0);
  expect((await apiGet(page, rolloutUrl)).data.paused).toBe(false);

  await act("undo");
  await expect(dlg.getByText("rev 2")).toHaveCount(0);
  await expect(dlg.getByText("rev 1")).toBeVisible();
  st = (await apiGet(page, rolloutUrl)).data;
  expect(st.history.map((h) => h.revision)).toEqual([1]);
  expect(st.history[0].status).toBe("active");

  await dropWorkload(page, name);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("rollout negative: undo with no previous revision reports a failure and changes nothing", async ({ page }) => {
  const t = track(page);
  t.allow(/POST \/api\/v1\/workloads\/.*\/rollout\/undo$/);
  const name = `e2e-rollneg-${uid()}`;
  await seedWorkload(page, name, 1);
  await gotoWorkloads(page);
  await openWorkloadMenu(page, name, "Rollout…");
  const dlg = page.getByRole("dialog", { name: `Rollout · ${name}` });
  const p = waitMutation(page, "POST", /\/rollout\/undo$/);
  await dlg.getByRole("button", { name: "undo", exact: true }).click();
  expect((await p).status()).toBeGreaterThanOrEqual(400);
  await toastText(page, /undo failed/);
  await toastText(page, /no previous revision/);
  await expect(dlg.getByText("rev 1")).toBeVisible();
  await dropWorkload(page, name);
  expect(t.errors).toEqual([]);
});

// "no previous revision to undo to" is a caller error, but the API answers 500,
// which clients (and monitoring) read as a server failure.
test("rollout negative: undo with no previous revision is a 4xx, not a 500", async ({ page }) => {
  const name = `e2e-roll4xx-${uid()}`;
  await seedWorkload(page, name, 1);
  const r = await apiSend(page, "POST", `/workloads/default/Deployment/${name}/rollout/undo`);
  expect(r.status).toBeGreaterThanOrEqual(400);
  expect(r.status).toBeLessThan(500);
  await dropWorkload(page, name);
});

test("start/stop/restart/seal/unseal from the row menu hit the API and change state", async ({ page }) => {
  const t = track(page);
  const name = `e2e-life-${uid()}`;
  await seedWorkload(page, name, 3);
  await gotoWorkloads(page);
  const path = (a) => new RegExp(`/workloads/default/Deployment/${name}/${a}$`);
  const menu = async (item, action) => {
    const p = waitMutation(page, "POST", path(action));
    await openWorkloadMenu(page, name, item);
    expect((await p).status()).toBeLessThan(300);
    await toastText(page, new RegExp(`${name}: ${action} requested`));
  };

  await menu("Stop", "stop");
  await expect(wlRow(page, name)).toContainText("Stopped");
  expect((await findWorkload(page, name)).desired).toBe(0);
  expect(await findPods(page, name)).toHaveLength(0);

  await menu("Start", "start");
  await expect(wlRow(page, name)).toContainText("Healthy");
  expect((await findWorkload(page, name)).desired).toBe(3); // restored to the pre-stop count
  expect(await findPods(page, name)).toHaveLength(3);

  await menu("Restart", "restart");
  expect(await findPods(page, name)).toHaveLength(3);

  // Seal goes through Still: preview GET, confirm dialog, then POST /still.
  const sealPosted = waitMutation(page, "POST", new RegExp(`/workloads/default/Deployment/${name}/still$`));
  await openWorkloadMenu(page, name, "Seal");
  await confirmDialog(page, "Seal");
  expect((await sealPosted).status()).toBeLessThan(300);
  await toastText(page, new RegExp(`${name}: seal requested`));
  expect((await findPods(page, name)).every((p) => p.status === "Sealed")).toBe(true);
  await menu("Unseal", "unseal");
  expect((await findPods(page, name)).every((p) => p.status === "Running")).toBe(true);

  await dropWorkload(page, name);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("a DaemonSet only offers Restart and Delete (no scale/start/stop/rollout/seal)", async ({ page }) => {
  const t = track(page);
  await gotoWorkloads(page);
  await page.getByRole("button", { name: "Actions for log-shipper", exact: true }).click();
  await expect(page.getByRole("menuitem", { name: "Restart" })).toBeVisible();
  await expect(page.getByRole("menuitem", { name: "Delete" })).toBeVisible();
  for (const gone of ["Start", "Stop", "Scale…", "Rollout…", "Seal", "Unseal"]) {
    await expect(page.getByRole("menuitem", { name: gone, exact: true })).toHaveCount(0);
  }
  const p = waitMutation(page, "POST", /\/workloads\/observability\/DaemonSet\/log-shipper\/restart$/);
  await page.getByRole("menuitem", { name: "Restart" }).click();
  expect((await p).status()).toBeLessThan(300);
  await toastText(page, /log-shipper: restart requested/);
  expect((await findWorkload(page, "log-shipper")).status).toBe("Healthy");
  expect(t.failures()).toEqual([]);
});

test("delete: confirm dialog gates DELETE; cancel keeps it, confirm removes it from list and API", async ({ page }) => {
  const t = track(page);
  const name = `e2e-del-${uid()}`;
  await seedWorkload(page, name, 2);
  await gotoWorkloads(page);
  await expect(wlRow(page, name)).toBeVisible();

  // cancel path: nothing is sent
  await openWorkloadMenu(page, name, "Delete");
  const dlg = page.getByRole("alertdialog");
  await expect(dlg).toContainText(`Delete Deployment ${name}?`);
  await dlg.getByRole("button", { name: "Cancel" }).click();
  await expect(dlg).toBeHidden();
  expect(t.find("DELETE", /\/workloads\//)).toHaveLength(0);
  await expect(wlRow(page, name)).toBeVisible();
  expect(await findWorkload(page, name)).toBeTruthy();

  // confirm path
  await openWorkloadMenu(page, name, "Delete");
  const del = waitMutation(page, "DELETE", new RegExp(`/workloads/default/Deployment/${name}$`));
  await confirmDialog(page);
  expect((await del).status()).toBeLessThan(300);
  await toastText(page, new RegExp(`${name} deleted`));
  await expect(wlRow(page, name)).toHaveCount(0);
  expect(await findWorkload(page, name)).toBeUndefined();
  expect(await findPods(page, name)).toHaveLength(0);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("delete negative: deleting a workload that was already removed elsewhere -> 404 toast, row disappears on refresh", async ({ page }) => {
  const t = track(page);
  t.allow(/DELETE \/api\/v1\/workloads\/.*/);
  const name = `e2e-gone-${uid()}`;
  await seedWorkload(page, name, 1);
  await gotoWorkloads(page);
  await expect(wlRow(page, name)).toBeVisible();
  await dropWorkload(page, name); // someone else deletes it; the list is now stale
  await openWorkloadMenu(page, name, "Delete");
  const del = waitMutation(page, "DELETE", /\/workloads\//);
  await confirmDialog(page);
  expect((await del).status()).toBe(404);
  await toastText(page, /Delete failed/);
  expect(t.errors).toEqual([]);
});

test("bulk: select workloads -> Restart then Delete hit every selected workload", async ({ page }) => {
  const t = track(page);
  const a = `e2e-bulk-${uid()}`;
  const b = `e2e-bulk-${uid()}`;
  await seedWorkload(page, a, 1);
  await seedWorkload(page, b, 2);
  await gotoWorkloads(page);
  await page.getByRole("checkbox", { name: `Select workload ${a}` }).click();
  await page.getByRole("checkbox", { name: `Select workload ${b}` }).click();
  await expect(page.getByText("2 selected")).toBeVisible();

  await page.getByRole("button", { name: "Restart", exact: true }).click();
  await toastText(page, /Restarted 2 workloads/);
  expect(t.find("POST", /\/restart$/).map((c) => c.url).sort()).toEqual([
    `/api/v1/workloads/default/Deployment/${a}/restart`,
    `/api/v1/workloads/default/Deployment/${b}/restart`,
  ].sort());
  await expect(page.getByText("2 selected")).toHaveCount(0); // selection cleared

  await page.getByRole("checkbox", { name: `Select workload ${a}` }).click();
  await page.getByRole("checkbox", { name: `Select workload ${b}` }).click();
  await page.getByRole("button", { name: "Delete", exact: true }).click();
  await expect(page.getByRole("alertdialog")).toContainText("Delete 2 workloads?");
  await confirmDialog(page);
  await toastText(page, /Deleted 2 workloads/);
  await expect(wlRow(page, a)).toHaveCount(0);
  await expect(wlRow(page, b)).toHaveCount(0);
  expect(await findWorkload(page, a)).toBeUndefined();
  expect(await findWorkload(page, b)).toBeUndefined();
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

// ============================================================================
// WorkloadDetail: POST /apply
// ============================================================================

test("workload detail: saving the manifest posts it to /apply and reports the result", async ({ page }) => {
  const t = track(page);
  const name = `e2e-detail-${uid()}`;
  await seedWorkload(page, name, 1);
  await page.goto(`/workloads/default/Deployment/${name}`);
  await expect(page.getByRole("heading", { name })).toBeVisible();
  await page.getByRole("tab", { name: /YAML/ }).click();
  const editor = page.getByRole("textbox").first();
  const manifest = JSON.parse(await editor.inputValue());
  expect(manifest).toMatchObject({ kind: "Deployment", metadata: { name, namespace: "default" } });
  manifest.metadata.labels = { edited: "yes" };
  await editor.fill(JSON.stringify(manifest, null, 2));

  // Save opens a "Review changes" dialog (POST /apply/diff); Apply there posts to /apply
  await page.getByRole("button", { name: "Save" }).click();
  const review = page.getByTestId("diff-review-dialog");
  await expect(review.locator('[data-diff="add"]').filter({ hasText: '"edited": "yes"' })).toBeVisible();
  const applied = waitMutation(page, "POST", /\/apply$/);
  await review.getByRole("button", { name: "Apply", exact: true }).click();
  const res = await applied;
  expect(res.status()).toBe(200);
  expect(t.find("POST", /\/apply$/)[0].body.manifest).toContain('"edited": "yes"');
  const body = await res.json();
  if (body.valid) await toastText(page, /Applied/);
  else await toastText(page, /Apply failed/); // demo: must not claim success
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: /Applied/ })).toHaveCount(body.valid ? 1 : 0);

  await dropWorkload(page, name);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("workload detail negative: a malformed manifest is not reported as applied", async ({ page }) => {
  const t = track(page);
  t.allow(/POST \/api\/v1\/apply$/);
  const name = `e2e-detailbad-${uid()}`;
  await seedWorkload(page, name, 1);
  await page.goto(`/workloads/default/Deployment/${name}`);
  await page.getByRole("tab", { name: /YAML/ }).click();
  await page.getByRole("textbox").first().fill("{ this is not a manifest");
  // the change review rejects it, so Apply never enables and nothing is posted to /apply
  await page.getByRole("button", { name: "Save" }).click();
  const review = page.getByTestId("diff-review-dialog");
  await expect(review.getByRole("alert")).toBeVisible();
  await expect(review.getByRole("button", { name: "Apply", exact: true })).toBeDisabled();
  expect(t.find("POST", /\/apply$/)).toHaveLength(0);
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: /Applied/ })).toHaveCount(0);
  await dropWorkload(page, name);
  expect(t.errors).toEqual([]);
});

// ============================================================================
// Pods
// ============================================================================

test("pods list: Evict is confirmed, POSTs /evict, and the pod disappears from UI and API", async ({ page }) => {
  const t = track(page);
  const name = `e2e-pod-${uid()}`;
  await seedWorkload(page, name, 3);
  await page.getByTestId("nav-link-pods").click();
  const victim = `${name}-02`;
  await expect(rowWith(page, victim)).toBeVisible();

  // cancel: nothing sent
  await rowWith(page, victim).getByRole("button", { name: "Evict" }).click();
  await expect(page.getByRole("alertdialog")).toContainText(`Evict pod ${victim}?`);
  await page.getByRole("alertdialog").getByRole("button", { name: "Cancel" }).click();
  expect(t.find("POST", /\/evict$/)).toHaveLength(0);

  await rowWith(page, victim).getByRole("button", { name: "Evict" }).click();
  const posted = waitMutation(page, "POST", new RegExp(`/pods/default/${victim}/evict$`));
  await confirmDialog(page, "Evict");
  expect((await posted).status()).toBeLessThan(300);
  await toastText(page, new RegExp(`Evicted ${victim}`));
  await expect(rowWith(page, victim)).toHaveCount(0);
  const pods = (await findPods(page, name)).map((p) => p.name);
  expect(pods).not.toContain(victim);
  expect(pods).toHaveLength(2);

  await dropWorkload(page, name);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("pods list negative: evicting a pod that is already gone -> 404 error toast", async ({ page }) => {
  const t = track(page);
  t.allow(/POST \/api\/v1\/pods\/.*\/evict$/);
  const name = `e2e-podgone-${uid()}`;
  await seedWorkload(page, name, 1);
  await page.getByTestId("nav-link-pods").click();
  const pod = `${name}-01`;
  await expect(rowWith(page, pod)).toBeVisible();
  await apiSend(page, "DELETE", `/pods/default/${pod}`); // row is now stale
  await rowWith(page, pod).getByRole("button", { name: "Evict" }).click();
  const posted = waitMutation(page, "POST", /\/evict$/);
  await confirmDialog(page, "Evict");
  expect((await posted).status()).toBe(404);
  await toastText(page, /Evict failed/);
  await dropWorkload(page, name);
  expect(t.errors).toEqual([]);
});

test("pods list: bulk Evict removes every selected pod", async ({ page }) => {
  const t = track(page);
  const name = `e2e-podbulk-${uid()}`;
  await seedWorkload(page, name, 3);
  await page.getByTestId("nav-link-pods").click();
  await expect(rowWith(page, `${name}-03`)).toBeVisible();
  await page.getByRole("checkbox", { name: `Select pod ${name}-01` }).click();
  await page.getByRole("checkbox", { name: `Select pod ${name}-03` }).click();
  await expect(page.getByText("2 selected")).toBeVisible();
  await page.getByText("2 selected").locator("..").getByRole("button", { name: "Evict" }).click();
  await expect(page.getByRole("alertdialog")).toContainText("Evict 2 pods?");
  await confirmDialog(page, "Evict");
  await toastText(page, /Evicted 2 pods/);
  expect(t.find("POST", /\/evict$/).map((c) => c.status)).toEqual([200, 200]);
  await expect(rowWith(page, `${name}-01`)).toHaveCount(0);
  await expect(rowWith(page, `${name}-03`)).toHaveCount(0);
  await expect(rowWith(page, `${name}-02`)).toBeVisible();
  expect((await findPods(page, name)).map((p) => p.name)).toEqual([`${name}-02`]);
  await dropWorkload(page, name);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("pods list: Exec dialog runs a command through POST /exec and shows the output", async ({ page }) => {
  const t = track(page);
  const name = `e2e-podexec-${uid()}`;
  await seedWorkload(page, name, 1);
  await page.getByTestId("nav-link-pods").click();
  await rowWith(page, `${name}-01`).getByRole("button", { name: "Exec" }).click();
  const dlg = page.getByRole("dialog", { name: `Exec · ${name}-01` });
  await dlg.getByRole("textbox").fill("whoami");
  const posted = waitMutation(page, "POST", /\/exec$/);
  await dlg.getByRole("button", { name: "Run" }).click();
  expect((await posted).status()).toBe(200);
  expect(t.find("POST", /\/exec$/)[0].body).toMatchObject({ command: "whoami" });
  await expect(dlg.getByText("axiom", { exact: true })).toBeVisible();
  await dropWorkload(page, name);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("pod detail: Evict (confirmed) POSTs /evict, returns to the list and the pod is gone", async ({ page }) => {
  const t = track(page);
  const name = `e2e-poddet-${uid()}`;
  await seedWorkload(page, name, 2);
  const pod = `${name}-01`;
  await page.goto(`/pods/default/${pod}`);
  await expect(page.getByRole("heading", { name: pod })).toBeVisible();
  await page.getByRole("button", { name: "Evict" }).click();
  const posted = waitMutation(page, "POST", new RegExp(`/pods/default/${pod}/evict$`));
  await confirmDialog(page, "Evict");
  expect((await posted).status()).toBeLessThan(300);
  await toastText(page, new RegExp(`Evicted ${pod}`));
  await expect(page).toHaveURL(/\/pods$/);
  await expect(rowWith(page, pod)).toHaveCount(0);
  expect((await findPods(page, name)).map((p) => p.name)).toEqual([`${name}-02`]);
  await dropWorkload(page, name);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("pod detail: Terminal tab one-shot exec posts to /exec; YAML Save posts to /apply and does not fake success", async ({ page }) => {
  const t = track(page);
  const name = `e2e-podyaml-${uid()}`;
  await seedWorkload(page, name, 1);
  const pod = `${name}-01`;
  await page.goto(`/pods/default/${pod}`);
  await expect(page.getByRole("heading", { name: pod })).toBeVisible();

  await page.getByRole("tab", { name: /Terminal/ }).click();
  const cmd = page.getByRole("textbox").first();
  await cmd.fill("hostname");
  const ex = waitMutation(page, "POST", /\/exec$/);
  await page.getByRole("button", { name: "Run" }).click();
  expect((await ex).status()).toBe(200);
  await expect(page.getByText(pod, { exact: true }).last()).toBeVisible();

  await page.getByRole("tab", { name: /YAML/ }).click();
  const editor = page.getByRole("textbox").first();
  const manifest = JSON.parse(await editor.inputValue());
  expect(manifest.metadata.name).toBe(pod);
  manifest.metadata.labels = { edited: "yes" };
  await editor.fill(JSON.stringify(manifest, null, 2));
  await page.getByRole("button", { name: "Save" }).click();
  const applied = waitMutation(page, "POST", /\/apply$/);
  await page.getByTestId("diff-review-dialog").getByRole("button", { name: "Apply", exact: true }).click();
  const res = await applied;
  expect(res.status()).toBe(200);
  const body = await res.json();
  await toastText(page, body.valid ? /Applied/ : /Apply failed/);
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: /Applied/ })).toHaveCount(body.valid ? 1 : 0);

  await dropWorkload(page, name);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

// ============================================================================
// Jobs & CronJobs
// ============================================================================

const cronRow = (page, name) => page.locator("tbody tr").filter({ has: page.getByRole("link", { name, exact: true }) });

test("cronjob: Run now creates a Job (POST /trigger), visible in the Jobs table and API; deleting that Job works", async ({ page }) => {
  const t = track(page);
  await page.getByTestId("nav-link-jobs").click();
  await expect(cronRow(page, "nightly-etl")).toBeVisible();
  const before = (await apiGet(page, "/jobs")).data.map((j) => j.name);

  const posted = waitMutation(page, "POST", /\/cronjobs\/default\/nightly-etl\/trigger$/);
  await cronRow(page, "nightly-etl").getByRole("button", { name: "Run now" }).click();
  expect((await posted).status()).toBeLessThan(300);
  await toastText(page, /Triggered nightly-etl/);

  const after = (await apiGet(page, "/jobs")).data;
  const created = after.filter((j) => !before.includes(j.name));
  expect(created).toHaveLength(1);
  const job = created[0];
  expect(job.ownerCronJob).toBe("nightly-etl");
  await expect(cronRow(page, job.name)).toBeVisible();
  await expect(cronRow(page, job.name)).toContainText("Running");
  expect((await apiGet(page, "/cronjobs")).data.find((c) => c.name === "nightly-etl").lastSchedule).toBe("now");

  // delete the job that the trigger created
  await cronRow(page, job.name).getByRole("button", { name: "Delete" }).click();
  await expect(page.getByRole("alertdialog")).toContainText(`Delete job ${job.name}?`);
  const del = waitMutation(page, "DELETE", new RegExp(`/jobs/default/${job.name}$`));
  await confirmDialog(page);
  expect((await del).status()).toBeLessThan(300);
  await toastText(page, new RegExp(`Deleted ${job.name}`));
  await expect(cronRow(page, job.name)).toHaveCount(0);
  expect((await apiGet(page, "/jobs")).data.map((j) => j.name)).not.toContain(job.name);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("cronjob: suspend and resume via the schedule switch (POST /suspend), UI badge and API agree", async ({ page }) => {
  const t = track(page);
  await page.getByTestId("nav-link-jobs").click();
  await expect(cronRow(page, "nightly-etl")).toBeVisible(); // table loaded (a seeded cronjob no other test deletes)
  const row = cronRow(page, "report-export");
  test.skip(!(await row.count()), "seeded cronjob report-export was deleted earlier on this demo server");
  const sw = row.getByRole("switch", { name: "Schedule enabled for report-export" });
  await expect(sw).toBeChecked();
  const suspended = async () => (await apiGet(page, "/cronjobs")).data.find((c) => c.name === "report-export").suspend;

  const p1 = waitMutation(page, "POST", /\/cronjobs\/commerce\/report-export\/suspend$/);
  await sw.click();
  expect((await p1).status()).toBeLessThan(300);
  expect(t.find("POST", /\/suspend$/)[0].body).toEqual({ suspend: true });
  await toastText(page, /Suspended report-export/);
  await expect(row).toContainText("Paused");
  await expect(sw).not.toBeChecked();
  expect(await suspended()).toBe(true);

  const p2 = waitMutation(page, "POST", /\/suspend$/);
  await sw.click();
  expect((await p2).status()).toBeLessThan(300);
  expect(t.find("POST", /\/suspend$/)[1].body).toEqual({ suspend: false });
  await toastText(page, /Resumed report-export/);
  await expect(row).toContainText("Running");
  await expect(sw).toBeChecked();
  expect(await suspended()).toBe(false);

  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("cronjob detail: Run now posts /trigger and the new Job shows under Recent jobs", async ({ page }) => {
  const t = track(page);
  await page.goto("/cronjobs/default/nightly-etl");
  await expect(page.getByRole("heading", { name: "nightly-etl" })).toBeVisible();
  const before = (await apiGet(page, "/jobs")).data.map((j) => j.name);
  const posted = waitMutation(page, "POST", /\/cronjobs\/default\/nightly-etl\/trigger$/);
  await page.getByRole("button", { name: "Run now" }).click();
  expect((await posted).status()).toBeLessThan(300);
  await toastText(page, /Triggered nightly-etl/);

  const created = (await apiGet(page, "/jobs")).data.filter((j) => !before.includes(j.name));
  expect(created).toHaveLength(1);
  await expect(page.getByRole("cell", { name: created[0].name })).toBeVisible();
  await expect(page.getByText(/Recent jobs \(\d+\)/)).toBeVisible();
  expect((await apiGet(page, "/cronjobs")).data.find((c) => c.name === "nightly-etl").lastSchedule).toBe("now");

  await apiSend(page, "DELETE", `/jobs/default/${created[0].name}`); // cleanup
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("cronjob negative: an unknown CronJob renders 'not found' with no actions, and the trigger API answers 404", async ({ page }) => {
  const t = track(page);
  t.allow(/POST \/api\/v1\/cronjobs\/.*\/trigger$/);
  await page.goto("/cronjobs/default/does-not-exist");
  await expect(page.getByText("was not found")).toBeVisible();
  await expect(page.getByRole("button", { name: "Run now" })).toHaveCount(0);
  // and the API itself refuses
  const r = await apiSend(page, "POST", "/cronjobs/default/does-not-exist/trigger");
  expect(r.status).toBe(404);
  expect(t.errors).toEqual([]);
});

test("cronjob: Delete is confirmed, sends DELETE and removes it from the list and API (acts on seeded commerce/report-export)", async ({ page }) => {
  const t = track(page);
  await page.getByTestId("nav-link-jobs").click();
  await expect(cronRow(page, "nightly-etl")).toBeVisible(); // table loaded (a seeded cronjob no other test deletes)
  const row = cronRow(page, "report-export");
  test.skip(!(await row.count()), "seeded cronjob report-export already deleted on this demo server (restart it to re-run)");

  await row.getByRole("button", { name: "Delete" }).click();
  await expect(page.getByRole("alertdialog")).toContainText("Delete CronJob report-export?");
  await page.getByRole("alertdialog").getByRole("button", { name: "Cancel" }).click();
  expect(t.find("DELETE", /\/cronjobs\//)).toHaveLength(0);
  await expect(row).toBeVisible();

  await row.getByRole("button", { name: "Delete" }).click();
  const del = waitMutation(page, "DELETE", /\/cronjobs\/commerce\/report-export$/);
  await confirmDialog(page);
  expect((await del).status()).toBeLessThan(300);
  await toastText(page, /Deleted report-export/);
  await expect(row).toHaveCount(0);
  expect((await apiGet(page, "/cronjobs")).data.map((c) => c.name)).not.toContain("report-export");
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

// ============================================================================
// Nodes (list + detail). The seeded nodes are shared and there is no create /
// delete API, so every test restores the node it touched. gpu-01 is used for
// drain because the demo places no synthetic pods on it (drain also cordons).
// ============================================================================

const nodeTile = (page, name) => page.getByTestId(`node-row-${name}`);

test("nodes list: cordon then uncordon (POST /cordon, /uncordon), badge, button and API agree", async ({ page }) => {
  const t = track(page);
  await page.getByTestId("nav-link-nodes").click();
  const tile = nodeTile(page, "axiom-02");
  await expect(tile).toBeVisible();
  await expect(tile.getByRole("button", { name: "Cordon" })).toBeVisible();

  const c = waitMutation(page, "POST", /\/nodes\/axiom-02\/cordon$/);
  await tile.getByRole("button", { name: "Cordon" }).click();
  expect((await c).status()).toBeLessThan(300);
  await toastText(page, /axiom-02: cordon requested/);
  await expect(tile.getByRole("button", { name: "Uncordon" })).toBeVisible();
  await expect(tile.getByRole("button", { name: "Cordon", exact: true })).toHaveCount(0);
  await expect(tile).toContainText("Cordoned");
  expect(await findNode(page, "axiom-02")).toMatchObject({ unschedulable: true, status: "SchedulingDisabled" });

  const u = waitMutation(page, "POST", /\/nodes\/axiom-02\/uncordon$/);
  await tile.getByRole("button", { name: "Uncordon" }).click();
  expect((await u).status()).toBeLessThan(300);
  await toastText(page, /axiom-02: uncordon requested/);
  await expect(tile.getByRole("button", { name: "Cordon", exact: true })).toBeVisible();
  await expect(tile).not.toContainText("Cordoned");
  expect((await findNode(page, "axiom-02")).status).toBe("Ready");

  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("nodes list: drain POSTs /drain, cordons the node (UI + API); uncordon restores it", async ({ page }) => {
  const t = track(page);
  await page.getByTestId("nav-link-nodes").click();
  const tile = nodeTile(page, "gpu-01");
  await expect(tile).toBeVisible();
  const d = waitMutation(page, "POST", /\/nodes\/gpu-01\/drain$/);
  await tile.getByRole("button", { name: "Drain" }).click();
  expect((await d).status()).toBeLessThan(300);
  await toastText(page, /gpu-01: drain requested/);
  await expect(tile).toContainText("Cordoned");
  await expect(tile.getByRole("button", { name: "Uncordon" })).toBeVisible();
  expect(await findNode(page, "gpu-01")).toMatchObject({ unschedulable: true });

  await tile.getByRole("button", { name: "Uncordon" }).click();
  await expect(tile.getByRole("button", { name: "Cordon", exact: true })).toBeVisible();
  expect((await findNode(page, "gpu-01")).status).toBe("Ready");
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("node detail: cordon, uncordon and drain buttons drive the same API and reflect in header + API", async ({ page }) => {
  const t = track(page);
  await page.goto("/nodes/gpu-01");
  await expect(page.getByRole("heading", { name: "gpu-01" })).toBeVisible();

  const c = waitMutation(page, "POST", /\/nodes\/gpu-01\/cordon$/);
  await page.getByRole("button", { name: "Cordon", exact: true }).click();
  expect((await c).status()).toBeLessThan(300);
  await toastText(page, /gpu-01: cordon requested/);
  await expect(page.getByRole("button", { name: "Uncordon" })).toBeVisible();
  expect((await findNode(page, "gpu-01")).unschedulable).toBe(true);

  const u = waitMutation(page, "POST", /\/nodes\/gpu-01\/uncordon$/);
  await page.getByRole("button", { name: "Uncordon" }).click();
  expect((await u).status()).toBeLessThan(300);
  await expect(page.getByRole("button", { name: "Cordon", exact: true })).toBeVisible();
  expect((await findNode(page, "gpu-01")).unschedulable).toBeFalsy();

  const d = waitMutation(page, "POST", /\/nodes\/gpu-01\/drain$/);
  await page.getByRole("button", { name: "Drain" }).click();
  expect((await d).status()).toBeLessThan(300);
  await toastText(page, /gpu-01: drain requested/);
  await expect(page.getByRole("button", { name: "Uncordon" })).toBeVisible();
  expect((await findNode(page, "gpu-01")).unschedulable).toBe(true);

  await page.getByRole("button", { name: "Uncordon" }).click(); // restore
  await expect(page.getByRole("button", { name: "Cordon", exact: true })).toBeVisible();
  expect((await findNode(page, "gpu-01")).unschedulable).toBeFalsy();
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("node detail: add a taint (POST /taints) then remove it (DELETE /taints/{key}?effect=)", async ({ page }) => {
  const t = track(page);
  const key = `e2e-${uid()}`;
  await page.goto("/nodes/axiom-03");
  await expect(page.getByRole("heading", { name: "axiom-03" })).toBeVisible();
  await page.getByRole("tab", { name: /Taints/ }).click();

  const add = page.getByRole("button", { name: "Add", exact: true });
  await expect(add).toBeDisabled(); // negative: no key yet
  await page.getByLabel("Key", { exact: true }).fill(key);
  await page.getByLabel("Value", { exact: true }).fill("gpu");
  await expect(add).toBeEnabled();
  const posted = waitMutation(page, "POST", /\/nodes\/axiom-03\/taints$/);
  await add.click();
  expect((await posted).status()).toBeLessThan(300);
  expect(t.find("POST", /\/taints$/)[0].body).toEqual({ key, value: "gpu", effect: "NoSchedule" });
  await toastText(page, /Taint added to axiom-03/);
  await expect(page.getByText(`${key}=gpu:NoSchedule`)).toBeVisible();
  await expect(page.getByLabel("Key", { exact: true })).toHaveValue(""); // form reset
  expect((await findNode(page, "axiom-03")).taints).toContainEqual({ key, value: "gpu", effect: "NoSchedule" });

  await page.getByRole("button", { name: `Remove taint ${key}` }).click();
  const del = waitMutation(page, "DELETE", new RegExp(`/nodes/axiom-03/taints/${key}$`));
  await confirmDialog(page, "Remove");
  const res = await del;
  expect(res.status()).toBeLessThan(300);
  expect(new URL(res.url()).searchParams.get("effect")).toBe("NoSchedule");
  await toastText(page, /Taint removed from axiom-03/);
  await expect(page.getByText(`${key}=gpu:NoSchedule`)).toHaveCount(0);
  expect(((await findNode(page, "axiom-03")).taints || []).map((x) => x.key)).not.toContain(key);
  expect(t.failures()).toEqual([]);
  expect(t.errors).toEqual([]);
});

test("nodes negative: cordon/drain against a node that does not exist -> 404, and no node is created or changed", async ({ page }) => {
  const before = (await apiGet(page, "/nodes")).data.map((n) => `${n.name}:${n.status}`);
  for (const action of ["cordon", "uncordon", "drain"]) {
    const r = await apiSend(page, "POST", `/nodes/no-such-node/${action}`);
    expect(r.status).toBe(404);
  }
  expect((await apiGet(page, "/nodes")).data.map((n) => `${n.name}:${n.status}`)).toEqual(before);
  // There is no node-delete endpoint at all (nodes are cordoned/drained, not deleted, from Axiom).
  const del = await apiSend(page, "DELETE", "/nodes/gpu-01");
  expect([404, 405]).toContain(del.status);
  expect(await findNode(page, "gpu-01")).toBeTruthy();
});
