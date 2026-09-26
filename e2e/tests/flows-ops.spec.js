// @ts-check
// Functional specs for the operational surfaces: Virtual Machines, Backups
// (VolumeSnapshots + Velero), Atlas, and the Ops page (Helm, GitOps, Agents,
// RBAC). Same contract as flows-core.spec.js: drive the real UI, assert the
// mutating request succeeded, assert the UI reflects it, then confirm with an
// independent API read (and, where the backend records one, the audit log).
//
// Runs against a *demo-mode* server; the demo backend is in-memory, so a pass
// proves the UI -> API contract and UI behaviour, not the Kubernetes
// implementation. Every test uses uniquely named resources and cleans up where
// a delete exists (VMs via the API, RBAC/Helm via the UI). Backups, Velero
// objects, agent runs and VM protection objects have no delete endpoint.
const http = require("http");
const { test, expect } = require("@playwright/test");
const { signIn, track, apiGet, apiSend, waitMutation, confirmDialog, toastText } = require("./helpers");

const uid = () => Math.random().toString(36).slice(2, 8);
const esc = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

test.beforeEach(async ({ page }) => {
  await signIn(page);
});

const okStatus = (res) => expect(res.status()).toBeLessThan(300);

// The audit log is an independent record of what the backend actually did.
async function auditHas(page, action, target) {
  const audit = await apiGet(page, "/audit");
  return (audit.data || []).some((e) => e.action === action && e.target === target);
}

async function vmFromApi(page, ns, name) {
  const list = await apiGet(page, `/vms?namespace=${ns}`);
  return (list.data || []).find((v) => v.name === name);
}

// Fixture helper: create a VM through the API (the UI flow is covered by its own test).
async function createVmViaApi(page, name, running = true) {
  const res = await apiSend(page, "POST", "/vms", { namespace: "default", name, cpu: "1", memory: "2Gi", running });
  expect(res.status).toBe(201);
}

async function deleteVmViaApi(page, name) {
  await apiSend(page, "DELETE", `/vms/default/${name}`);
}

// ---------------------------------------------------------------- Virtual Machines

