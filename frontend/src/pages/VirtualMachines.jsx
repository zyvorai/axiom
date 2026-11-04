import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { Cpu, MemoryStick, X, Play, Square, ArrowRightLeft, Terminal, Copy, Check, Camera, Copy as CopyIcon, Upload, RotateCcw, Plus } from "lucide-react";
import { VMS } from "@/constants/testIds";
import { toast } from "sonner";

// Toast keys per action id (the ids themselves are API paths and stay English).
const ACTION_TOASTS = {
    stop: { ok: "toasts:vms.stopRequested", fail: "toasts:vms.stopFailed" },
    start: { ok: "toasts:vms.startRequested", fail: "toasts:vms.startFailed" },
    migrate: { ok: "toasts:vms.migrateRequested", fail: "toasts:vms.migrateFailed" },
};

function ConsolePanel({ vm }) {
    const { t } = useTranslation();
    const [info, setInfo] = useState(null);
    const [copied, setCopied] = useState(false);

    useEffect(() => {
        let alive = true;
        api
            .get(`/vms/${vm.namespace}/${encodeURIComponent(vm.name)}/console`)
            .then((r) => alive && setInfo(r.data))
            .catch(() => alive && setInfo(false));
        return () => {
            alive = false;
        };
    }, [vm]);

    if (info === null) return <Skeleton className="h-40 rounded-xl bg-muted" />;
    if (info === false) return <div className="rounded-xl border border-border p-6 text-center text-sm text-muted-foreground">{t("pages:vms.console.unavailable")}</div>;

    const copy = () => {
        navigator.clipboard?.writeText(info.virtctl);
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
    };

    return (
        <div className="space-y-4 rounded-xl border border-border bg-muted p-6">
            <div className="flex items-center gap-3">
                <div className="grid h-10 w-10 place-items-center rounded-xl bg-muted">
                    <Terminal className="h-4 w-4 text-tone-link" />
                </div>
                <div>
                    <div className="text-sm font-medium">{info.ready ? t("pages:vms.console.ready") : t("pages:vms.console.notReady")}</div>
                    <div className="text-xs text-muted-foreground">
                        {t("pages:vms.console.hint")}
                    </div>
                </div>
            </div>
            {info.wsPath && (
                <div>
                    <div className="mb-1 text-[11px] uppercase tracking-widest text-muted-foreground">{t("pages:vms.console.wsProxy")}</div>
                    <code className="mono block truncate text-[11px] text-tone-green">{info.wsPath}</code>
                </div>
            )}
            <div>
                <div className="mb-1 text-[11px] uppercase tracking-widest text-muted-foreground">{t("pages:vms.console.runLocally")}</div>
                <div className="flex items-center gap-2 rounded-lg border border-border bg-muted px-3 py-2">
                    <code className="mono flex-1 truncate text-xs text-tone-green">{info.virtctl}</code>
                    <button onClick={copy} aria-label={t("common:actions.copy")} title={t("common:actions.copy")} className="text-muted-foreground hover:text-foreground">
                        {copied ? <Check className="h-3.5 w-3.5 text-tone-green" /> : <Copy className="h-3.5 w-3.5" />}
                    </button>
                </div>
            </div>
            <div>
                <div className="mb-1 text-[11px] uppercase tracking-widest text-muted-foreground">{t("pages:vms.console.subresource")}</div>
                <code className="mono block truncate text-[11px] text-muted-foreground">{info.vncPath}</code>
            </div>
        </div>
    );
}

