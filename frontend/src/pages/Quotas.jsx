import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Gauge, Ruler, ShieldHalf, ArrowUpDown } from "lucide-react";
import { useTranslation } from "react-i18next";

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
                <div className="max-h-[420px] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={title}>
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
                <Badge key={k} variant="outline" className="mono text-[11px] bg-muted/30">
                    {k}={v}
                </Badge>
            ))}
        </div>
    );
}

export default function Quotas() {
    const { t } = useTranslation();
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
                title={t("pages:quotas.title")}
                description={t("pages:quotas.description")}
            />

            <Section
                title={t("pages:quotas.resourceQuotas.title")}
                icon={Gauge}
                rows={quotas}
                empty={t("pages:quotas.resourceQuotas.empty")}
                columns={[
                    { key: "name", label: t("common:fields.name"), render: (r) => <><div className="mono">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                    { key: "hard", label: t("pages:quotas.columns.hard"), render: (r) => <ResourceMap map={r.hard} /> },
                    { key: "used", label: t("pages:quotas.columns.used"), render: (r) => <ResourceMap map={r.used} /> },
                    { key: "age", label: t("common:fields.age"), render: (r) => <span className="mono num">{r.age}</span> },
                ]}
            />

            <div className="mt-6">
                <Section
                    title={t("pages:quotas.limitRanges.title")}
                    icon={Ruler}
                    rows={limitRanges || []}
                    empty={t("pages:quotas.limitRanges.empty")}
                    columns={[
                        { key: "name", label: t("common:fields.name"), render: (r) => <><div className="mono">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                        {
                            key: "limits",
                            label: t("pages:quotas.columns.limits"),
                            render: (r) => (
                                <div className="space-y-1.5">
                                    {(r.limits || []).map((l, i) => (
                                        <div key={i} className="flex flex-wrap items-center gap-1.5">
                                            <Badge className="text-[11px] bg-muted text-foreground border-none">{l.type}</Badge>
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
                    title={t("pages:quotas.pdbs.title")}
                    icon={ShieldHalf}
                    rows={pdbs || []}
                    empty={t("pages:quotas.pdbs.empty")}
                    columns={[
                        { key: "name", label: t("common:fields.name"), render: (r) => <><div className="mono">{r.name}</div><div className="text-[11px] text-muted-foreground">{r.namespace}</div></> },
                        { key: "minAvailable", label: t("pages:quotas.columns.minAvailable"), render: (r) => <span className="mono text-muted-foreground">{r.minAvailable || "—"}</span> },
                        { key: "maxUnavailable", label: t("pages:quotas.columns.maxUnavailable"), render: (r) => <span className="mono text-muted-foreground">{r.maxUnavailable || "—"}</span> },
                        { key: "healthy", label: t("pages:quotas.columns.healthy"), render: (r) => <span className="mono">{r.currentHealthy}/{r.desiredHealthy}</span> },
                        { key: "expected", label: t("pages:quotas.columns.expected"), render: (r) => <span className="mono text-muted-foreground">{t("pages:quotas.expectedPods", { count: r.expectedPods })}</span> },
                        { key: "age", label: t("common:fields.age"), render: (r) => <span className="mono num">{r.age}</span> },
                    ]}
                />
            </div>

            <div className="mt-6">
                <Section
                    title={t("pages:quotas.priorityClasses.title")}
                    icon={ArrowUpDown}
                    rows={priorityClasses || []}
                    empty={t("pages:quotas.priorityClasses.empty")}
                    columns={[
                        {
                            key: "name",
                            label: t("common:fields.name"),
                            render: (r) => (
                                <>
                                    <div className="mono">
                                        {r.name}
                                        {r.globalDefault && (
                                            <Badge className="ml-2 text-[11px] bg-muted text-foreground border-none">default</Badge>
                                        )}
                                    </div>
                                    {r.description && <div className="text-[11px] text-muted-foreground">{r.description}</div>}
                                </>
                            ),
                        },
                        { key: "value", label: t("common:fields.value"), render: (r) => <span className="mono text-muted-foreground">value: {r.value}</span> },
                        { key: "age", label: t("common:fields.age"), render: (r) => <span className="mono num">{r.age}</span> },
                    ]}
                />
            </div>
        </div>
    );
}