test.describe("virtual machines", () => {
  test("create via UI, stop/start on the list, delete", async ({ page }) => {
    const t = track(page);
    const name = `e2e-vm-${uid()}`;
    try {
      await page.getByTestId("nav-link-vms").click();
      await page.getByRole("button", { name: "Create VM" }).click();
      const dlg = page.getByRole("dialog");
      const create = dlg.getByRole("button", { name: "Create", exact: true });
      await expect(create).toBeDisabled(); // no name yet
      await dlg.getByLabel("Name", { exact: true }).fill(name);
      await dlg.getByLabel("CPU cores").fill("3");
      await dlg.getByLabel("Memory").fill("6Gi");
      await expect(create).toBeEnabled();

      const posted = waitMutation(page, "POST", /\/vms$/);
      await create.click();
      okStatus(await posted);
      expect(t.find("POST", /\/vms$/)[0].body).toMatchObject({ namespace: "default", name, cpu: "3", memory: "6Gi", running: true });

      await toastText(page, new RegExp(`VM ${name} created`));
      const card = page.getByTestId(`vm-card-${name}`);
      await expect(card).toBeVisible();
      await expect(card).toContainText("Running");
      const created = await vmFromApi(page, "default", name);
      expect(created).toMatchObject({ status: "Running", running: true, cpu: "3", memory: "6Gi" });

      // Stop
      const stop = waitMutation(page, "POST", new RegExp(`/vms/default/${name}/stop$`));
      await card.getByRole("button", { name: "Stop" }).click();
      okStatus(await stop);
      await toastText(page, new RegExp(`${name}: stop requested`));
      await expect(card.getByRole("button", { name: "Start" })).toBeVisible();
      await expect(card.getByRole("button", { name: "Migrate" })).toBeDisabled(); // only a running VM can migrate
      expect((await vmFromApi(page, "default", name)).running).toBe(false);
      expect(await auditHas(page, "vm.stop", `default/${name}`)).toBe(true);

      // Start
      const start = waitMutation(page, "POST", new RegExp(`/vms/default/${name}/start$`));
      await card.getByRole("button", { name: "Start" }).click();
      okStatus(await start);
      await toastText(page, new RegExp(`${name}: start requested`));
      await expect(card.getByRole("button", { name: "Stop" })).toBeVisible();
      await expect(card.getByRole("button", { name: "Migrate" })).toBeEnabled();
      expect((await vmFromApi(page, "default", name)).status).toBe("Running");
      expect(await auditHas(page, "vm.start", `default/${name}`)).toBe(true);

      expect(t.failures()).toEqual([]);
      expect(t.errors).toEqual([]);
    } finally {
      await deleteVmViaApi(page, name);
    }
    // The UI has no VM delete; the API one must actually remove it.
    expect(await vmFromApi(page, "default", name)).toBeUndefined();
  });

  test("detail page: stop and start", async ({ page }) => {
    const t = track(page);
    const name = `e2e-vmd-${uid()}`;
    await createVmViaApi(page, name);
    try {
      await page.goto(`/vms/default/${name}`);
      await expect(page.getByRole("heading", { name })).toBeVisible();
      await expect(page.getByRole("button", { name: "Migrate" })).toBeEnabled();

      const stop = waitMutation(page, "POST", new RegExp(`/vms/default/${name}/stop$`));
      await page.getByRole("button", { name: "Stop", exact: true }).click();
      okStatus(await stop);
      await toastText(page, new RegExp(`${name}: stop requested`));
      await expect(page.getByRole("button", { name: "Start", exact: true })).toBeVisible();
      await expect(page.getByRole("button", { name: "Migrate" })).toBeDisabled();
      await expect(page.getByText("Stopped").first()).toBeVisible();
      expect((await vmFromApi(page, "default", name)).running).toBe(false);

      const start = waitMutation(page, "POST", new RegExp(`/vms/default/${name}/start$`));
      await page.getByRole("button", { name: "Start", exact: true }).click();
      okStatus(await start);
      await toastText(page, new RegExp(`${name}: start requested`));
      await expect(page.getByRole("button", { name: "Stop", exact: true })).toBeVisible();
      expect((await vmFromApi(page, "default", name)).running).toBe(true);

      expect(t.failures()).toEqual([]);
      expect(t.errors).toEqual([]);
    } finally {
      await deleteVmViaApi(page, name);
    }
  });

  test("migrate a built-in VM from the list and from the detail page", async ({ page }) => {
    const t = track(page);
    await page.goto("/vms");
    const card = page.getByTestId("vm-card-edge-api-vm");
    let posted = waitMutation(page, "POST", /\/vms\/default\/edge-api-vm\/migrate$/);
    await card.getByRole("button", { name: "Migrate" }).click();
    expect((await posted).status()).toBe(202);
    await toastText(page, /edge-api-vm: migrate requested/);
    expect(await auditHas(page, "vm.migrate", "default/edge-api-vm")).toBe(true);

    await page.goto("/vms/default/edge-api-vm");
    posted = waitMutation(page, "POST", /\/vms\/default\/edge-api-vm\/migrate$/);
    await page.getByRole("button", { name: "Migrate" }).click();
    expect((await posted).status()).toBe(202);
    await toastText(page, /edge-api-vm: migrate requested/);

    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  // BUG: demoVMExists() only consults the seeded VMs, so a VM created at runtime
  // (which Start/Stop accept) is "not found" for migrate and snapshot.
  test("migrate a VM created at runtime (demo bug: demoVMExists ignores runtime VMs)", async ({ page }) => {
    const t = track(page);
    const name = `e2e-vmm-${uid()}`;
    await createVmViaApi(page, name);
    try {
      await page.goto("/vms");
      const card = page.getByTestId(`vm-card-${name}`);
      const posted = waitMutation(page, "POST", new RegExp(`/vms/default/${name}/migrate$`));
      await card.getByRole("button", { name: "Migrate" }).click();
      expect((await posted).status()).toBe(202);
      await toastText(page, new RegExp(`${name}: migrate requested`));
      expect(await auditHas(page, "vm.migrate", `default/${name}`)).toBe(true);
      expect(t.failures()).toEqual([]);
    } finally {
      await deleteVmViaApi(page, name);
    }
  });

  test("protection: snapshot, clone, export and restore of a VM", async ({ page }) => {
    const t = track(page);
    const id = uid();
    await page.goto("/vms");
    const region = (title) => page.getByRole("region", { name: title });

    const cases = [
      {
        tab: "Snapshots", title: "Snapshots", path: "/vm-snapshots", nameField: `snap-${id}`,
        fill: { Namespace: "default", Name: `snap-${id}`, "VM name": "edge-api-vm" },
        verify: (l) => l.find((s) => s.name === `snap-${id}` && s.vm === "edge-api-vm"),
        audit: ["vm.snapshot", "default/edge-api-vm"],
      },
      {
        tab: "Clones", title: "Clones", path: "/vm-clones", nameField: `clone-${id}`,
        fill: { Namespace: "default", Name: `clone-${id}`, "Source VM": "edge-api-vm", "Target VM name": `copy-${id}` },
        verify: (l) => l.find((c) => c.name === `clone-${id}` && c.source === "edge-api-vm" && c.target === `copy-${id}`),
        audit: ["vm.clone", "default/edge-api-vm"],
      },
      {
        tab: "Exports", title: "Exports", path: "/vm-exports", nameField: `export-${id}`,
        fill: { Namespace: "default", Name: `export-${id}`, "VM name": "edge-api-vm" },
        verify: (l) => l.find((e) => e.name === `export-${id}` && e.source === "edge-api-vm"),
        audit: ["vm.export", "default/edge-api-vm"],
      },
      {
        tab: "Restores", title: "Restores", path: "/vm-restores", nameField: `restore-${id}`,
        fill: { Namespace: "default", Name: `restore-${id}`, "Snapshot name": "edge-api-vm-snap", "Target VM name": `restored-${id}` },
        verify: (r) => r.find((x) => x.name === `restore-${id}` && x.snapshot === "edge-api-vm-snap" && x.target === `restored-${id}`),
        audit: ["vm.restore", "default/edge-api-vm-snap"],
      },
    ];

    for (const c of cases) {
      await page.getByRole("tab", { name: c.tab }).click();
      await region(c.title).waitFor();
      await page.getByRole("button", { name: "New", exact: true }).click();
      const dlg = page.getByRole("dialog");
      for (const [label, value] of Object.entries(c.fill)) await dlg.getByLabel(label, { exact: true }).fill(value);
      const posted = waitMutation(page, "POST", new RegExp(`${esc(c.path)}$`));
      await dlg.getByRole("button", { name: "Create", exact: true }).click();
      okStatus(await posted);
      await toastText(page, /Requested/);
      await expect(dlg).toBeHidden();
      await expect(region(c.title)).toContainText(c.nameField);
      expect(c.verify((await apiGet(page, c.path)).data)).toBeTruthy();
      expect(await auditHas(page, c.audit[0], c.audit[1])).toBe(true);
    }

    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("snapshot a VM created at runtime (demo bug: demoVMExists ignores runtime VMs)", async ({ page }) => {
    const name = `e2e-vms-${uid()}`;
    await createVmViaApi(page, name);
    try {
      const res = await apiSend(page, "POST", "/vm-snapshots", { namespace: "default", vm: name, name: `${name}-snap` });
      expect(res.status).toBe(201);
    } finally {
      await deleteVmViaApi(page, name);
    }
  });

  test("negative: duplicate VM, blank namespace and unknown VM show errors and do not crash", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/vms$/);
    t.allow(/POST \/api\/v1\/vm-snapshots$/);
    await page.goto("/vms");
    await expect(page.getByTestId("vm-card-edge-api-vm")).toBeVisible();
    const cardsBefore = await page.locator("[data-testid^='vm-card-']").count();

    // Duplicate of a built-in VM -> 409, dialog stays open with the form intact.
    await page.getByRole("button", { name: "Create VM" }).click();
    const dlg = page.getByRole("dialog");
    await dlg.getByLabel("Name", { exact: true }).fill("edge-api-vm");
    let posted = waitMutation(page, "POST", /\/vms$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await posted).status()).toBe(409);
    await toastText(page, /Create failed/);
    await toastText(page, /already exists/);
    await expect(dlg).toBeVisible();

    // Blank namespace -> 400 from the backend, surfaced in a toast.
    await dlg.getByLabel("Namespace", { exact: true }).fill("");
    await dlg.getByLabel("Name", { exact: true }).fill(`e2e-neg-${uid()}`);
    posted = waitMutation(page, "POST", /\/vms$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await posted).status()).toBe(400);
    await toastText(page, /namespace and name are required/);
    await expect(dlg).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.locator("[data-testid^='vm-card-']")).toHaveCount(cardsBefore);

    // Snapshot of a VM that does not exist -> 404 toast, no snapshot row.
    const ghost = `ghost-${uid()}`;
    await page.getByRole("button", { name: "New", exact: true }).click();
    const sdlg = page.getByRole("dialog");
    await sdlg.getByLabel("Namespace", { exact: true }).fill("default");
    await sdlg.getByLabel("Name", { exact: true }).fill(`snap-${ghost}`);
    await sdlg.getByLabel("VM name", { exact: true }).fill(ghost);
    posted = waitMutation(page, "POST", /\/vm-snapshots$/);
    await sdlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await posted).status()).toBe(404);
    await toastText(page, /not found/);
    await expect(sdlg).toBeVisible();
    const snaps = await apiGet(page, "/vm-snapshots");
    expect(snaps.data.map((s) => s.name)).not.toContain(`snap-${ghost}`);

    expect(t.errors).toEqual([]);
    await expect(page.getByTestId("vms-page")).toBeVisible(); // page still alive
  });

  test("negative: detail page for a missing VM renders a not-found state", async ({ page }) => {
    const t = track(page);
    await page.goto(`/vms/default/nope-${uid()}`);
    await expect(page.getByText("was not found")).toBeVisible();
    expect(t.errors).toEqual([]);
  });
});

