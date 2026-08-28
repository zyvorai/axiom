import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import AutoTable from "@/components/AutoTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { Search, Layers, Boxes } from "lucide-react";

function InstanceDetail({ crd, instance, onOpenChange }) {
    if (!instance) return null;
    const name = instance.metadata?.name || instance.name || "—";
    return (
        <Dialog open onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-2xl border-white/10">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">
                        {crd.kind} · {name}
                    </DialogTitle>
                </DialogHeader>
                <pre className="mono max-h-[60vh] overflow-auto whitespace-pre-wrap rounded-lg border border-white/10 bg-black/30 p-3 text-[11px] text-foreground/90">
                    {JSON.stringify(instance, null, 2)}
                </pre>
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
            <div className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-96 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    return (
        <div className="space-y-4">
            <div>
                <h1 className="text-3xl font-semibold tracking-tight">Custom Resources</h1>
                <p className="text-sm text-muted-foreground">Browse any CustomResourceDefinition installed in the cluster.</p>
            </div>

            <div className="grid gap-4 lg:grid-cols-[320px_1fr]">
                <GlassCard className="p-0 overflow-hidden">
                    <div className="border-b border-white/5 p-3">
                        <div className="relative">
                            <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                            <Input
                                placeholder="Search CRDs"
                                value={q}
                                onChange={(e) => setQ(e.target.value)}
                                className="border-white/10 bg-white/[0.03] pl-9"
                            />
                        </div>
                    </div>
                    <div className="max-h-[65vh] overflow-auto scrollbar-thin">
                        {filtered.length === 0 && <div className="px-4 py-8 text-center text-sm text-muted-foreground">No CRDs match.</div>}
                        {filtered.map((c) => {
                            const active = selected && selected.group === c.group && selected.plural === c.plural;
                            return (
                                <button
                                    key={`${c.group}/${c.version}/${c.plural}`}
                                    onClick={() => {
                                        setSelected(c);
                                        setDetail(null);
                                    }}
                                    className={`flex w-full items-center gap-3 border-b border-white/5 px-4 py-3 text-left last:border-none hover:bg-white/[0.03] ${active ? "bg-white/[0.06]" : ""}`}
                                >
                                    <Layers className="h-4 w-4 shrink-0 text-[#b49bd8]" />
                                    <div className="min-w-0 flex-1">
                                        <div className="mono truncate text-sm">{c.kind}</div>
                                        <div className="truncate text-[11px] text-muted-foreground">
                                            {c.group}/{c.version}
                                        </div>
                                    </div>
                                    <Badge variant="outline" className="border-white/10 bg-white/[0.03] text-[10px]">
                                        {c.scope}
                                    </Badge>
                                </button>
                            );
                        })}
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
                    <GlassCard className="flex items-center justify-center p-16 text-sm text-muted-foreground">
                        Select a CRD to browse its instances.
                    </GlassCard>
                )}
            </div>

            {detail && <InstanceDetail crd={selected} instance={detail} onOpenChange={(o) => !o && setDetail(null)} />}
        </div>
    );
}
