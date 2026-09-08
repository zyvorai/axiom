import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { Archive, CalendarClock, Snowflake, Plus, Undo2 } from "lucide-react";
import { toast } from "sonner";

function Section({ title, icon: Icon, rows, columns, empty, action }) {
    return (
        <GlassCard className="overflow-hidden">
            <div className="card-h">
                <h2 style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 15 }}>
                    <Icon className="h-4 w-4" style={{ color: "var(--blue)" }} /> {title}
                </h2>
                {action && <span className="r">{action}</span>}
            </div>
            {rows.length === 0 ? (
                <div className="empty">{empty}</div>
            ) : (
                <div style={{ overflowX: "auto", maxHeight: 360, overflowY: "auto" }}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                {columns.map((c) => (
                                    <th key={c.key}>{c.label}</th>
                                ))}
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((r, i) => (
                                <tr key={i}>
                                    {columns.map((c) => (
                                        <td key={c.key}>{c.render ? c.render(r) : r[c.key]}</td>
                                    ))}
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </GlassCard>
    );
}

function Field({ label, ...props }) {
    return (
        <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">{label}</Label>
            <Input className="bg-muted/30" {...props} />
        </div>
    );
}

function CreateBackupDialog({ open, onOpenChange, onCreated }) {
    const [form, setForm] = useState({ namespace: "default", name: "", pvc: "", volumeSnapshotClass: "" });
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const submit = async () => {
        setBusy(true);
        try {
            await api.post("/backups", form);
            toast.success(`Backup ${form.name} requested`);
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error("Create backup failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-md">
                <DialogHeader>
                    <DialogTitle>Create volume snapshot</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <Field label="Namespace" value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} />
                    <Field label="Name" value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="postgres-nightly" />
                    <Field label="PVC" value={form.pvc} onChange={(e) => set({ pvc: e.target.value })} placeholder="postgres-data" />
                    <Field
                        label="VolumeSnapshotClass (optional)"
                        value={form.volumeSnapshotClass}
                        onChange={(e) => set({ volumeSnapshotClass: e.target.value })}
                        placeholder="csi-rbd"
                    />
                </div>
                <DialogFooter>
                    <Button variant="brand" onClick={submit} disabled={busy || !form.name || !form.pvc} className="rounded-xl">
                        {busy ? "Requesting…" : "Create"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function RestoreDialog({ open, onOpenChange, onCreated }) {
    const [form, setForm] = useState({ namespace: "default", snapshotName: "", pvcName: "", storageClass: "", size: "", accessModes: "ReadWriteOnce" });
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const submit = async () => {
        setBusy(true);
        try {
            await api.post("/backups/restore", { ...form, accessModes: form.accessModes ? [form.accessModes] : undefined });
            toast.success(`Restore ${form.pvcName} requested`);
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error("Restore failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-md">
                <DialogHeader>
                    <DialogTitle>Restore from snapshot</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <Field label="Namespace" value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} />
                    <Field label="Snapshot name" value={form.snapshotName} onChange={(e) => set({ snapshotName: e.target.value })} placeholder="postgres-nightly" />
                    <Field label="New PVC name" value={form.pvcName} onChange={(e) => set({ pvcName: e.target.value })} placeholder="postgres-data-restored" />
                    <div className="grid grid-cols-2 gap-3">
                        <Field label="Size" value={form.size} onChange={(e) => set({ size: e.target.value })} placeholder="200Gi" />
                        <Field label="Access mode" value={form.accessModes} onChange={(e) => set({ accessModes: e.target.value })} placeholder="ReadWriteOnce" />
                    </div>
                    <Field label="StorageClass (optional)" value={form.storageClass} onChange={(e) => set({ storageClass: e.target.value })} />
                </div>
                <DialogFooter>
                    <Button onClick={submit} disabled={busy || !form.snapshotName || !form.pvcName} className="rounded-xl">
                        {busy ? "Requesting…" : "Restore"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function CreateScheduleDialog({ open, onOpenChange, onCreated }) {
    const [form, setForm] = useState({ namespace: "velero", name: "", schedule: "0 2 * * *", ttl: "" });
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const submit = async () => {
        setBusy(true);
        try {
            await api.post("/velero/schedules", form);
            toast.success(`Schedule ${form.name} created`);
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error("Create schedule failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-md">
                <DialogHeader>
                    <DialogTitle>New Velero schedule</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <Field label="Namespace" value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} />
                    <Field label="Name" value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="nightly-backup" />
                    <Field label="Cron schedule" value={form.schedule} onChange={(e) => set({ schedule: e.target.value })} placeholder="0 2 * * *" />
                    <Field label="TTL (optional)" value={form.ttl} onChange={(e) => set({ ttl: e.target.value })} placeholder="720h0m0s" />
                </div>
                <DialogFooter>
                    <Button onClick={submit} disabled={busy || !form.name || !form.schedule} className="rounded-xl">
                        {busy ? "Creating…" : "Create"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

export default function Backups() {
    const [snapshots, setSnapshots] = useState(null);
    const [veleroBackups, setVeleroBackups] = useState(null);
    const [veleroSchedules, setVeleroSchedules] = useState(null);
    const [createOpen, setCreateOpen] = useState(false);
    const [restoreOpen, setRestoreOpen] = useState(false);
    const [scheduleOpen, setScheduleOpen] = useState(false);
    const [veleroOpen, setVeleroOpen] = useState(false);
    const [veleroForm, setVeleroForm] = useState({ namespace: "velero", name: "", includedNamespaces: "*", ttl: "" });
    const [veleroBusy, setVeleroBusy] = useState(false);

    const load = () => {
        Promise.all([
            api.get("/backups").catch(() => ({ data: [] })),
            api.get("/velero/backups").catch(() => ({ data: [] })),
            api.get("/velero/schedules").catch(() => ({ data: [] })),
        ]).then(([a, b, c]) => {
            setSnapshots(a.data);
            setVeleroBackups(b.data);
            setVeleroSchedules(c.data);
        });
    };

    useEffect(() => {
        load();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    if (!snapshots) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="mt-4 h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title="Backups"
                description="VolumeSnapshots and Velero cluster backups."
                action={
                    <>
                        <button type="button" className="btn btn-sm btn-line" onClick={() => setRestoreOpen(true)}>
                            <Undo2 size={14} /> Restore
                        </button>
                        <button type="button" className="btn btn-sm btn-line" onClick={() => setVeleroOpen(true)}>
                            <Archive size={14} /> Velero backup
                        </button>
                        <button type="button" className="btn btn-sm btn-fill" onClick={() => setCreateOpen(true)}>
                            <Plus size={14} /> Create snapshot
                        </button>
                    </>
                }
            />

            <Section
                title="Volume snapshots"
                icon={Snowflake}
                rows={snapshots}
                empty="No volume snapshots."
                columns={[
                    { key: "name", label: "Name", render: (r) => <><div className="name mono">{r.name}</div><div style={{ fontSize: 11, color: "var(--ink-3)" }}>{r.namespace} · {r.pvc}</div></> },
                    { key: "volumeSnapshotClass", label: "VolumeSnapshotClass", render: (r) => <span className="mono num">{r.volumeSnapshotClass}</span> },
                    { key: "status", label: "Status", render: (r) => <StatusBadge status={r.status} /> },
                    { key: "age", label: "Age", render: (r) => <span className="mono num">{r.age}</span> },
                ]}
            />

            <Section
                title="Velero backups"
                icon={Archive}
                rows={veleroBackups || []}
                empty="No Velero backups."
                columns={[
                    { key: "name", label: "Name", render: (r) => <><div className="name mono">{r.name}</div><div style={{ fontSize: 11, color: "var(--ink-3)" }}>{r.namespace}</div></> },
                    { key: "namespaces", label: "Namespaces", render: (r) => <span className="mono num">{r.namespaces}</span> },
                    { key: "phase", label: "Phase", render: (r) => <StatusBadge status={r.phase} /> },
                    { key: "age", label: "Age", render: (r) => <span className="mono num">{r.age}</span> },
                ]}
            />

            <Section
                title="Velero schedules"
                icon={CalendarClock}
                rows={veleroSchedules || []}
                empty="No Velero schedules."
                action={
                    <button type="button" className="btn btn-sm btn-line" onClick={() => setScheduleOpen(true)}>
                        <Plus size={13} /> New schedule
                    </button>
                }
                columns={[
                    { key: "name", label: "Name", render: (r) => <><div className="name mono">{r.name}</div><div style={{ fontSize: 11, color: "var(--ink-3)" }}>{r.namespace}</div></> },
                    { key: "schedule", label: "Schedule", render: (r) => <span className="mono num">{r.schedule}</span> },
                    { key: "paused", label: "Status", render: (r) => <StatusBadge status={r.paused ? "Paused" : "Running"} /> },
                    { key: "lastBackup", label: "Last backup", render: (r) => <span className="mono num">{r.lastBackup ? new Date(r.lastBackup).toLocaleString() : "—"}</span> },
                    { key: "age", label: "Age", render: (r) => <span className="mono num">{r.age}</span> },
                ]}
            />

            <CreateBackupDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
            <RestoreDialog open={restoreOpen} onOpenChange={setRestoreOpen} onCreated={load} />
            <CreateScheduleDialog open={scheduleOpen} onOpenChange={setScheduleOpen} onCreated={load} />
            <Dialog open={veleroOpen} onOpenChange={setVeleroOpen}>
                <DialogContent className="glass max-w-md">
                    <DialogHeader>
                        <DialogTitle>Create Velero backup</DialogTitle>
                    </DialogHeader>
                    <div className="space-y-3">
                        <Field label="Namespace" value={veleroForm.namespace} onChange={(e) => setVeleroForm((f) => ({ ...f, namespace: e.target.value }))} />
                        <Field label="Name" value={veleroForm.name} onChange={(e) => setVeleroForm((f) => ({ ...f, name: e.target.value }))} placeholder="on-demand-1" />
                        <Field
                            label="Included namespaces (comma-separated or *)"
                            value={veleroForm.includedNamespaces}
                            onChange={(e) => setVeleroForm((f) => ({ ...f, includedNamespaces: e.target.value }))}
                        />
                        <Field label="TTL (optional)" value={veleroForm.ttl} onChange={(e) => setVeleroForm((f) => ({ ...f, ttl: e.target.value }))} placeholder="720h0m0s" />
                    </div>
                    <DialogFooter>
                        <Button
                            variant="brand"
                            className="rounded-xl"
                            disabled={veleroBusy || !veleroForm.name}
                            onClick={async () => {
                                setVeleroBusy(true);
                                try {
                                    const included = veleroForm.includedNamespaces
                                        .split(",")
                                        .map((s) => s.trim())
                                        .filter(Boolean);
                                    await api.post("/velero/backups", {
                                        namespace: veleroForm.namespace,
                                        name: veleroForm.name,
                                        includedNamespaces: included,
                                        ttl: veleroForm.ttl || undefined,
                                    });
                                    toast.success(`Velero backup ${veleroForm.name} requested`);
                                    setVeleroOpen(false);
                                    load();
                                } catch (e) {
                                    toast.error("Velero backup failed", { description: e?.response?.data?.error || e.message });
                                } finally {
                                    setVeleroBusy(false);
                                }
                            }}
                        >
                            {veleroBusy ? "Creating…" : "Create"}
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </div>
    );
}
