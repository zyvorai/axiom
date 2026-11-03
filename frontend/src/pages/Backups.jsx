import { useEffect, useState, useId } from "react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import { formatDate } from "@/lib/i18n";
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
                <h2 className="flex items-center gap-2 text-[15px]">
                    <Icon className="h-4 w-4 text-tone-link" /> {title}
                </h2>
                {action && <span className="r">{action}</span>}
            </div>
            {rows.length === 0 ? (
                <div className="empty">{empty}</div>
            ) : (
                <div className="overflow-x-auto max-h-[360px] overflow-y-auto" tabIndex={0} role="region" aria-label={title}>
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

function Field({ label, id, ...props }) {
    const auto = useId();
    const fieldId = id || auto;
    return (
        <div className="space-y-1.5">
            <Label htmlFor={fieldId} className="text-xs text-muted-foreground">{label}</Label>
            <Input id={fieldId} className="bg-muted/30" {...props} />
        </div>
    );
}

function CreateBackupDialog({ open, onOpenChange, onCreated }) {
    const { t } = useTranslation();
    const [form, setForm] = useState({ namespace: "default", name: "", pvc: "", volumeSnapshotClass: "" });
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const submit = async () => {
        setBusy(true);
        try {
            await api.post("/backups", form);
            toast.success(t("toasts:backups.backupRequested", { name: form.name }));
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error(t("toasts:backups.createBackupFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>{t("dialogs:backups.createSnapshot.title")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <Field label={t("common:fields.namespace")} value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} />
                    <Field label={t("common:fields.name")} value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="postgres-nightly" />
                    <Field label={t("dialogs:backups.createSnapshot.pvc")} value={form.pvc} onChange={(e) => set({ pvc: e.target.value })} placeholder="postgres-data" />
                    <Field
                        label={t("dialogs:backups.createSnapshot.snapshotClass")}
                        value={form.volumeSnapshotClass}
                        onChange={(e) => set({ volumeSnapshotClass: e.target.value })}
                        placeholder="csi-rbd"
                    />
                </div>
                <DialogFooter>
                    <Button variant="brand" onClick={submit} disabled={busy || !form.name || !form.pvc} className="rounded-xl">
                        {busy ? t("dialogs:backups.requesting") : t("common:actions.create")}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function RestoreDialog({ open, onOpenChange, onCreated }) {
    const { t } = useTranslation();
    const [form, setForm] = useState({ namespace: "default", snapshotName: "", pvcName: "", storageClass: "", size: "", accessModes: "ReadWriteOnce" });
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const submit = async () => {
        setBusy(true);
        try {
            await api.post("/backups/restore", { ...form, accessModes: form.accessModes ? [form.accessModes] : undefined });
            toast.success(t("toasts:backups.restoreRequested", { name: form.pvcName }));
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error(t("toasts:backups.restoreFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>{t("dialogs:backups.restore.title")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <Field label={t("common:fields.namespace")} value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} />
                    <Field label={t("dialogs:backups.restore.snapshotName")} value={form.snapshotName} onChange={(e) => set({ snapshotName: e.target.value })} placeholder="postgres-nightly" />
                    <Field label={t("dialogs:backups.restore.pvcName")} value={form.pvcName} onChange={(e) => set({ pvcName: e.target.value })} placeholder="postgres-data-restored" />
                    <div className="grid grid-cols-2 gap-3">
                        <Field label={t("dialogs:backups.restore.size")} value={form.size} onChange={(e) => set({ size: e.target.value })} placeholder="200Gi" />
                        <Field label={t("dialogs:backups.restore.accessMode")} value={form.accessModes} onChange={(e) => set({ accessModes: e.target.value })} placeholder="ReadWriteOnce" />
                    </div>
                    <Field label={t("dialogs:backups.restore.storageClass")} value={form.storageClass} onChange={(e) => set({ storageClass: e.target.value })} />
                </div>
                <DialogFooter>
                    <Button onClick={submit} disabled={busy || !form.snapshotName || !form.pvcName} className="rounded-xl">
                        {busy ? t("dialogs:backups.requesting") : t("dialogs:backups.restore.submit")}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function CreateScheduleDialog({ open, onOpenChange, onCreated }) {
    const { t } = useTranslation();
    const [form, setForm] = useState({ namespace: "velero", name: "", schedule: "0 2 * * *", ttl: "" });
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const submit = async () => {
        setBusy(true);
        try {
            await api.post("/velero/schedules", form);
            toast.success(t("toasts:backups.scheduleCreated", { name: form.name }));
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error(t("toasts:backups.createScheduleFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>{t("dialogs:backups.schedule.title")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <Field label={t("common:fields.namespace")} value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} />
                    <Field label={t("common:fields.name")} value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="nightly-backup" />
                    <Field label={t("dialogs:backups.schedule.cron")} value={form.schedule} onChange={(e) => set({ schedule: e.target.value })} placeholder="0 2 * * *" />
                    <Field label={t("dialogs:backups.ttl")} value={form.ttl} onChange={(e) => set({ ttl: e.target.value })} placeholder="720h0m0s" />
                </div>
                <DialogFooter>
                    <Button onClick={submit} disabled={busy || !form.name || !form.schedule} className="rounded-xl">
                        {busy ? t("dialogs:backups.creating") : t("common:actions.create")}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

export default function Backups() {
    const { t } = useTranslation();
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
                title={t("pages:backups.title")}
                description={t("pages:backups.description")}
                action={
                    <>
                        <button type="button" className="btn btn-sm btn-line" onClick={() => setRestoreOpen(true)}>
                            <Undo2 size={14} /> {t("pages:backups.restore")}
                        </button>
                        <button type="button" className="btn btn-sm btn-line" onClick={() => setVeleroOpen(true)}>
                            <Archive size={14} /> {t("pages:backups.veleroBackup")}
                        </button>
                        <button type="button" className="btn btn-sm btn-fill" onClick={() => setCreateOpen(true)}>
                            <Plus size={14} /> {t("pages:backups.createSnapshot")}
                        </button>
                    </>
                }
            />

            <Section
                title={t("pages:backups.snapshots.title")}
                icon={Snowflake}
                rows={snapshots}
                empty={t("pages:backups.snapshots.empty")}
                columns={[
                    { key: "name", label: t("common:fields.name"), render: (r) => <><div className="name mono">{r.name}</div><div className="text-[11px] text-ink-3">{r.namespace} · {r.pvc}</div></> },
                    { key: "volumeSnapshotClass", label: "VolumeSnapshotClass", render: (r) => <span className="mono num">{r.volumeSnapshotClass}</span> },
                    { key: "status", label: t("common:fields.status"), render: (r) => <StatusBadge status={r.status} /> },
                    { key: "age", label: t("common:fields.age"), render: (r) => <span className="mono num">{r.age}</span> },
                ]}
            />

            <Section
                title={t("pages:backups.veleroBackups.title")}
                icon={Archive}
                rows={veleroBackups || []}
                empty={t("pages:backups.veleroBackups.empty")}
                columns={[
                    { key: "name", label: t("common:fields.name"), render: (r) => <><div className="name mono">{r.name}</div><div className="text-[11px] text-ink-3">{r.namespace}</div></> },
                    { key: "namespaces", label: t("pages:backups.columns.namespaces"), render: (r) => <span className="mono num">{r.namespaces}</span> },
                    { key: "phase", label: t("pages:backups.columns.phase"), render: (r) => <StatusBadge status={r.phase} /> },
                    { key: "age", label: t("common:fields.age"), render: (r) => <span className="mono num">{r.age}</span> },
                ]}
            />

            <Section
                title={t("pages:backups.veleroSchedules.title")}
                icon={CalendarClock}
                rows={veleroSchedules || []}
                empty={t("pages:backups.veleroSchedules.empty")}
                action={
                    <button type="button" className="btn btn-sm btn-line" onClick={() => setScheduleOpen(true)}>
                        <Plus size={13} /> {t("pages:backups.newSchedule")}
                    </button>
                }
                columns={[
                    { key: "name", label: t("common:fields.name"), render: (r) => <><div className="name mono">{r.name}</div><div className="text-[11px] text-ink-3">{r.namespace}</div></> },
                    { key: "schedule", label: t("pages:backups.columns.schedule"), render: (r) => <span className="mono num">{r.schedule}</span> },
                    { key: "paused", label: t("common:fields.status"), render: (r) => <StatusBadge status={r.paused ? "Paused" : "Running"} /> },
                    { key: "lastBackup", label: t("pages:backups.columns.lastBackup"), render: (r) => <span className="mono num">{r.lastBackup ? formatDate(r.lastBackup) : "—"}</span> },
                    { key: "age", label: t("common:fields.age"), render: (r) => <span className="mono num">{r.age}</span> },
                ]}
            />

            <CreateBackupDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
            <RestoreDialog open={restoreOpen} onOpenChange={setRestoreOpen} onCreated={load} />
            <CreateScheduleDialog open={scheduleOpen} onOpenChange={setScheduleOpen} onCreated={load} />
            <Dialog open={veleroOpen} onOpenChange={setVeleroOpen}>
                <DialogContent className="max-w-md">
                    <DialogHeader>
                        <DialogTitle>{t("dialogs:backups.velero.title")}</DialogTitle>
                    </DialogHeader>
                    <div className="space-y-3">
                        <Field label={t("common:fields.namespace")} value={veleroForm.namespace} onChange={(e) => setVeleroForm((f) => ({ ...f, namespace: e.target.value }))} />
                        <Field label={t("common:fields.name")} value={veleroForm.name} onChange={(e) => setVeleroForm((f) => ({ ...f, name: e.target.value }))} placeholder="on-demand-1" />
                        <Field
                            label={t("dialogs:backups.velero.included")}
                            value={veleroForm.includedNamespaces}
                            onChange={(e) => setVeleroForm((f) => ({ ...f, includedNamespaces: e.target.value }))}
                        />
                        <Field label={t("dialogs:backups.ttl")} value={veleroForm.ttl} onChange={(e) => setVeleroForm((f) => ({ ...f, ttl: e.target.value }))} placeholder="720h0m0s" />
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
                                    toast.success(t("toasts:backups.veleroRequested", { name: veleroForm.name }));
                                    setVeleroOpen(false);
                                    load();
                                } catch (e) {
                                    toast.error(t("toasts:backups.veleroFailed"), { description: e?.response?.data?.error || e.message });
                                } finally {
                                    setVeleroBusy(false);
                                }
                            }}
                        >
                            {veleroBusy ? t("dialogs:backups.creating") : t("common:actions.create")}
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </div>
    );
}
