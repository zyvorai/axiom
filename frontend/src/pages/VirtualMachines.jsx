import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
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

function ConsolePanel({ vm }) {
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

    if (info === null) return <Skeleton className="h-40 rounded-xl bg-white/[0.04]" />;
    if (info === false) return <div className="rounded-xl border border-white/10 p-6 text-center text-sm text-muted-foreground">Console info unavailable.</div>;

    const copy = () => {
        navigator.clipboard?.writeText(info.virtctl);
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
    };

    return (
        <div className="space-y-4 rounded-xl border border-white/10 bg-muted p-6">
            <div className="flex items-center gap-3">
                <div className="grid h-10 w-10 place-items-center rounded-xl bg-white/[0.06]">
                    <Terminal className="h-4 w-4 text-i17-sky" />
                </div>
                <div>
                    <div className="text-sm font-medium">{info.ready ? "VM is ready for console access" : "VM is not ready"}</div>
                    <div className="text-xs text-muted-foreground">
                        Use virtctl locally, or connect a VNC client to the proxied WebSocket path below.
                    </div>
                </div>
            </div>
            {info.wsPath && (
                <div>
                    <div className="mb-1 text-[10px] uppercase tracking-widest text-muted-foreground">Browser WebSocket proxy</div>
                    <code className="mono block truncate text-[11px] text-i17-sage">{info.wsPath}</code>
                </div>
            )}
            <div>
                <div className="mb-1 text-[10px] uppercase tracking-widest text-muted-foreground">Run locally (kubeconfig required)</div>
                <div className="flex items-center gap-2 rounded-lg border border-white/10 bg-white/[0.03] px-3 py-2">
                    <code className="mono flex-1 truncate text-xs text-i17-sage">{info.virtctl}</code>
                    <button onClick={copy} className="text-muted-foreground hover:text-foreground">
                        {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                    </button>
                </div>
            </div>
            <div>
                <div className="mb-1 text-[10px] uppercase tracking-widest text-muted-foreground">Kubernetes API subresource</div>
                <code className="mono block truncate text-[11px] text-muted-foreground">{info.vncPath}</code>
            </div>
        </div>
    );
}

function ProtectionSection({ title, icon: Icon, endpoint, columns, formFields, emptyHint }) {
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
            toast.success("Requested");
            setOpen(false);
            setForm({});
            load();
        } catch (e) {
            toast.error("Create failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <GlassCard className="overflow-hidden">
            <div className="flex items-center justify-between border-b px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                    <Icon className="h-4 w-4 text-i17-cosmic" /> {title}
                </div>
                <Button size="sm" variant="brand" onClick={() => setOpen(true)} className="rounded-xl">
                    <Plus className="mr-1 h-3.5 w-3.5" /> New
                </Button>
            </div>
            <div className="max-h-[360px] overflow-auto scrollbar-thin">
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
                <DialogContent className="glass max-w-md">
                    <DialogHeader>
                        <DialogTitle>New {title.replace(/s$/, "")}</DialogTitle>
                    </DialogHeader>
                    <div className="space-y-3">
                        {formFields.map((f) => (
                            <div key={f.key} className="space-y-1.5">
                                <Label className="text-xs text-muted-foreground">{f.label}</Label>
                                <Input
                                    value={form[f.key] || ""}
                                    onChange={(e) => set({ [f.key]: e.target.value })}
                                    placeholder={f.placeholder}
                                />
                            </div>
                        ))}
                    </div>
                    <DialogFooter>
                        <Button variant="brand" onClick={submit} disabled={busy} className="rounded-xl">
                            {busy ? "Creating…" : "Create"}
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </GlassCard>
    );
}

function VmProtection() {
    return (
        <Tabs defaultValue="snapshots">
            <TabsList className="tabs" style={{ background: "none", padding: 0, height: "auto", borderRadius: 0 }}>
                <TabsTrigger value="snapshots" className="gap-1.5">
                    <Camera className="h-3.5 w-3.5" /> Snapshots
                </TabsTrigger>
                <TabsTrigger value="clones" className="gap-1.5">
                    <CopyIcon className="h-3.5 w-3.5" /> Clones
                </TabsTrigger>
                <TabsTrigger value="exports" className="gap-1.5">
                    <Upload className="h-3.5 w-3.5" /> Exports
                </TabsTrigger>
                <TabsTrigger value="restores" className="gap-1.5">
                    <RotateCcw className="h-3.5 w-3.5" /> Restores
                </TabsTrigger>
            </TabsList>
            <TabsContent value="snapshots" className="mt-3">
                <ProtectionSection
                    title="Snapshots"
                    icon={Camera}
                    endpoint="/vm-snapshots"
                    emptyHint="No VM snapshots yet."
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
                        { key: "namespace", label: "Namespace", placeholder: "default" },
                        { key: "name", label: "Name", placeholder: "snap-1" },
                        { key: "vm", label: "VM name", placeholder: "edge-api-vm" },
                    ]}
                />
            </TabsContent>
            <TabsContent value="clones" className="mt-3">
                <ProtectionSection
                    title="Clones"
                    icon={CopyIcon}
                    endpoint="/vm-clones"
                    emptyHint="No VM clones yet."
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
                        { key: "namespace", label: "Namespace", placeholder: "default" },
                        { key: "name", label: "Name", placeholder: "clone-1" },
                        { key: "sourceVM", label: "Source VM", placeholder: "edge-api-vm" },
                        { key: "targetVM", label: "Target VM name", placeholder: "edge-api-vm-clone" },
                    ]}
                />
            </TabsContent>
            <TabsContent value="exports" className="mt-3">
                <ProtectionSection
                    title="Exports"
                    icon={Upload}
                    endpoint="/vm-exports"
                    emptyHint="No VM exports yet."
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
                        { key: "namespace", label: "Namespace", placeholder: "default" },
                        { key: "name", label: "Name", placeholder: "export-1" },
                        { key: "vm", label: "VM name", placeholder: "edge-api-vm" },
                    ]}
                />
            </TabsContent>
            <TabsContent value="restores" className="mt-3">
                <ProtectionSection
                    title="Restores"
                    icon={RotateCcw}
                    endpoint="/vm-restores"
                    emptyHint="No VM restores yet."
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
                        { key: "namespace", label: "Namespace", placeholder: "default" },
                        { key: "name", label: "Name", placeholder: "restore-1" },
                        { key: "snapshot", label: "Snapshot name", placeholder: "snap-1" },
                        { key: "targetVM", label: "Target VM name", placeholder: "edge-api-vm-restored" },
                    ]}
                />
            </TabsContent>
        </Tabs>
    );
}

