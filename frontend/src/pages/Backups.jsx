import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Archive, CalendarClock, Snowflake } from "lucide-react";

function Section({ title, icon: Icon, rows, columns, empty }) {
    return (
        <GlassCard className="p-0 overflow-hidden">
            <div className="flex items-center gap-2 border-b border-white/5 px-4 py-3 text-sm font-medium">
                <Icon className="h-4 w-4 text-[#ff7550]" /> {title}
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

export default function Backups() {
    const [snapshots, setSnapshots] = useState(null);
    const [veleroBackups, setVeleroBackups] = useState(null);
    const [veleroSchedules, setVeleroSchedules] = useState(null);

    useEffect(() => {
        let alive = true;
        Promise.all([
            api.get("/backups").catch(() => ({ data: [] })),
            api.get("/velero/backups").catch(() => ({ data: [] })),
            api.get("/velero/schedules").catch(() => ({ data: [] })),
        ]).then(([a, b, c]) => {
            if (!alive) return;
            setSnapshots(a.data);
            setVeleroBackups(b.data);
            setVeleroSchedules(c.data);
        });
        return () => {
            alive = false;
        };
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
            <div>
                <h1 className="text-3xl font-semibold tracking-tight">Backups</h1>
                <p className="text-sm text-muted-foreground">VolumeSnapshots and Velero cluster backups.</p>
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
                columns={[
                    { key: "name", span: "col-span-4", render: (r) => <><div className="mono truncate text-foreground">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                    { key: "schedule", span: "col-span-3", render: (r) => <span className="mono text-muted-foreground">{r.schedule}</span> },
                    { key: "paused", span: "col-span-2", render: (r) => <StatusBadge status={r.paused ? "Paused" : "Running"} /> },
                    { key: "lastBackup", span: "col-span-2", render: (r) => <span className="mono text-muted-foreground">{r.lastBackup ? new Date(r.lastBackup).toLocaleString() : "—"}</span> },
                    { key: "age", span: "col-span-1", render: (r) => <span className="mono text-right text-muted-foreground">{r.age}</span> },
                ]}
            />
        </div>
    );
}
