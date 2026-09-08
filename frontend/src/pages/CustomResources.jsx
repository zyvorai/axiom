import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import AutoTable from "@/components/AutoTable";
import { Skeleton } from "@/components/ui/skeleton";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { Search, Layers, Boxes } from "lucide-react";
import CodeEditor from "@/components/CodeEditor";

function InstanceDetail({ crd, instance, onOpenChange }) {
    if (!instance) return null;
    const name = instance.metadata?.name || instance.name || "—";
    return (
        <Dialog open onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-2xl">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">
                        {crd.kind} · {name}
                    </DialogTitle>
                </DialogHeader>
                <CodeEditor value={JSON.stringify(instance, null, 2)} editable={false} className="min-h-0 [&_.code-pane]:min-h-[50vh] [&_.code-pane]:max-h-[50vh]" />
            </DialogContent>
        </Dialog>
    );
}

export default function CustomResources() {
    const [crds, setCrds] = useState(null);
    const [q, setQ] = useState("");
    const [selected, setSelected] = useState(null);
    const [instances, setInstances] = useState(null);
    const [instancesError, setInstancesError] = useState(null);
    const [detail, setDetail] = useState(null);

    useEffect(() => {
        api.get("/crds").then((r) => setCrds(r.data || []));
    }, []);

    useEffect(() => {
        if (!selected) return;
        let alive = true;
        setInstances(null);
        setInstancesError(null);
        api
            .get(`/crds/${selected.group}/${selected.version}/${selected.plural}`)
            .then((r) => alive && setInstances(r.data || []))
            .catch((e) => {
                if (!alive) return;
                setInstances([]);
                setInstancesError(e?.response?.data?.error || e.message);
            });
        return () => {
            alive = false;
        };
    }, [selected]);

    const filtered = useMemo(() => {
        if (!crds) return [];
        if (!q) return crds;
        const needle = q.toLowerCase();
        return crds.filter((c) => c.kind.toLowerCase().includes(needle) || c.group.toLowerCase().includes(needle));
    }, [crds, q]);

    const openInstance = (row) => {
        const ns = row.metadata?.namespace || row.namespace;
        const name = row.metadata?.name || row.name;
        if (!selected || !name) return;
        api
            .get(`/crds/${selected.group}/${selected.version}/${selected.plural}/${encodeURIComponent(name)}`, {
                params: ns ? { namespace: ns } : {},
            })
            .then((r) => setDetail(r.data))
            .catch(() => setDetail(row));
    };

    if (!crds) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="mt-4 h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title="Custom Resources"
                description="Browse any CustomResourceDefinition installed in the cluster."
                tools={
                    <div className="input">
                        <Search size={15} style={{ color: "var(--ink-3)" }} />
                        <input
                            placeholder="Search CRDs"
                            value={q}
                            onChange={(e) => setQ(e.target.value)}
                            style={{ background: "transparent", border: "none", outline: "none", color: "inherit", flex: 1, font: "inherit" }}
                        />
                    </div>
                }
            />

            <div className="grid gap-4 lg:grid-cols-[320px_1fr]">
                <GlassCard className="overflow-hidden">
                    <div style={{ maxHeight: "65vh", overflow: "auto" }}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>Kind</th>
                                    <th>Group / Version</th>
                                    <th>Scope</th>
                                </tr>
                            </thead>
                            <tbody>
                                {filtered.length === 0 && (
                                    <tr>
                                        <td colSpan={3}>
                                            <div className="empty">No CRDs match.</div>
                                        </td>
                                    </tr>
                                )}
                                {filtered.map((c) => {
                                    const active = selected && selected.group === c.group && selected.plural === c.plural;
                                    return (
                                        <tr
                                            key={`${c.group}/${c.version}/${c.plural}`}
                                            onClick={() => {
                                                setSelected(c);
                                                setDetail(null);
                                            }}
                                            style={{ cursor: "pointer", background: active ? "var(--active)" : undefined }}
                                        >
                                            <td className="name mono">
                                                <span style={{ display: "flex", alignItems: "center", gap: 8 }}>
                                                    <Layers size={14} style={{ color: "var(--ink-3)" }} />
                                                    {c.kind}
                                                </span>
                                            </td>
                                            <td className="mono num">
                                                {c.group}/{c.version}
                                            </td>
                                            <td>
                                                <span className="pill soft">{c.scope}</span>
                                            </td>
                                        </tr>
                                    );
                                })}
                            </tbody>
                        </table>
                    </div>
                </GlassCard>

                {selected ? (
                    <AutoTable
                        title={`${selected.kind} instances`}
                        icon={Boxes}
                        items={instances}
                        emptyHint={instances === null ? "Loading…" : instancesError ? `Permission denied: ${instancesError}` : "No instances found."}
                        onRowClick={openInstance}
                    />
                ) : (
                    <GlassCard>
                        <div className="empty">Select a CRD to browse its instances.</div>
                    </GlassCard>
                )}
            </div>

            {detail && <InstanceDetail crd={selected} instance={detail} onOpenChange={(o) => !o && setDetail(null)} />}
        </div>
    );
}
