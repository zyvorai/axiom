import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Gauge, Ruler, ShieldHalf, ArrowUpDown } from "lucide-react";

function Section({ title, icon: Icon, rows, columns, empty }) {
    return (
        <GlassCard className="overflow-hidden">
            <div className="card-h">
                <h2 className="flex items-center gap-2 text-[15px]">
                    <Icon size={16} className="text-tone-link" /> {title}
                </h2>
            </div>
            {rows.length === 0 ? (
                <div className="empty">{empty}</div>
            ) : (
                <div className="max-h-[420px] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label="Quotas and limits">
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
                                        <td key={c.key}>{c.render ? c.render(r) : <span className="mono">{r[c.key]}</span>}</td>
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

function ResourceMap({ map }) {
    const entries = Object.entries(map || {});
    if (entries.length === 0) return <span className="text-muted-foreground">—</span>;
    return (
        <div className="flex flex-wrap gap-1">
            {entries.map(([k, v]) => (
                <Badge key={k} variant="outline" className="mono text-[10px] bg-muted/30">
                    {k}={v}
                </Badge>
            ))}
        </div>
    );
}

export default function Quotas() {
    const [quotas, setQuotas] = useState(null);
    const [limitRanges, setLimitRanges] = useState(null);
    const [pdbs, setPdbs] = useState(null);
    const [priorityClasses, setPriorityClasses] = useState(null);

    useEffect(() => {
        let alive = true;
        Promise.all([
            api.get("/resourcequotas").catch(() => ({ data: [] })),
            api.get("/limitranges").catch(() => ({ data: [] })),
            api.get("/poddisruptionbudgets").catch(() => ({ data: [] })),
            api.get("/priorityclasses").catch(() => ({ data: [] })),
        ]).then(([q, l, p, pc]) => {
            if (!alive) return;
            setQuotas(q.data);
            setLimitRanges(l.data);
            setPdbs(p.data);
            setPriorityClasses(pc.data);
        });
        return () => {
            alive = false;
        };
    }, []);

    if (!quotas) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title="Quotas & Limits"
                description="ResourceQuotas, LimitRanges, PodDisruptionBudgets, and PriorityClasses."
            />

            <Section
                title="Resource quotas"
                icon={Gauge}
                rows={quotas}
                empty="No ResourceQuotas configured."
                columns={[
                    { key: "name", label: "Name", render: (r) => <><div className="mono">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                    { key: "hard", label: "Hard", render: (r) => <ResourceMap map={r.hard} /> },
                    { key: "used", label: "Used", render: (r) => <ResourceMap map={r.used} /> },
                    { key: "age", label: "Age", render: (r) => <span className="mono num">{r.age}</span> },
                ]}
            />

            <div className="mt-6">
                <Section
                    title="Limit ranges"
                    icon={Ruler}
                    rows={limitRanges || []}
                    empty="No LimitRanges configured."
                    columns={[
                        { key: "name", label: "Name", render: (r) => <><div className="mono">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                        {
                            key: "limits",
                            label: "Limits",
                            render: (r) => (
                                <div className="space-y-1.5">
                                    {(r.limits || []).map((l, i) => (
                                        <div key={i} className="flex flex-wrap items-center gap-1.5">
                                            <Badge className="text-[10px] bg-muted text-foreground border-none">{l.type}</Badge>
                                            {l.default && <span className="mono text-[11px] text-muted-foreground">default: {l.default}</span>}
                                            {l.defaultRequest && <span className="mono text-[11px] text-muted-foreground">request: {l.defaultRequest}</span>}
                                            {l.max && <span className="mono text-[11px] text-muted-foreground">max: {l.max}</span>}
                                            {l.min && <span className="mono text-[11px] text-muted-foreground">min: {l.min}</span>}
                                        </div>
                                    ))}
                                </div>
                            ),
                        },
                    ]}
                />
            </div>

            <div className="mt-6">
                <Section
                    title="Pod disruption budgets"
                    icon={ShieldHalf}
                    rows={pdbs || []}
                    empty="No PodDisruptionBudgets configured."
                    columns={[
                        { key: "name", label: "Name", render: (r) => <><div className="mono">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                        { key: "minAvailable", label: "Min available", render: (r) => <span className="mono text-muted-foreground">{r.minAvailable || "—"}</span> },
                        { key: "maxUnavailable", label: "Max unavailable", render: (r) => <span className="mono text-muted-foreground">{r.maxUnavailable || "—"}</span> },
                        { key: "healthy", label: "Healthy", render: (r) => <span className="mono">{r.currentHealthy}/{r.desiredHealthy}</span> },
                        { key: "expected", label: "Expected", render: (r) => <span className="mono text-muted-foreground">{r.expectedPods} pods</span> },
                        { key: "age", label: "Age", render: (r) => <span className="mono num">{r.age}</span> },
                    ]}
                />
            </div>

            <div className="mt-6">
                <Section
                    title="Priority classes"
                    icon={ArrowUpDown}
                    rows={priorityClasses || []}
                    empty="No PriorityClasses configured."
                    columns={[
                        {
                            key: "name",
                            label: "Name",
                            render: (r) => (
                                <>
                                    <div className="mono">
                                        {r.name}
                                        {r.globalDefault && (
                                            <Badge className="ml-2 text-[10px] bg-muted text-foreground border-none">default</Badge>
                                        )}
                                    </div>
                                    {r.description && <div className="text-[11px] text-muted-foreground">{r.description}</div>}
                                </>
                            ),
                        },
                        { key: "value", label: "Value", render: (r) => <span className="mono text-muted-foreground">value: {r.value}</span> },
                        { key: "age", label: "Age", render: (r) => <span className="mono num">{r.age}</span> },
                    ]}
                />
            </div>
        </div>
    );
}
