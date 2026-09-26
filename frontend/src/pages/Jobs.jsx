import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { CalendarClock, PlayCircle, Boxes, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useConfirm } from "@/components/ConfirmDialog";

function Section({ title, icon: Icon, rows, columns, empty }) {
    return (
        <GlassCard className="overflow-hidden">
            <div className="flex items-center justify-between border-b px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                    <Icon className="h-4 w-4 text-tone-link" /> {title}
                </div>
            </div>
            <div className="max-h-[420px] overflow-auto scrollbar-thin" tabIndex={0} role="region" aria-label="Jobs and CronJobs">
                <table className="tbl">
                    <thead>
                        <tr>
                            {columns.map((c) => (
                                <th key={c.key} style={c.align === "right" ? { textAlign: "right" } : undefined}>
                                    {c.header}
                                </th>
                            ))}
                        </tr>
                    </thead>
                    <tbody>
                        {rows.map((r, i) => (
                            <tr key={i}>
                                {columns.map((c) => (
                                    <td key={c.key} className={c.className} style={c.align === "right" ? { textAlign: "right" } : undefined}>
                                        {c.render ? c.render(r) : r[c.key]}
                                    </td>
                                ))}
                            </tr>
                        ))}
                        {rows.length === 0 && (
                            <tr>
                                <td colSpan={columns.length}>
                                    <div className="empty">{empty}</div>
                                </td>
                            </tr>
                        )}
                    </tbody>
                </table>
            </div>
        </GlassCard>
    );
}

export default function Jobs() {
    const confirm = useConfirm();
    const [cronJobs, setCronJobs] = useState(null);
    const [jobs, setJobs] = useState(null);

    const load = () => {
        Promise.all([
            api.get("/cronjobs").catch(() => ({ data: [] })),
            api.get("/jobs").catch(() => ({ data: [] })),
        ]).then(([c, j]) => {
            setCronJobs(c.data);
            setJobs(j.data);
        });
    };

    useEffect(() => {
        load();
        const t = setInterval(load, 10000);
        return () => clearInterval(t);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const trigger = async (c) => {
        try {
            await api.post(`/cronjobs/${c.namespace}/${encodeURIComponent(c.name)}/trigger`);
            toast.success(`Triggered ${c.name}`);
            load();
        } catch (e) {
            toast.error("Trigger failed", { description: e?.response?.data?.error || e.message });
        }
    };

    const setSuspend = async (c, suspend) => {
        try {
            await api.post(`/cronjobs/${c.namespace}/${encodeURIComponent(c.name)}/suspend`, { suspend });
            toast.success(suspend ? `Suspended ${c.name}` : `Resumed ${c.name}`);
            load();
        } catch (e) {
            toast.error("Update failed", { description: e?.response?.data?.error || e.message });
        }
    };

    const deleteCronJob = async (c) => {
        const ok = await confirm({
            title: `Delete CronJob ${c.name}?`,
            description: "This cannot be undone.",
            confirmLabel: "Delete",
        });
        if (!ok) return;
        try {
            await api.delete(`/cronjobs/${c.namespace}/${encodeURIComponent(c.name)}`);
            toast.success(`Deleted ${c.name}`);
            load();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };

    const deleteJob = async (j) => {
        const ok = await confirm({
            title: `Delete job ${j.name}?`,
            description: "This cannot be undone.",
            confirmLabel: "Delete",
        });
        if (!ok) return;
        try {
            await api.delete(`/jobs/${j.namespace}/${encodeURIComponent(j.name)}`);
            toast.success(`Deleted ${j.name}`);
            load();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };

    if (!cronJobs) {
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
                title="Jobs & CronJobs"
                description="Scheduled and one-off batch workloads across all namespaces."
            />

            <Section
                title="CronJobs"
                icon={CalendarClock}
                rows={cronJobs}
                empty="No CronJobs configured."
                columns={[
                    {
                        key: "name",
                        header: "Name",
                        className: "name",
                        render: (r) => (
                            <>
                                <Link to={`/cronjobs/${encodeURIComponent(r.namespace)}/${encodeURIComponent(r.name)}`} className="hover:underline">
                                    {r.name}
                                </Link>
                                <small>{r.namespace}</small>
                            </>
                        ),
                    },
                    { key: "schedule", header: "Schedule", className: "mono", render: (r) => r.schedule },
                    { key: "status", header: "Status", render: (r) => <StatusBadge status={r.suspend ? "Paused" : "Running"} /> },
                    { key: "lastSchedule", header: "Last run", className: "mono", render: (r) => r.lastSchedule || "—" },
                    { key: "age", header: "Age", className: "mono", render: (r) => r.age },
                    {
                        key: "actions",
                        header: "",
                        align: "right",
                        render: (r) => (
                            <div className="flex items-center justify-end gap-2">
                                <button type="button" className="btn btn-line btn-sm" title="Run now" onClick={() => trigger(r)}>
                                    <PlayCircle size={13} />
                                </button>
                                <Switch aria-label={`Schedule enabled for ${r.name}`} checked={!r.suspend} onCheckedChange={(v) => setSuspend(r, !v)} />
                                <button type="button" className="btn btn-line btn-sm" title="Delete" onClick={() => deleteCronJob(r)}>
                                    <Trash2 size={13} />
                                </button>
                            </div>
                        ),
                    },
                ]}
            />

            <div className="h-6" />

            <Section
                title="Jobs"
                icon={Boxes}
                rows={jobs || []}
                empty="No Jobs found."
                columns={[
                    {
                        key: "name",
                        header: "Name",
                        className: "name",
                        render: (r) => (
                            <>
                                <Link to={`/jobs/${encodeURIComponent(r.namespace)}/${encodeURIComponent(r.name)}`} className="hover:underline">
                                    {r.name}
                                </Link>
                                <small>{r.namespace}{r.ownerCronJob ? ` · from ${r.ownerCronJob}` : ""}</small>
                            </>
                        ),
                    },
                    { key: "status", header: "Status", render: (r) => <StatusBadge status={r.status} /> },
                    { key: "completions", header: "Completions", className: "mono", render: (r) => r.completions },
                    { key: "duration", header: "Duration", className: "mono", render: (r) => r.duration || "—" },
                    { key: "age", header: "Age", className: "mono", render: (r) => r.age },
                    {
                        key: "actions",
                        header: "",
                        align: "right",
                        render: (r) => (
                            <div className="flex justify-end">
                                <button type="button" className="btn btn-line btn-sm" title="Delete" onClick={() => deleteJob(r)}>
                                    <Trash2 size={13} />
                                </button>
                            </div>
                        ),
                    },
                ]}
            />
        </div>
    );
}
