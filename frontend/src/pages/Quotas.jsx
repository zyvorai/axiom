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
            <div className="flex items-center justify-between border-b px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                    <Icon className="h-4 w-4 text-i17-cosmic" /> {title}
                </div>
            </div>
            <div className="max-h-[420px] overflow-auto scrollbar-thin">
                {rows.length === 0 && <div className="px-4 py-8 text-center text-sm text-muted-foreground">{empty}</div>}
                {rows.map((r, i) => (
                    <div key={i} className="grid grid-cols-12 items-start gap-3 border-b px-4 py-3 last:border-none hover:bg-muted">
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
            <div className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="space-y-6">
            <PageHeader
                eyebrow="QUOTAS & LIMITS"
                title="Quotas & Limits"
                description="ResourceQuotas, LimitRanges, PodDisruptionBudgets, and PriorityClasses."
            />

            <Section
                title="Resource quotas"
                icon={Gauge}
                rows={quotas}
                empty="No ResourceQuotas configured."
                columns={[
                    { key: "name", span: "col-span-3", render: (r) => <><div className="mono truncate text-foreground">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                    { key: "hard", span: "col-span-4", render: (r) => <><div className="mb-1 text-[10px] uppercase tracking-wide text-muted-foreground">Hard</div><ResourceMap map={r.hard} /></> },
                    { key: "used", span: "col-span-4", render: (r) => <><div className="mb-1 text-[10px] uppercase tracking-wide text-muted-foreground">Used</div><ResourceMap map={r.used} /></> },
                    { key: "age", span: "col-span-1", render: (r) => <span className="mono text-right text-muted-foreground">{r.age}</span> },
                ]}
            />

            <Section
                title="Limit ranges"
                icon={Ruler}
                rows={limitRanges || []}
                empty="No LimitRanges configured."
                columns={[
                    { key: "name", span: "col-span-3", render: (r) => <><div className="mono truncate text-foreground">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                    {
                        key: "limits",
                        span: "col-span-9",
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

            <Section
                title="Pod disruption budgets"
                icon={ShieldHalf}
                rows={pdbs || []}
                empty="No PodDisruptionBudgets configured."
                columns={[
                    { key: "name", span: "col-span-3", render: (r) => <><div className="mono truncate text-foreground">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                    { key: "minAvailable", span: "col-span-2", render: (r) => <span className="mono text-muted-foreground">{r.minAvailable || "—"}</span> },
                    { key: "maxUnavailable", span: "col-span-2", render: (r) => <span className="mono text-muted-foreground">{r.maxUnavailable || "—"}</span> },
                    { key: "healthy", span: "col-span-2", render: (r) => <span className="mono">{r.currentHealthy}/{r.desiredHealthy}</span> },
                    { key: "expected", span: "col-span-2", render: (r) => <span className="mono text-muted-foreground">{r.expectedPods} pods</span> },
                    { key: "age", span: "col-span-1", render: (r) => <span className="mono text-right text-muted-foreground">{r.age}</span> },
                ]}
            />

            <Section
                title="Priority classes"
                icon={ArrowUpDown}
                rows={priorityClasses || []}
                empty="No PriorityClasses configured."
                columns={[
                    {
                        key: "name",
                        span: "col-span-4",
                        render: (r) => (
                            <>
                                <div className="mono truncate text-foreground">
                                    {r.name}
                                    {r.globalDefault && (
                                        <Badge className="ml-2 text-[10px] bg-muted text-foreground border-none">default</Badge>
                                    )}
                                </div>
                                {r.description && <div className="text-[11px] text-muted-foreground">{r.description}</div>}
                            </>
                        ),
                    },
                    { key: "value", span: "col-span-6", render: (r) => <span className="mono text-muted-foreground">value: {r.value}</span> },
                    { key: "age", span: "col-span-2", render: (r) => <span className="mono text-right text-muted-foreground">{r.age}</span> },
                ]}
            />
        </div>
    );
}