function ProtectionSection({ title, newTitle, icon: Icon, endpoint, columns, formFields, emptyHint }) {
    const { t } = useTranslation();
    const [items, setItems] = useState(null);
    const [open, setOpen] = useState(false);
    const [form, setForm] = useState({});
    const [busy, setBusy] = useState(false);

    const load = () => api.get(endpoint).then((r) => setItems(r.data));

    useEffect(() => {
        let alive = true;
        api.get(endpoint).then((r) => alive && setItems(r.data));
        return () => {
            alive = false;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [endpoint]);

    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const submit = async () => {
        setBusy(true);
        try {
            await api.post(endpoint, form);
            toast.success(t("toasts:vms.requested"));
            setOpen(false);
            setForm({});
            load();
        } catch (e) {
            toast.error(t("toasts:vms.createFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <GlassCard className="overflow-hidden">
            <div className="flex items-center justify-between border-b px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                    <Icon className="h-4 w-4 text-tone-link" /> {title}
                </div>
                <Button size="sm" variant="brand" onClick={() => setOpen(true)} className="rounded-xl">
                    <Plus className="mr-1 h-3.5 w-3.5" /> {t("pages:vms.protection.new")}
                </Button>
            </div>
            <div className="max-h-[360px] overflow-auto scrollbar-thin" tabIndex={0} role="region" aria-label={title}>
                {!items && <Skeleton className="m-4 h-24 rounded-xl" />}
                {items && items.length === 0 && (
                    <div className="px-4 py-8 text-center text-sm text-muted-foreground">{emptyHint}</div>
                )}
                {items &&
                    items.map((it, i) => (
                        <div
                            key={i}
                            className="grid grid-cols-12 items-center gap-3 border-b px-4 py-3 last:border-none hover:bg-muted"
                        >
                            {columns.map((c) => (
                                <div key={c.key} className={c.span || "col-span-3"}>
                                    {c.render ? c.render(it) : <span className="mono text-xs text-muted-foreground">{String(it[c.key] ?? "—")}</span>}
                                </div>
                            ))}
                        </div>
                    ))}
            </div>

            <Dialog open={open} onOpenChange={setOpen}>
                <DialogContent className="max-w-md">
                    <DialogHeader>
                        <DialogTitle>{newTitle}</DialogTitle>
                    </DialogHeader>
                    <div className="space-y-3">
                        {formFields.map((f) => (
                            <div key={f.key} className="space-y-1.5">
                                <Label htmlFor={`vm-field-${f.key}`} className="text-xs text-muted-foreground">{f.label}</Label>
                                <Input
                                    id={`vm-field-${f.key}`}
                                    value={form[f.key] || ""}
                                    onChange={(e) => set({ [f.key]: e.target.value })}
                                    placeholder={f.placeholder}
                                />
                            </div>
                        ))}
                    </div>
                    <DialogFooter>
                        <Button variant="brand" onClick={submit} disabled={busy} className="rounded-xl">
                            {busy ? t("dialogs:vms.creating") : t("common:actions.create")}
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </GlassCard>
    );
}

function VmProtection() {
    const { t } = useTranslation();
    return (
        <Tabs defaultValue="snapshots">
            <TabsList className="tabs !h-auto !rounded-none !bg-transparent !p-0">
                <TabsTrigger value="snapshots" className="gap-1.5">
                    <Camera className="h-3.5 w-3.5" /> {t("pages:vms.protection.snapshots.title")}
                </TabsTrigger>
                <TabsTrigger value="clones" className="gap-1.5">
                    <CopyIcon className="h-3.5 w-3.5" /> {t("pages:vms.protection.clones.title")}
                </TabsTrigger>
                <TabsTrigger value="exports" className="gap-1.5">
                    <Upload className="h-3.5 w-3.5" /> {t("pages:vms.protection.exports.title")}
                </TabsTrigger>
                <TabsTrigger value="restores" className="gap-1.5">
                    <RotateCcw className="h-3.5 w-3.5" /> {t("pages:vms.protection.restores.title")}
                </TabsTrigger>
            </TabsList>
            <TabsContent value="snapshots" className="mt-3">
                <ProtectionSection
                    title={t("pages:vms.protection.snapshots.title")}
                    newTitle={t("pages:vms.protection.snapshots.new")}
                    icon={Camera}
                    endpoint="/vm-snapshots"
                    emptyHint={t("pages:vms.protection.snapshots.empty")}
                    columns={[
                        {
                            key: "name",
                            span: "col-span-6",
                            render: (r) => (
                                <>
                                    <div className="mono truncate text-sm">{r.name}</div>
                                    <div className="text-[11px] text-muted-foreground">
                                        {r.namespace} · {r.vm}
                                    </div>
                                </>
                            ),
                        },
                        { key: "status", span: "col-span-4", render: (r) => <StatusBadge status={r.ready ? "Ready" : r.status} /> },
                        { key: "age", span: "col-span-2", render: (r) => <span className="mono text-right text-xs text-muted-foreground block">{r.age}</span> },
                    ]}
                    formFields={[
                        { key: "namespace", label: t("common:fields.namespace"), placeholder: "default" },
                        { key: "name", label: t("common:fields.name"), placeholder: "snap-1" },
                        { key: "vm", label: t("pages:vms.protection.fields.vmName"), placeholder: "edge-api-vm" },
                    ]}
                />
            </TabsContent>
            <TabsContent value="clones" className="mt-3">
                <ProtectionSection
                    title={t("pages:vms.protection.clones.title")}
                    newTitle={t("pages:vms.protection.clones.new")}
                    icon={CopyIcon}
                    endpoint="/vm-clones"
                    emptyHint={t("pages:vms.protection.clones.empty")}
                    columns={[
                        {
                            key: "name",
                            span: "col-span-5",
                            render: (r) => (
                                <>
                                    <div className="mono truncate text-sm">{r.name}</div>
                                    <div className="text-[11px] text-muted-foreground">{r.namespace}</div>
                                </>
                            ),
                        },
                        { key: "source", span: "col-span-3", render: (r) => <span className="mono text-xs text-muted-foreground">{r.source}</span> },
                        { key: "target", span: "col-span-2", render: (r) => <span className="mono text-xs text-muted-foreground">{r.target}</span> },
                        { key: "phase", span: "col-span-1", render: (r) => <StatusBadge status={r.phase} /> },
                        { key: "age", span: "col-span-1", render: (r) => <span className="mono text-right text-xs text-muted-foreground block">{r.age}</span> },
                    ]}
                    formFields={[
                        { key: "namespace", label: t("common:fields.namespace"), placeholder: "default" },
                        { key: "name", label: t("common:fields.name"), placeholder: "clone-1" },
                        { key: "sourceVM", label: t("pages:vms.protection.fields.sourceVm"), placeholder: "edge-api-vm" },
                        { key: "targetVM", label: t("pages:vms.protection.fields.targetVmName"), placeholder: "edge-api-vm-clone" },
                    ]}
                />
            </TabsContent>
            <TabsContent value="exports" className="mt-3">
                <ProtectionSection
                    title={t("pages:vms.protection.exports.title")}
                    newTitle={t("pages:vms.protection.exports.new")}
                    icon={Upload}
                    endpoint="/vm-exports"
                    emptyHint={t("pages:vms.protection.exports.empty")}
                    columns={[
                        {
                            key: "name",
                            span: "col-span-6",
                            render: (r) => (
                                <>
                                    <div className="mono truncate text-sm">{r.name}</div>
                                    <div className="text-[11px] text-muted-foreground">
                                        {r.namespace} · {r.source}
                                    </div>
                                </>
                            ),
                        },
                        { key: "phase", span: "col-span-4", render: (r) => <StatusBadge status={r.phase} /> },
                        { key: "age", span: "col-span-2", render: (r) => <span className="mono text-right text-xs text-muted-foreground block">{r.age}</span> },
                    ]}
                    formFields={[
                        { key: "namespace", label: t("common:fields.namespace"), placeholder: "default" },
                        { key: "name", label: t("common:fields.name"), placeholder: "export-1" },
                        { key: "vm", label: t("pages:vms.protection.fields.vmName"), placeholder: "edge-api-vm" },
                    ]}
                />
            </TabsContent>
            <TabsContent value="restores" className="mt-3">
                <ProtectionSection
                    title={t("pages:vms.protection.restores.title")}
                    newTitle={t("pages:vms.protection.restores.new")}
                    icon={RotateCcw}
                    endpoint="/vm-restores"
                    emptyHint={t("pages:vms.protection.restores.empty")}
                    columns={[
                        {
                            key: "name",
                            span: "col-span-5",
                            render: (r) => (
                                <>
                                    <div className="mono truncate text-sm">{r.name}</div>
                                    <div className="text-[11px] text-muted-foreground">{r.namespace}</div>
                                </>
                            ),
                        },
                        { key: "snapshot", span: "col-span-3", render: (r) => <span className="mono text-xs text-muted-foreground">{r.snapshot}</span> },
                        { key: "target", span: "col-span-2", render: (r) => <span className="mono text-xs text-muted-foreground">{r.target}</span> },
                        { key: "phase", span: "col-span-1", render: (r) => <StatusBadge status={r.phase} /> },
                        { key: "age", span: "col-span-1", render: (r) => <span className="mono text-right text-xs text-muted-foreground block">{r.age}</span> },
                    ]}
                    formFields={[
                        { key: "namespace", label: t("common:fields.namespace"), placeholder: "default" },
                        { key: "name", label: t("common:fields.name"), placeholder: "restore-1" },
                        { key: "snapshot", label: t("pages:vms.protection.fields.snapshotName"), placeholder: "snap-1" },
                        { key: "targetVM", label: t("pages:vms.protection.fields.targetVmName"), placeholder: "edge-api-vm-restored" },
                    ]}
                />
            </TabsContent>
        </Tabs>
    );
}

export default function VirtualMachines() {
    const { t } = useTranslation();
    const [vms, setVms] = useState(null);
    const [openVm, setOpenVm] = useState(null);
    const [busy, setBusy] = useState("");
    const [createOpen, setCreateOpen] = useState(false);
    const [createForm, setCreateForm] = useState({
        namespace: "default",
        name: "",
        cpu: "1",
        memory: "2Gi",
        image: "quay.io/containerdisks/ubuntu:22.04",
    });

    const load = () => api.get("/vms").then((r) => setVms(r.data));

    useEffect(() => {
        let alive = true;
        const tick = () => api.get("/vms").then((r) => alive && setVms(r.data));
        tick();
        const timer = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    const act = async (vm, action, path) => {
        const key = `${vm.namespace}/${vm.name}:${action}`;
        setBusy(key);
        try {
            await api.post(`/vms/${vm.namespace}/${encodeURIComponent(vm.name)}/${path}`);
            toast.success(t(ACTION_TOASTS[action].ok, { name: vm.name }));
            await load();
        } catch (e) {
            toast.error(t(ACTION_TOASTS[action].fail), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    if (!vms) {
        return (
            <div data-testid={VMS.root} className="page">
                <div className="grid g3">
                    {[...Array(6)].map((_, i) => (
                        <Skeleton key={i} className="h-64 rounded-2xl" />
                    ))}
                </div>
            </div>
        );
    }

    return (
        <div data-testid={VMS.root} className="page">
            <PageHeader
                title={t("pages:vms.title")}
                description={t("pages:vms.description")}
                action={
                    <>
                        <span className="text-[13px] text-ink-3">{t("pages:vms.vmCount", { n: vms.length })}</span>
                        <button type="button" className="btn btn-fill" onClick={() => setCreateOpen(true)}>
                            <Plus size={15} /> {t("pages:vms.createVm")}
                        </button>
                    </>
                }
            />

            <Dialog open={createOpen} onOpenChange={setCreateOpen}>
                <DialogContent className="max-w-md">
                    <DialogHeader>
                        <DialogTitle>{t("dialogs:vms.create.title")}</DialogTitle>
                    </DialogHeader>
                    <div className="space-y-3">
                        <div className="space-y-1.5">
                            <Label htmlFor="virtual-machines-namespace" className="text-xs text-muted-foreground">{t("common:fields.namespace")}</Label>
                            <Input id="virtual-machines-namespace" className="bg-muted/30" value={createForm.namespace} onChange={(e) => setCreateForm((f) => ({ ...f, namespace: e.target.value }))} />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="virtual-machines-name" className="text-xs text-muted-foreground">{t("common:fields.name")}</Label>
                            <Input id="virtual-machines-name" className="bg-muted/30" value={createForm.name} onChange={(e) => setCreateForm((f) => ({ ...f, name: e.target.value }))} placeholder="web-vm" />
                        </div>
                        <div className="grid grid-cols-2 gap-3">
                            <div className="space-y-1.5">
                                <Label htmlFor="virtual-machines-cpu-cores" className="text-xs text-muted-foreground">{t("dialogs:vms.create.cpuCores")}</Label>
                                <Input id="virtual-machines-cpu-cores" className="bg-muted/30" value={createForm.cpu} onChange={(e) => setCreateForm((f) => ({ ...f, cpu: e.target.value }))} />
                            </div>
                            <div className="space-y-1.5">
                                <Label htmlFor="virtual-machines-memory" className="text-xs text-muted-foreground">{t("common:fields.memory")}</Label>
                                <Input id="virtual-machines-memory" className="bg-muted/30" value={createForm.memory} onChange={(e) => setCreateForm((f) => ({ ...f, memory: e.target.value }))} />
                            </div>
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="virtual-machines-container-disk-image" className="text-xs text-muted-foreground">{t("dialogs:vms.create.image")}</Label>
                            <Input id="virtual-machines-container-disk-image" className="bg-muted/30" value={createForm.image} onChange={(e) => setCreateForm((f) => ({ ...f, image: e.target.value }))} />
                        </div>
                    </div>
                    <DialogFooter>
                        <Button
                            variant="brand"
                            className="rounded-xl"
                            disabled={busy === "create" || !createForm.name}
                            onClick={async () => {
                                setBusy("create");
                                try {
                                    await api.post("/vms", { ...createForm, running: true });
                                    toast.success(t("toasts:vms.created", { name: createForm.name }));
                                    setCreateOpen(false);
                                    await load();
                                } catch (e) {
                                    toast.error(t("toasts:vms.createFailed"), { description: e?.response?.data?.error || e.message });
                                } finally {
                                    setBusy("");
                                }
                            }}
                        >
                            {busy === "create" ? t("dialogs:vms.creating") : t("common:actions.create")}
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>

            <div className="grid g3">
                {vms.map((vm) => {
                    const running = vm.running || vm.status === "Running";
                    return (
                        <div key={`${vm.namespace}/${vm.name}`} data-testid={VMS.card(vm.name)} className="vm">
                            <div className="vm-h">
                                <Link
                                    to={`/vms/${encodeURIComponent(vm.namespace)}/${encodeURIComponent(vm.name)}`}
                                    className="mono hover:underline text-ink"
                                >
                                    <b>{vm.name}</b>
                                </Link>
                                <StatusBadge status={vm.status} />
                            </div>
                            <div className="vm-specs">
                                <div>
                                    <Cpu size={13} /> vCPU
                                    <b>{vm.cpu}</b>
                                </div>
                                <div>
                                    <MemoryStick size={13} /> {t("common:fields.memory")}
                                    <b>{vm.memory}</b>
                                </div>
                                <div>
                                    {t("common:fields.node")}
                                    <b className="mono text-[14px] mt-1.5">
                                        {vm.node || "—"}
                                    </b>
                                </div>
                            </div>
                            <div className="vm-acts">
                                {running ? (
                                    <button
                                        type="button"
                                        className="btn btn-line btn-sm"
                                        disabled={busy === `${vm.namespace}/${vm.name}:stop`}
                                        onClick={() => act(vm, "stop", "stop")}
                                    >
                                        <Square size={12} /> {t("pages:vms.actions.stop")}
                                    </button>
                                ) : (
                                    <button
                                        type="button"
                                        className="btn btn-line btn-sm"
                                        disabled={busy === `${vm.namespace}/${vm.name}:start`}
                                        onClick={() => act(vm, "start", "start")}
                                    >
                                        <Play size={12} /> {t("pages:vms.actions.start")}
                                    </button>
                                )}
                                <button
                                    type="button"
                                    className="btn btn-line btn-sm"
                                    disabled={!running || busy === `${vm.namespace}/${vm.name}:migrate`}
                                    onClick={() => act(vm, "migrate", "migrate")}
                                >
                                    <ArrowRightLeft size={13} /> {t("pages:vms.actions.migrate")}
                                </button>
                                <button type="button" className="btn btn-fill btn-sm" onClick={() => setOpenVm(vm)}>
                                    <Terminal size={13} /> {t("pages:vms.actions.console")}
                                </button>
                                <span className="ml-auto text-[12.5px] text-ink-3 self-center">{vm.age}</span>
                            </div>
                        </div>
                    );
                })}
            </div>

            <VmProtection />

            <Dialog open={!!openVm} onOpenChange={(o) => !o && setOpenVm(null)}>
                <DialogContent
                    data-testid={VMS.consoleModal}
                    className="max-w-lg border-border bg-card p-4 backdrop-blur-2xl"
                >
                    <DialogHeader className="flex-row items-center justify-between">
                        <div>
                            <DialogTitle className="mono text-sm">
                                {openVm?.name} <span className="text-muted-foreground">· {openVm?.os}</span>
                            </DialogTitle>
                        </div>
                        <button
                            data-testid={VMS.consoleClose}
                            aria-label={t("common:actions.close")} title={t("common:actions.close")} onClick={() => setOpenVm(null)}
                            className="rounded-lg border border-border bg-muted p-1.5 text-muted-foreground hover:bg-accent"
                        >
                            <X className="h-4 w-4" />
                        </button>
                    </DialogHeader>
                    {openVm && <ConsolePanel vm={openVm} />}
                </DialogContent>
            </Dialog>
        </div>
    );
}