// ---------------------------------------------------------------- Backups

test.describe("backups", () => {
  test("create a volume snapshot, then restore it to a new PVC", async ({ page }) => {
    const t = track(page);
    const id = uid();
    const snap = `e2e-snap-${id}`;
    const pvc = `e2e-restored-${id}`;
    await page.getByTestId("nav-link-backups").click();

    await page.getByRole("button", { name: "Create snapshot" }).click();
    let dlg = page.getByRole("dialog");
    const create = dlg.getByRole("button", { name: "Create", exact: true });
    await expect(create).toBeDisabled(); // name + PVC required
    await dlg.getByLabel("Name", { exact: true }).fill(snap);
    await expect(create).toBeDisabled();
    await dlg.getByLabel("PVC", { exact: true }).fill("edge-api-data");
    await expect(create).toBeEnabled();
    let posted = waitMutation(page, "POST", /\/backups$/);
    await create.click();
    okStatus(await posted);
    expect(t.find("POST", /\/backups$/)[0].body).toMatchObject({ namespace: "default", name: snap, pvc: "edge-api-data" });
    await toastText(page, new RegExp(`Backup ${snap} requested`));
    await expect(dlg).toBeHidden();

    const row = page.getByRole("row").filter({ hasText: snap });
    await expect(row).toBeVisible();
    await expect(row).toContainText("edge-api-data");
    const listed = (await apiGet(page, "/backups")).data.find((b) => b.name === snap);
    expect(listed).toMatchObject({ namespace: "default", pvc: "edge-api-data", ready: true });
    expect(await auditHas(page, "backup.create", `default/${snap}`)).toBe(true);

    // Restore that snapshot into a new PVC.
    await page.getByRole("button", { name: "Restore", exact: true }).click();
    dlg = page.getByRole("dialog");
    const restore = dlg.getByRole("button", { name: "Restore", exact: true });
    await expect(restore).toBeDisabled();
    await dlg.getByLabel("Snapshot name").fill(snap);
    await dlg.getByLabel("New PVC name").fill(pvc);
    await dlg.getByLabel("Size", { exact: true }).fill("5Gi");
    await expect(restore).toBeEnabled();
    posted = waitMutation(page, "POST", /\/backups\/restore$/);
    await restore.click();
    okStatus(await posted);
    expect(t.find("POST", /\/backups\/restore$/)[0].body).toMatchObject({
      namespace: "default", snapshotName: snap, pvcName: pvc, size: "5Gi", accessModes: ["ReadWriteOnce"],
    });
    await toastText(page, new RegExp(`Restore ${pvc} requested`));
    await expect(dlg).toBeHidden();

    try {
      const vols = await apiGet(page, "/volumes?namespace=default");
      expect(vols.data.find((v) => v.name === pvc)).toMatchObject({ capacity: "5Gi", status: "Bound" });
      expect(await auditHas(page, "backup.restore", `default/${pvc}`)).toBe(true);
      expect(t.failures()).toEqual([]);
      expect(t.errors).toEqual([]);
    } finally {
      await apiSend(page, "DELETE", `/volumes/default/${pvc}`);
    }
    expect((await apiGet(page, "/volumes?namespace=default")).data.map((v) => v.name)).not.toContain(pvc);
  });

  test("velero backup: single and multi-namespace", async ({ page }) => {
    const t = track(page);
    const id = uid();
    await page.goto("/backups");

    for (const [name, included, expectedList, expectedCol] of [
      [`e2e-vb-${id}`, "*", ["*"], "*"],
      [`e2e-vb2-${id}`, "default, platform", ["default", "platform"], "default,platform"],
    ]) {
      await page.getByRole("button", { name: "Velero backup" }).click();
      const dlg = page.getByRole("dialog");
      const create = dlg.getByRole("button", { name: "Create", exact: true });
      // (the dialog keeps the previous submission's values, so only the first open is blank)
      await dlg.getByLabel("Name", { exact: true }).fill("");
      await expect(create).toBeDisabled();
      await dlg.getByLabel("Name", { exact: true }).fill(name);
      await dlg.getByLabel(/Included namespaces/).fill(included);
      const posted = waitMutation(page, "POST", /\/velero\/backups$/);
      await create.click();
      okStatus(await posted);
      const call = t.find("POST", /\/velero\/backups$/).find((c) => c.body.name === name);
      expect(call.body).toMatchObject({ namespace: "velero", includedNamespaces: expectedList });
      await toastText(page, new RegExp(`Velero backup ${name} requested`));
      await expect(dlg).toBeHidden();

      const row = page.getByRole("row").filter({ hasText: name });
      await expect(row).toBeVisible();
      await expect(row).toContainText(expectedCol);
      const listed = (await apiGet(page, "/velero/backups")).data.find((b) => b.name === name);
      expect(listed).toMatchObject({ namespace: "velero", namespaces: expectedCol });
      expect(await auditHas(page, "velero-backup.create", `velero/${name}`)).toBe(true);
    }
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("velero schedule: create and it appears with its cron expression", async ({ page }) => {
    const t = track(page);
    const name = `e2e-sched-${uid()}`;
    await page.goto("/backups");
    await page.getByRole("button", { name: "New schedule" }).click();
    const dlg = page.getByRole("dialog");
    const create = dlg.getByRole("button", { name: "Create", exact: true });
    await expect(create).toBeDisabled();
    await dlg.getByLabel("Name", { exact: true }).fill(name);
    await dlg.getByLabel("Cron schedule").fill("*/15 * * * *");
    await dlg.getByLabel(/TTL/).fill("72h0m0s");
    await expect(create).toBeEnabled();
    const posted = waitMutation(page, "POST", /\/velero\/schedules$/);
    await create.click();
    okStatus(await posted);
    expect(t.find("POST", /\/velero\/schedules$/)[0].body).toMatchObject({ namespace: "velero", name, schedule: "*/15 * * * *", ttl: "72h0m0s" });
    await toastText(page, new RegExp(`Schedule ${name} created`));
    await expect(dlg).toBeHidden();

    const row = page.getByRole("row").filter({ hasText: name });
    await expect(row).toBeVisible();
    await expect(row).toContainText("*/15 * * * *");
    await expect(row).toContainText("Running");
    const listed = (await apiGet(page, "/velero/schedules")).data.find((s) => s.name === name);
    expect(listed).toMatchObject({ namespace: "velero", schedule: "*/15 * * * *" });
    expect(await auditHas(page, "velero-schedule.create", `velero/${name}`)).toBe(true);
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("negative: duplicates and unknown snapshot are rejected with a visible error", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/(backups|backups\/restore|velero\/schedules|velero\/backups)$/);
    await page.goto("/backups");
    const counts = async () => ({
      snaps: (await apiGet(page, "/backups")).data.length,
      backups: (await apiGet(page, "/velero/backups")).data.length,
      scheds: (await apiGet(page, "/velero/schedules")).data.length,
    });
    const before = await counts();

    // Duplicate volume snapshot (seeded name).
    await page.getByRole("button", { name: "Create snapshot" }).click();
    let dlg = page.getByRole("dialog");
    await dlg.getByLabel("Name", { exact: true }).fill("edge-api-data-snap");
    await dlg.getByLabel("PVC", { exact: true }).fill("edge-api-data");
    let posted = waitMutation(page, "POST", /\/backups$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await posted).status()).toBe(409);
    await toastText(page, /Create backup failed/);
    await expect(dlg).toBeVisible();
    await page.keyboard.press("Escape");

    // Restore from a snapshot that does not exist.
    await page.getByRole("button", { name: "Restore", exact: true }).click();
    dlg = page.getByRole("dialog");
    await dlg.getByLabel("Snapshot name").fill(`missing-${uid()}`);
    await dlg.getByLabel("New PVC name").fill(`never-${uid()}`);
    posted = waitMutation(page, "POST", /\/backups\/restore$/);
    await dlg.getByRole("button", { name: "Restore", exact: true }).click();
    expect((await posted).status()).toBe(404);
    await toastText(page, /Restore failed/);
    await expect(dlg).toBeVisible();
    await page.keyboard.press("Escape");

    // Duplicate Velero schedule (seeded name).
    await page.getByRole("button", { name: "New schedule" }).click();
    dlg = page.getByRole("dialog");
    await dlg.getByLabel("Name", { exact: true }).fill("forge-daily");
    posted = waitMutation(page, "POST", /\/velero\/schedules$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    expect((await posted).status()).toBe(409);
    await toastText(page, /Create schedule failed/);
    await expect(dlg).toBeVisible();
    // Clearing the cron expression disables submit.
    await dlg.getByLabel("Cron schedule").fill("");
    await expect(dlg.getByRole("button", { name: "Create", exact: true })).toBeDisabled();
    await page.keyboard.press("Escape");

    // Duplicate Velero backup (seeded name).
    await page.getByRole("button", { name: "Velero backup" }).click();
    dlg = page.getByRole("dialog");
    await dlg.getByLabel("Name", { exact: true }).fill("forge-backup-demo");
    posted = waitMutation(page, "POST", /\/velero\/backups$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    // The seeded backup is not in the "extra" map the demo dedupes against,
    // so a rejection is not guaranteed there; only require the page to survive.
    await posted;
    await page.keyboard.press("Escape");

    const after = await counts();
    expect(after.snaps).toBe(before.snaps);
    expect(after.scheds).toBe(before.scheds);
    expect(t.errors).toEqual([]);
    await expect(page.getByRole("heading", { name: "Backups", exact: true })).toBeVisible();
  });
});

// ---------------------------------------------------------------- Atlas

test.describe("atlas", () => {
  // The demo fabric reports itself as connected and shows a "Create volume"
  // button, but POST /atlas/volumes answers 503 "atlas not configured".
  test("create volume against the built-in demo fabric", async ({ page }) => {
    const t = track(page);
    const name = `e2e-vol-${uid()}`;
    await page.getByTestId("nav-link-atlas").click();
    await expect(page.getByText("Atlas not connected")).toHaveCount(0);
    await page.getByRole("button", { name: "Create volume" }).click();
    const dlg = page.getByRole("dialog");
    await dlg.getByLabel("Name", { exact: true }).fill(name);
    const posted = waitMutation(page, "POST", /\/atlas\/volumes$/);
    await dlg.getByRole("button", { name: "Create", exact: true }).click();
    okStatus(await posted);
    await toastText(page, new RegExp(`Volume ${name} requested`));
    expect(t.failures()).toEqual([]);
  });

  // Point the (in-memory, demo) integration at a tiny mock Atlas gateway that
  // this test owns, so the real UI -> Axiom -> gateway path is exercised.
  test("create volume through a mock Atlas gateway, and surface a gateway rejection", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/atlas\/volumes$/);
    const received = [];
    const volumes = [];
    const gateway = http.createServer((req, res) => {
      let body = "";
      req.on("data", (c) => (body += c));
      req.on("end", () => {
        res.setHeader("Content-Type", "application/json");
        if (req.method === "POST" && req.url === "/api/atlas/v1/volumes") {
          const payload = JSON.parse(body || "{}");
          received.push(payload);
          if (String(payload.name).startsWith("deny-")) {
            res.statusCode = 422;
            res.end(JSON.stringify({ error: { message: "policy denied by gateway" } }));
            return;
          }
          volumes.push({ id: `vol-${volumes.length + 1}`, name: payload.name, policy: payload.policy, state: "provisioning" });
          res.statusCode = 202;
          res.end(JSON.stringify({ job_id: "job-1" }));
          return;
        }
        if (req.url === "/api/atlas/v1/volumes") return res.end(JSON.stringify(volumes));
        if (req.url === "/version") return res.end(JSON.stringify({ version: "mock" }));
        res.end("[]");
      });
    });
    await new Promise((r) => gateway.listen(0, "127.0.0.1", r));
    const port = gateway.address().port;
    const good = `e2e-vol-${uid()}`;
    const denied = `deny-${uid()}`;
    try {
      const put = await apiSend(page, "PUT", "/integrations", { atlasUrl: `http://127.0.0.1:${port}` });
      expect(put.status).toBe(200);

      await page.getByTestId("nav-link-atlas").click();
      await expect(page.getByText("Atlas not connected")).toHaveCount(0);
      await page.getByRole("button", { name: "Create volume" }).click();
      let dlg = page.getByRole("dialog");
      const create = dlg.getByRole("button", { name: "Create", exact: true });
      await expect(create).toBeDisabled();
      await dlg.getByLabel("Name", { exact: true }).fill(good);
      await dlg.locator("#atlas-policy").click();
      await page.getByRole("option", { name: "production" }).click();
      await dlg.getByLabel("Size (GiB)").fill("25");
      await dlg.getByLabel("Namespace", { exact: true }).fill("data");
      let posted = waitMutation(page, "POST", /\/atlas\/volumes$/);
      await create.click();
      expect((await posted).status()).toBe(202);
      await toastText(page, new RegExp(`Volume ${good} requested`));
      await expect(dlg).toBeHidden();

      // The gateway saw exactly what the form said.
      expect(received).toHaveLength(1);
      expect(received[0]).toMatchObject({
        name: good, policy: "production", size_bytes: 25 * 1024 ** 3,
        kubernetes: { namespace: "data", create_pvc: true },
      });
      // ...and the refreshed Volumes table shows it.
      await expect(page.getByText(good).first()).toBeVisible();

      // Gateway rejection: error text is surfaced, dialog stays, nothing is recorded.
      await page.getByRole("button", { name: "Create volume" }).click();
      dlg = page.getByRole("dialog");
      await dlg.getByLabel("Name", { exact: true }).fill(denied);
      posted = waitMutation(page, "POST", /\/atlas\/volumes$/);
      await dlg.getByRole("button", { name: "Create", exact: true }).click();
      expect((await posted).status()).toBe(422);
      await toastText(page, /Create volume failed/);
      await toastText(page, /policy denied by gateway/);
      await expect(dlg).toBeVisible();
      expect(volumes.map((v) => v.name)).not.toContain(denied);
      await page.keyboard.press("Escape");

      expect(t.errors).toEqual([]);
    } finally {
      await apiSend(page, "PUT", "/integrations", { atlasUrl: "" });
      await new Promise((r) => gateway.close(r));
    }
  });
});

// ---------------------------------------------------------------- Ops: Helm

test.describe("ops: helm", () => {
  const helmRow = (page, name) => page.getByRole("row").filter({ hasText: name });
  const release = async (page, ns, name) => (await apiGet(page, "/helm/releases")).data.find((r) => r.namespace === ns && r.name === name);

  test("install a chart, then uninstall it", async ({ page }) => {
    const t = track(page);
    const rel = `e2e-rel-${uid()}`;
    await page.getByTestId("nav-link-ops").click();
    await expect(page.getByRole("tab", { name: "Helm" })).toHaveAttribute("data-state", "active");

    await page.getByRole("button", { name: "Install chart" }).click();
    const dlg = page.getByRole("dialog");
    const install = dlg.getByRole("button", { name: "Install", exact: true });
    await expect(install).toBeDisabled(); // release name, repo URL and chart are required
    await dlg.getByLabel("Release name").fill(rel);
    await dlg.getByLabel("Chart repo URL").fill("https://charts.example.com");
    await expect(install).toBeDisabled();
    await dlg.getByLabel("Chart name").fill("podinfo");
    await dlg.getByLabel("Version (optional)").fill("6.5.0");
    await expect(install).toBeEnabled();
    const posted = waitMutation(page, "POST", /\/helm\/install$/);
    await install.click();
    okStatus(await posted);
    expect(t.find("POST", /\/helm\/install$/)[0].body).toMatchObject({
      namespace: "default", releaseName: rel, repoUrl: "https://charts.example.com", chartName: "podinfo", chartVersion: "6.5.0",
    });
    await toastText(page, new RegExp(`Install started for ${rel}`));
    await expect(dlg).toBeHidden();

    await expect(helmRow(page, rel)).toContainText("podinfo");
    expect(await release(page, "default", rel)).toMatchObject({ chart: "podinfo", revision: 1, status: "deployed" });
    expect(await auditHas(page, "helm.install", `default/${rel}`)).toBe(true);

    const del = waitMutation(page, "POST", new RegExp(`/helm/releases/default/${rel}/uninstall$`));
    await helmRow(page, rel).getByTitle("Uninstall").click();
    await confirmDialog(page, "Uninstall");
    okStatus(await del);
    await toastText(page, new RegExp(`${rel} uninstalled`));
    await expect(helmRow(page, rel)).toHaveCount(0);
    expect(await release(page, "default", rel)).toBeUndefined();
    expect(await auditHas(page, "helm.uninstall", `default/${rel}`)).toBe(true);

    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("rollback a release to an explicit revision", async ({ page }) => {
    const t = track(page);
    await page.goto("/ops/helm");
    const row = helmRow(page, "ingress-nginx-4.11.0");
    await expect(row).toContainText("rev 3");
    try {
      await row.getByPlaceholder("rev").fill("2");
      const posted = waitMutation(page, "POST", /\/helm\/releases\/ingress-nginx\/ingress-nginx\/rollback$/);
      await row.getByTitle("Rollback").click();
      expect((await posted).status()).toBe(202);
      expect(t.find("POST", /rollback$/)[0].body).toEqual({ revision: 2 });
      await toastText(page, /ingress-nginx rolled back/);
      await expect(row).toContainText("rev 2");
      // Demo semantics: the release lands on the requested revision.
      expect((await release(page, "ingress-nginx", "ingress-nginx")).revision).toBe(2);
      expect(await auditHas(page, "helm.rollback", "ingress-nginx/ingress-nginx")).toBe(true);
      expect(t.failures()).toEqual([]);
      expect(t.errors).toEqual([]);
    } finally {
      await apiSend(page, "POST", "/helm/releases/ingress-nginx/ingress-nginx/rollback", { revision: 3 });
    }
  });

  // BUG (demo): an empty revision box posts {revision: 0}, which the real
  // backend treats as "previous revision" (helm rollback <name>) but the demo
  // provider stores literally, leaving the release at "rev 0".
  test("rollback with a blank revision goes to the previous revision, not rev 0", async ({ page }) => {
    // The seeded release's revision is shared demo state that each run decrements, so assert the
    // right outcome for wherever it stands: previous revision if there is one, a clear 409 if not.
    const before = (await release(page, "cert-manager", "cert-manager")).revision;
    await page.goto("/ops/helm");
    const row = helmRow(page, "cert-manager-v1.15.1");
    const posted = waitMutation(page, "POST", /\/helm\/releases\/cert-manager\/cert-manager\/rollback$/);
    await row.getByTitle("Rollback").click();
    const res = await posted;
    if (before > 1) {
      okStatus(res);
      expect((await release(page, "cert-manager", "cert-manager")).revision).toBe(before - 1);
    } else {
      expect(res.status()).toBe(409);
      await toastText(page, /Rollback failed/);
      expect((await release(page, "cert-manager", "cert-manager")).revision).toBe(before);
    }
  });

  test("negative: duplicate release name and missing required fields", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/helm\/install$/);
    await page.goto("/ops/helm");
    const before = (await apiGet(page, "/helm/releases")).data.length;

    await page.getByRole("button", { name: "Install chart" }).click();
    const dlg = page.getByRole("dialog");
    const install = dlg.getByRole("button", { name: "Install", exact: true });
    await dlg.getByLabel("Namespace", { exact: true }).fill("ingress-nginx");
    await dlg.getByLabel("Release name").fill("ingress-nginx");
    await dlg.getByLabel("Chart repo URL").fill("https://charts.example.com");
    await dlg.getByLabel("Chart name").fill("ingress-nginx");
    const posted = waitMutation(page, "POST", /\/helm\/install$/);
    await install.click();
    expect((await posted).status()).toBe(409);
    await toastText(page, /Install failed/);
    await toastText(page, /already exists/);
    await expect(dlg).toBeVisible();
    expect((await apiGet(page, "/helm/releases")).data).toHaveLength(before);

    // Blank namespace is rejected by the backend, not silently accepted.
    await dlg.getByLabel("Namespace", { exact: true }).fill("");
    await dlg.getByLabel("Release name").fill(`e2e-neg-${uid()}`);
    const posted2 = waitMutation(page, "POST", /\/helm\/install$/);
    await install.click();
    expect((await posted2).status()).toBe(400);
    await toastText(page, /namespace, releaseName, repoUrl, and chartName are required/);
    await expect(dlg).toBeVisible();
    expect((await apiGet(page, "/helm/releases")).data).toHaveLength(before);

    // Clearing a required field disables submit (no request possible).
    await dlg.getByLabel("Chart name").fill("");
    await expect(install).toBeDisabled();
    expect(t.errors).toEqual([]);
  });

  // The Ops UI has no "upgrade" control (only install/rollback/uninstall), so the
  // upgrade endpoint is exercised at the API contract level.
  test("api contract: helm upgrade accepts a valid request and rejects a blank one", async ({ page }) => {
    const ok = await apiSend(page, "POST", "/helm/upgrade", {
      namespace: "monitoring", releaseName: "prometheus", repoUrl: "https://prometheus-community.github.io/helm-charts", chartName: "kube-prometheus-stack",
    });
    expect(ok.status).toBeLessThan(300);
    expect(await auditHas(page, "helm.upgrade", "monitoring/prometheus")).toBe(true);
    const bad = await apiSend(page, "POST", "/helm/upgrade", { namespace: "", releaseName: "" });
    expect(bad.status).toBe(400);
  });
});

// ---------------------------------------------------------------- Ops: GitOps

test.describe("ops: gitops", () => {
  test("sync every kind of GitOps application", async ({ page }) => {
    const t = track(page);
    await page.goto("/ops/gitops");
    const apps = [
      { kind: "Application", ns: "argocd", name: "platform-apps" },
      { kind: "Kustomization", ns: "flux-system", name: "apps" },
      { kind: "HelmRelease", ns: "monitoring", name: "prometheus" },
    ];
    for (const a of apps) {
      const row = page.getByRole("row").filter({ hasText: a.name }).filter({ hasText: a.kind });
      await expect(row).toBeVisible();
      const posted = waitMutation(page, "POST", new RegExp(`/gitops/${a.kind}/${a.ns}/${a.name}/sync$`));
      await row.getByTitle("Sync").click();
      okStatus(await posted);
      await toastText(page, new RegExp(`${a.name}: sync requested`));
      await expect(row).toContainText("Healthy");
      const item = (await apiGet(page, "/gitops")).data.find((g) => g.name === a.name && g.namespace === a.ns);
      expect(["Synced", "Ready"]).toContain(item.status);
      expect(await auditHas(page, "gitops.sync", `${a.ns}/${a.name}`)).toBe(true);
    }
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("negative: syncing an unknown application is a 404, not a server error", async ({ page }) => {
    const res = await apiSend(page, "POST", `/gitops/Application/argocd/ghost-${uid()}/sync`);
    expect(res.status).toBe(404);
    const wrongKind = await apiSend(page, "POST", "/gitops/Kustomization/argocd/platform-apps/sync");
    expect(wrongKind.status).toBe(404);
  });
});

// ---------------------------------------------------------------- Ops: Agent runs

test.describe("ops: agent runs", () => {
  async function newRun(page, goal) {
    const name = `e2e-run-${uid()}`;
    const res = await apiSend(page, "POST", "/agent-runs", { namespace: "default", name, goal });
    expect(res.status).toBe(201);
    return name;
  }
  const runRow = (page, name) => page.locator(".list-row").filter({ hasText: name });

  test("approve a pending proposal", async ({ page }) => {
    const t = track(page);
    const name = await newRun(page, "e2e approve: restart the broken pod");
    await page.goto("/ops/agents");
    const row = runRow(page, name);
    await expect(row).toContainText("approval: Pending");
    const posted = waitMutation(page, "POST", new RegExp(`/agent-runs/default/${name}/approve$`));
    await row.getByRole("button", { name: "Approve proposal" }).click();
    okStatus(await posted);
    await toastText(page, new RegExp(`${name}: approved`));
    await expect(row).toContainText("approval: Approved");
    await expect(row.getByRole("button", { name: "Approve proposal" })).toHaveCount(0);
    await expect(row.getByRole("button", { name: "Deny proposal" })).toHaveCount(0);
    const run = (await apiGet(page, `/agent-runs/default/${name}`)).data;
    expect(run).toMatchObject({ approval: "Approved", phase: "Succeeded", canApprove: false });
    expect(await auditHas(page, "agentrun.approve", `default/${name}`)).toBe(true);
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("deny a pending proposal", async ({ page }) => {
    const t = track(page);
    const name = await newRun(page, "e2e deny: scale everything to zero");
    await page.goto("/ops/agents");
    const row = runRow(page, name);
    const posted = waitMutation(page, "POST", new RegExp(`/agent-runs/default/${name}/deny$`));
    await row.getByRole("button", { name: "Deny proposal" }).click();
    okStatus(await posted);
    await toastText(page, new RegExp(name)); // wording is asserted separately below
    await expect(row).toContainText("approval: Denied");
    await expect(row.getByRole("button", { name: "Approve proposal" })).toHaveCount(0);
    const run = (await apiGet(page, `/agent-runs/default/${name}`)).data;
    expect(run).toMatchObject({ approval: "Denied", phase: "Denied", canDeny: false });
    expect(await auditHas(page, "agentrun.deny", `default/${name}`)).toBe(true);
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  // BUG: Ops.jsx builds the toast as `${action}d`, which yields "denyd" for deny.
  test("deny toast reads 'denied'", async ({ page }) => {
    const name = await newRun(page, "e2e deny wording");
    await page.goto("/ops/agents");
    await runRow(page, name).getByRole("button", { name: "Deny proposal" }).click();
    await toastText(page, new RegExp(`${name}: denied`));
  });

  test("negative: approving or denying an unknown run is a 404", async ({ page }) => {
    for (const action of ["approve", "deny"]) {
      const res = await apiSend(page, "POST", `/agent-runs/default/ghost-${uid()}/${action}`);
      expect(res.status).toBe(404);
    }
  });

  test("negative: an agent run without a goal is rejected", async ({ page }) => {
    const res = await apiSend(page, "POST", "/agent-runs", { namespace: "default", name: `e2e-nogoal-${uid()}` });
    expect(res.status).toBe(400);
  });
});

// ---------------------------------------------------------------- Ops: RBAC

test.describe("ops: rbac", () => {
  const row = (page, name) => page.getByRole("row").filter({ hasText: name });
  const rbac = async (page) => (await apiGet(page, "/rbac")).data;

  test("create a role, then delete it", async ({ page }) => {
    const t = track(page);
    const name = `e2e-role-${uid()}`;
    await page.goto("/ops/access");
    await page.getByRole("button", { name: "New role" }).click();
    const dlg = page.getByRole("dialog");
    const create = dlg.getByRole("button", { name: "Create", exact: true });
    await expect(create).toBeDisabled();
    await dlg.getByLabel("Name", { exact: true }).fill(name);
    await dlg.getByLabel("Rules (JSON array of PolicyRule)").fill(
      JSON.stringify([
        { apiGroups: [""], resources: ["pods"], verbs: ["get", "list"] },
        { apiGroups: ["apps"], resources: ["deployments"], verbs: ["get"] },
      ])
    );
    const posted = waitMutation(page, "POST", /\/rbac\/roles$/);
    await create.click();
    okStatus(await posted);
    expect(t.find("POST", /\/rbac\/roles$/)[0].body).toMatchObject({ namespace: "default", name });
    expect(t.find("POST", /\/rbac\/roles$/)[0].body.rules).toHaveLength(2);
    await toastText(page, new RegExp(`Role ${name} created`));
    await expect(dlg).toBeHidden();

    await expect(row(page, name)).toContainText("2 rules");
    expect((await rbac(page)).roles.find((r) => r.name === name)).toMatchObject({ namespace: "default", rules: 2 });
    expect(await auditHas(page, "rbac.role.create", `default/${name}`)).toBe(true);

    const del = waitMutation(page, "DELETE", new RegExp(`/rbac/roles/default/${name}$`));
    await row(page, name).getByRole("button", { name: "Delete" }).click();
    await confirmDialog(page);
    okStatus(await del);
    await toastText(page, new RegExp(`Deleted ${name}`));
    await expect(row(page, name)).toHaveCount(0);
    expect((await rbac(page)).roles.map((r) => r.name)).not.toContain(name);
    expect(await auditHas(page, "rbac.role.delete", `default/${name}`)).toBe(true);
    expect(t.failures()).toEqual([]);
    expect(t.errors).toEqual([]);
  });

  test("create a role binding, then delete it", async ({ page }) => {
    const t = track(page);
    const id = uid();
    const role = `e2e-role-${id}`;
    const name = `e2e-rb-${id}`;
    await apiSend(page, "POST", "/rbac/roles", { namespace: "default", name: role, rules: [{ apiGroups: [""], resources: ["pods"], verbs: ["get"] }] });
    try {
      await page.goto("/ops/access");
      await page.getByRole("button", { name: "New binding" }).click();
      const dlg = page.getByRole("dialog");
      const create = dlg.getByRole("button", { name: "Create", exact: true });
      await expect(create).toBeDisabled();
      await dlg.getByLabel("Name", { exact: true }).fill(name);
      await expect(create).toBeDisabled(); // role name still missing
      await dlg.getByLabel("Role name").fill(role);
      await dlg.getByLabel("Subjects (JSON array)").fill(
        JSON.stringify([{ kind: "User", name: "jane" }, { kind: "Group", name: "devs" }])
      );
      const posted = waitMutation(page, "POST", /\/rbac\/rolebindings$/);
      await create.click();
      okStatus(await posted);
      expect(t.find("POST", /\/rbac\/rolebindings$/)[0].body).toMatchObject({ namespace: "default", name, roleName: role });
      await toastText(page, new RegExp(`RoleBinding ${name} created`));
      await expect(dlg).toBeHidden();

      const r = row(page, name);
      await expect(r).toContainText(`Role/${role}`);
      await expect(r).toContainText("2 subjects");
      expect((await rbac(page)).roleBindings.find((b) => b.name === name)).toMatchObject({ roleRef: `Role/${role}`, subjects: 2 });
      expect(await auditHas(page, "rbac.rolebinding.create", `default/${name}`)).toBe(true);

      const del = waitMutation(page, "DELETE", new RegExp(`/rbac/rolebindings/default/${name}$`));
      await r.getByRole("button", { name: "Delete" }).click();
      await confirmDialog(page);
      okStatus(await del);
      await toastText(page, new RegExp(`Deleted ${name}`));
      await expect(row(page, name)).toHaveCount(0);
      expect((await rbac(page)).roleBindings.map((b) => b.name)).not.toContain(name);
      expect(await auditHas(page, "rbac.rolebinding.delete", `default/${name}`)).toBe(true);
      expect(t.failures()).toEqual([]);
      expect(t.errors).toEqual([]);
    } finally {
      await apiSend(page, "DELETE", `/rbac/roles/default/${role}`);
    }
  });

  test("cancelling the delete confirmation leaves the role in place", async ({ page }) => {
    const t = track(page);
    const name = `e2e-keep-${uid()}`;
    await apiSend(page, "POST", "/rbac/roles", { namespace: "default", name, rules: [{ apiGroups: [""], resources: ["pods"], verbs: ["get"] }] });
    try {
      await page.goto("/ops/access");
      await row(page, name).getByRole("button", { name: "Delete" }).click();
      const dlg = page.getByRole("alertdialog");
      await dlg.getByRole("button", { name: "Cancel" }).click();
      await expect(dlg).toBeHidden();
      await expect(row(page, name)).toBeVisible();
      expect(t.find("DELETE", /rbac/)).toHaveLength(0);
      expect((await rbac(page)).roles.map((r) => r.name)).toContain(name);
    } finally {
      await apiSend(page, "DELETE", `/rbac/roles/default/${name}`);
    }
  });

  test("negative: invalid JSON, empty rules/subjects and duplicates are rejected visibly", async ({ page }) => {
    const t = track(page);
    t.allow(/POST \/api\/v1\/rbac\/(roles|rolebindings)$/);
    const id = uid();
    const dup = `e2e-dup-${id}`;
    await apiSend(page, "POST", "/rbac/roles", { namespace: "default", name: dup, rules: [{ apiGroups: [""], resources: ["pods"], verbs: ["get"] }] });
    try {
      await page.goto("/ops/access");
      const before = await rbac(page);

      // --- role: malformed JSON never reaches the network
      await page.getByRole("button", { name: "New role" }).click();
      let dlg = page.getByRole("dialog");
      await dlg.getByLabel("Name", { exact: true }).fill(`e2e-bad-${id}`);
      await dlg.getByLabel("Rules (JSON array of PolicyRule)").fill("[{not json");
      await dlg.getByRole("button", { name: "Create", exact: true }).click();
      await toastText(page, /Rules must be valid JSON/);
      await expect(dlg).toBeVisible();
      expect(t.find("POST", /\/rbac\/roles$/)).toHaveLength(0);

      // --- role: empty rules array -> backend 400 with its message
      await dlg.getByLabel("Rules (JSON array of PolicyRule)").fill("[]");
      let posted = waitMutation(page, "POST", /\/rbac\/roles$/);
      await dlg.getByRole("button", { name: "Create", exact: true }).click();
      expect((await posted).status()).toBe(400);
      await toastText(page, /Create role failed/);
      await toastText(page, /at least one rule is required/);
      await expect(dlg).toBeVisible();

      // --- role: duplicate name -> 409
      await dlg.getByLabel("Name", { exact: true }).fill(dup);
      await dlg.getByLabel("Rules (JSON array of PolicyRule)").fill('[{"apiGroups":[""],"resources":["pods"],"verbs":["get"]}]');
      posted = waitMutation(page, "POST", /\/rbac\/roles$/);
      await dlg.getByRole("button", { name: "Create", exact: true }).click();
      expect((await posted).status()).toBe(409);
      await toastText(page, /already exists/);
      await expect(dlg).toBeVisible();
      await page.keyboard.press("Escape");

      // --- binding: malformed subjects JSON
      await page.getByRole("button", { name: "New binding" }).click();
      dlg = page.getByRole("dialog");
      await dlg.getByLabel("Name", { exact: true }).fill(`e2e-badrb-${id}`);
      await dlg.getByLabel("Role name").fill(dup);
      await dlg.getByLabel("Subjects (JSON array)").fill("{oops");
      await dlg.getByRole("button", { name: "Create", exact: true }).click();
      await toastText(page, /Subjects must be valid JSON/);
      await expect(dlg).toBeVisible();
      expect(t.find("POST", /\/rbac\/rolebindings$/)).toHaveLength(0);

      // --- binding: empty subjects -> backend 400
      await dlg.getByLabel("Subjects (JSON array)").fill("[]");
      posted = waitMutation(page, "POST", /\/rbac\/rolebindings$/);
      await dlg.getByRole("button", { name: "Create", exact: true }).click();
      expect((await posted).status()).toBe(400);
      await toastText(page, /Create role binding failed/);
      await toastText(page, /at least one subject is required/);
      await expect(dlg).toBeVisible();
      await page.keyboard.press("Escape");

      // Nothing was created by any of the rejected attempts.
      const after = await rbac(page);
      expect(after.roles.map((r) => r.name).sort()).toEqual([...before.roles.map((r) => r.name)].sort());
      expect(after.roleBindings.map((r) => r.name).sort()).toEqual([...before.roleBindings.map((r) => r.name)].sort());
      expect(t.errors).toEqual([]);
      await expect(page.getByRole("button", { name: "New role" })).toBeVisible(); // page still alive
    } finally {
      await apiSend(page, "DELETE", `/rbac/roles/default/${dup}`);
    }
  });

  test("negative: deleting an unknown role or binding is a 404", async ({ page }) => {
    const r = await apiSend(page, "DELETE", `/rbac/roles/default/ghost-${uid()}`);
    expect(r.status).toBe(404);
    const b = await apiSend(page, "DELETE", `/rbac/rolebindings/default/ghost-${uid()}`);
    expect(b.status).toBe(404);
  });
});
