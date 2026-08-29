import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
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
        <GlassCard className="p-0 overflow-hidden">
            <div className="flex items-center justify-between border-b border-white/5 px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                    <Icon className="h-4 w-4 text-i17-cosmic" /> {title}
                </div>
                {action}
            </div>
            <div className="max-h-[360px] overflow-auto scrollbar-thin">
                {rows.length === 0 && <div className="px-4 py-8 text-center text-sm text-muted-foreground">{empty}</div>}
                {rows.map((r, i) => (
                    <div key={i} className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]">
                        {columns.map((c) => (
                            <div key={c.key} className={`${c.span || "col-span-3"} min-w-0 text-xs`}>
                                {c.render ? c.render(r) : <span className="mono truncate text-foreground">{r[c.key]}</span>}
                            </div>
                        ))}
                    </div>
                ))}
            </div>
        </GlassCard>
    );
}

function Field({ label, ...props }) {
    return (
        <div className="space-y-1.5">
            <Label className="text-xs text-muted-foreground">{label}</Label>
            <Input className="border-white/10 bg-white/[0.03]" {...props} />
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
            <DialogContent className="glass max-w-md border-white/10">
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
            <DialogContent className="glass max-w-md border-white/10">
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
            <DialogContent className="glass max-w-md border-white/10">
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
            <div className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-64 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    return (
        <div className="space-y-4">
            <div className="flex items-end justify-between">
                <div>
                    <h1 className="text-3xl font-semibold tracking-tight">Backups</h1>
                    <p className="text-sm text-muted-foreground">VolumeSnapshots and Velero cluster backups.</p>
                </div>
                <div className="flex gap-2">
                    <Button size="sm" variant="outline" onClick={() => setRestoreOpen(true)} className="rounded-xl border-white/10 bg-white/[0.03]">
                        <Undo2 className="mr-1.5 h-3.5 w-3.5" /> Restore
                    </Button>
                    <Button variant="brand" size="sm" onClick={() => setCreateOpen(true)} className="rounded-xl">
                        <Plus className="mr-1.5 h-3.5 w-3.5" /> Create snapshot
                    </Button>
                </div>
            </div>

            <Section
                title="Volume snapshots"
                icon={Snowflake}
                rows={snapshots}
                empty="No volume snapshots."
                columns={[
                    { key: "name", span: "col-span-4", render: (r) => <><div className="mono truncate text-foreground">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace} · {r.pvc}</div></> },
                    { key: "volumeSnapshotClass", span: "col-span-4", render: (r) => <span className="mono text-muted-foreground">{r.volumeSnapshotClass}</span> },
                    { key: "status", span: "col-span-3", render: (r) => <StatusBadge status={r.status} /> },
                    { key: "age", span: "col-span-1", render: (r) => <span className="mono text-right text-muted-foreground">{r.age}</span> },
                ]}
            />

            <Section
                title="Velero backups"
                icon={Archive}
                rows={veleroBackups || []}
                empty="No Velero backups."
                columns={[
                    { key: "name", span: "col-span-4", render: (r) => <><div className="mono truncate text-foreground">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                    { key: "namespaces", span: "col-span-4", render: (r) => <span className="mono text-muted-foreground">{r.namespaces}</span> },
                    { key: "phase", span: "col-span-3", render: (r) => <StatusBadge status={r.phase} /> },
                    { key: "age", span: "col-span-1", render: (r) => <span className="mono text-right text-muted-foreground">{r.age}</span> },
                ]}
            />

            <Section
                title="Velero schedules"
                icon={CalendarClock}
                rows={veleroSchedules || []}
                empty="No Velero schedules."
                action={
                    <Button size="sm" variant="outline" onClick={() => setScheduleOpen(true)} className="h-7 rounded-lg border-white/10 bg-white/[0.03] text-xs">
                        <Plus className="mr-1 h-3 w-3" /> New schedule
                    </Button>
                }
                columns={[
                    { key: "name", span: "col-span-4", render: (r) => <><div className="mono truncate text-foreground">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                    { key: "schedule", span: "col-span-3", render: (r) => <span className="mono text-muted-foreground">{r.schedule}</span> },
                    { key: "paused", span: "col-span-2", render: (r) => <StatusBadge status={r.paused ? "Paused" : "Running"} /> },
                    { key: "lastBackup", span: "col-span-2", render: (r) => <span className="mono text-muted-foreground">{r.lastBackup ? new Date(r.lastBackup).toLocaleString() : "—"}</span> },
                    { key: "age", span: "col-span-1", render: (r) => <span className="mono text-right text-muted-foreground">{r.age}</span> },
                ]}
            />

            <CreateBackupDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
            <RestoreDialog open={restoreOpen} onOpenChange={setRestoreOpen} onCreated={load} />
            <CreateScheduleDialog open={scheduleOpen} onOpenChange={setScheduleOpen} onCreated={load} />
        </div>
    );
}
