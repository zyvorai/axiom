import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { CalendarClock, PlayCircle, Boxes, Trash2 } from "lucide-react";
import { toast } from "sonner";

function Section({ title, icon: Icon, rows, columns, empty }) {
    return (
        <GlassCard className="p-0 overflow-hidden">
            <div className="flex items-center justify-between border-b border-white/5 px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                    <Icon className="h-4 w-4 text-i17-cosmic" /> {title}
                </div>
            </div>
            <div className="max-h-[420px] overflow-auto scrollbar-thin">
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

export default function Jobs() {
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
        try {
            await api.delete(`/cronjobs/${c.namespace}/${encodeURIComponent(c.name)}`);
            toast.success(`Deleted ${c.name}`);
            load();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };

    const deleteJob = async (j) => {
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
                    <h1 className="text-3xl font-semibold tracking-tight">Jobs &amp; CronJobs</h1>
                    <p className="text-sm text-muted-foreground">Scheduled and one-off batch workloads across all namespaces.</p>
                </div>
            </div>

            <Section
                title="CronJobs"
                icon={CalendarClock}
                rows={cronJobs}
                empty="No CronJobs configured."
                columns={[
                    { key: "name", span: "col-span-3", render: (r) => <><div className="mono truncate text-foreground">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                    { key: "schedule", span: "col-span-2", render: (r) => <span className="mono text-muted-foreground">{r.schedule}</span> },
                    { key: "status", span: "col-span-2", render: (r) => <StatusBadge status={r.suspend ? "Paused" : "Running"} /> },
                    { key: "lastSchedule", span: "col-span-2", render: (r) => <span className="mono text-muted-foreground">{r.lastSchedule || "—"}</span> },
                    { key: "age", span: "col-span-1", render: (r) => <span className="mono text-muted-foreground">{r.age}</span> },
                    {
                        key: "actions",
                        span: "col-span-2",
                        render: (r) => (
                            <div className="flex items-center justify-end gap-2">
                                <Button size="sm" variant="ghost" onClick={() => trigger(r)} title="Run now" className="h-7 px-2 text-muted-foreground hover:text-i17-sky">
                                    <PlayCircle className="h-3.5 w-3.5" />
                                </Button>
                                <Switch checked={!r.suspend} onCheckedChange={(v) => setSuspend(r, !v)} />
                                <Button size="sm" variant="ghost" onClick={() => deleteCronJob(r)} className="h-7 px-2 text-muted-foreground hover:text-rose-400">
                                    <Trash2 className="h-3.5 w-3.5" />
                                </Button>
                            </div>
                        ),
                    },
                ]}
            />

            <Section
                title="Jobs"
                icon={Boxes}
                rows={jobs || []}
                empty="No Jobs found."
                columns={[
                    { key: "name", span: "col-span-4", render: (r) => <><div className="mono truncate text-foreground">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}{r.ownerCronJob ? ` · from ${r.ownerCronJob}` : ""}</div></> },
                    { key: "status", span: "col-span-2", render: (r) => <StatusBadge status={r.status} /> },
                    { key: "completions", span: "col-span-2", render: (r) => <span className="mono text-muted-foreground">{r.completions}</span> },
                    { key: "duration", span: "col-span-2", render: (r) => <span className="mono text-muted-foreground">{r.duration || "—"}</span> },
                    { key: "age", span: "col-span-1", render: (r) => <span className="mono text-muted-foreground">{r.age}</span> },
                    {
                        key: "actions",
                        span: "col-span-1",
                        render: (r) => (
                            <div className="flex justify-end">
                                <Button size="sm" variant="ghost" onClick={() => deleteJob(r)} className="h-7 px-2 text-muted-foreground hover:text-rose-400">
                                    <Trash2 className="h-3.5 w-3.5" />
                                </Button>
                            </div>
                        ),
                    },
                ]}
            />
        </div>
    );
}