export default function VirtualMachines() {
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
        const t = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    const act = async (vm, action, path) => {
        const key = `${vm.namespace}/${vm.name}:${action}`;
        setBusy(key);
        try {
            await api.post(`/vms/${vm.namespace}/${encodeURIComponent(vm.name)}/${path}`);
            toast.success(`${vm.name}: ${action} requested`);
            await load();
        } catch (e) {
            toast.error(`${action} failed`, { description: e?.response?.data?.error || e.message });
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
                title="Virtual Machines"
                description="KubeVirt-managed VMs. Start, stop, live-migrate and open a console."
                action={
                    <>
                        <span style={{ fontSize: 13, color: "var(--ink-3)" }}>{vms.length} VMs</span>
                        <button type="button" className="btn btn-fill" onClick={() => setCreateOpen(true)}>
                            <Plus size={15} /> Create VM
                        </button>
                    </>
                }
            />

            <Dialog open={createOpen} onOpenChange={setCreateOpen}>
                <DialogContent className="glass max-w-md">
                    <DialogHeader>
                        <DialogTitle>Create virtual machine</DialogTitle>
                    </DialogHeader>
                    <div className="space-y-3">
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Namespace</Label>
                            <Input className="bg-muted/30" value={createForm.namespace} onChange={(e) => setCreateForm((f) => ({ ...f, namespace: e.target.value }))} />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Name</Label>
                            <Input className="bg-muted/30" value={createForm.name} onChange={(e) => setCreateForm((f) => ({ ...f, name: e.target.value }))} placeholder="web-vm" />
                        </div>
                        <div className="grid grid-cols-2 gap-3">
                            <div className="space-y-1.5">
                                <Label className="text-xs text-muted-foreground">CPU cores</Label>
                                <Input className="bg-muted/30" value={createForm.cpu} onChange={(e) => setCreateForm((f) => ({ ...f, cpu: e.target.value }))} />
                            </div>
                            <div className="space-y-1.5">
                                <Label className="text-xs text-muted-foreground">Memory</Label>
                                <Input className="bg-muted/30" value={createForm.memory} onChange={(e) => setCreateForm((f) => ({ ...f, memory: e.target.value }))} />
                            </div>
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Container disk image</Label>
                            <Input className="bg-muted/30" value={createForm.image} onChange={(e) => setCreateForm((f) => ({ ...f, image: e.target.value }))} />
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
                                    toast.success(`VM ${createForm.name} created`);
                                    setCreateOpen(false);
                                    await load();
                                } catch (e) {
                                    toast.error("Create failed", { description: e?.response?.data?.error || e.message });
                                } finally {
                                    setBusy("");
                                }
                            }}
                        >
                            {busy === "create" ? "Creating…" : "Create"}
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
                                    className="mono hover:underline"
                                    style={{ color: "var(--ink)" }}
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
                                    <MemoryStick size={13} /> Memory
                                    <b>{vm.memory}</b>
                                </div>
                                <div>
                                    Node
                                    <b className="mono" style={{ fontSize: 14, marginTop: 6 }}>
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
                                        <Square size={12} /> Stop
                                    </button>
                                ) : (
                                    <button
                                        type="button"
                                        className="btn btn-line btn-sm"
                                        disabled={busy === `${vm.namespace}/${vm.name}:start`}
                                        onClick={() => act(vm, "start", "start")}
                                    >
                                        <Play size={12} /> Start
                                    </button>
                                )}
                                <button
                                    type="button"
                                    className="btn btn-line btn-sm"
                                    disabled={!running || busy === `${vm.namespace}/${vm.name}:migrate`}
                                    onClick={() => act(vm, "migrate", "migrate")}
                                >
                                    <ArrowRightLeft size={13} /> Migrate
                                </button>
                                <button type="button" className="btn btn-fill btn-sm" onClick={() => setOpenVm(vm)}>
                                    <Terminal size={13} /> Console
                                </button>
                                <span style={{ marginLeft: "auto", fontSize: 12.5, color: "var(--ink-3)", alignSelf: "center" }}>{vm.age}</span>
                            </div>
                        </div>
                    );
                })}
            </div>

            <VmProtection />

            <Dialog open={!!openVm} onOpenChange={(o) => !o && setOpenVm(null)}>
                <DialogContent
                    data-testid={VMS.consoleModal}
                    className="glass max-w-lg border-border bg-card p-4 backdrop-blur-2xl"
                >
                    <DialogHeader className="flex-row items-center justify-between">
                        <div>
                            <DialogTitle className="mono text-sm">
                                {openVm?.name} <span className="text-muted-foreground">· {openVm?.os}</span>
                            </DialogTitle>
                        </div>
                        <button
                            data-testid={VMS.consoleClose}
                            onClick={() => setOpenVm(null)}
                            className="rounded-lg border border-white/10 bg-white/[0.03] p-1.5 text-muted-foreground hover:bg-white/[0.08]"
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
