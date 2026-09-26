import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import AutoTable from "@/components/AutoTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Search, Lock, LockOpen, Waypoints, Router } from "lucide-react";

export default function Ingresses() {
    const [data, setData] = useState(null);
    const [gateways, setGateways] = useState(null);
    const [q, setQ] = useState("");

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/ingresses").then((r) => alive && setData(r.data));
        load();
        api.get("/gateways").then((r) => alive && setGateways(r.data || [])).catch(() => alive && setGateways([]));
        const t = setInterval(load, 15000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    const rows = useMemo(
        () => (data || []).filter((i) => q === "" || i.name.toLowerCase().includes(q.toLowerCase()) || (i.hosts || []).some((h) => h.toLowerCase().includes(q.toLowerCase()))),
        [data, q]
    );

    if (!data) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title="Ingresses"
                description="External routes into cluster services, equivalent to OpenShift Routes."
                tools={
                    <div className="input">
                        <Search size={15} className="text-ink-3" />
                        <input
                            placeholder="Search by name or host"
                            value={q}
                            onChange={(e) => setQ(e.target.value)} className="bare-input"
                        />
                    </div>
                }
            />

            <GlassCard className="overflow-hidden p-0">
                <div className="max-h-[65vh] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label="Ingresses">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Namespace</th>
                                <th>Class</th>
                                <th>Hosts</th>
                                <th>Address</th>
                                <th>TLS</th>
                                <th>Age</th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((i) => (
                                <tr key={`${i.namespace}/${i.name}`}>
                                    <td className="name">
                                        <span className="flex gap-2 items-center">
                                            <Waypoints size={14} className="text-ink-3" /> {i.name}
                                        </span>
                                    </td>
                                    <td className="mono">{i.namespace}</td>
                                    <td>{i.class || "—"}</td>
                                    <td>
                                        <div className="flex flex-wrap gap-1">
                                            {(i.hosts || []).map((h) => (
                                                <Badge key={h} variant="outline" className="mono text-[10px] bg-muted/30">
                                                    {h}
                                                </Badge>
                                            ))}
                                        </div>
                                    </td>
                                    <td className="mono num">{i.address || "—"}</td>
                                    <td>
                                        {i.tls ? (
                                            <Lock size={14} className="text-tone-green" />
                                        ) : (
                                            <LockOpen size={14} className="text-ink-3" />
                                        )}
                                    </td>
                                    <td className="mono">{i.age}</td>
                                </tr>
                            ))}
                            {rows.length === 0 && (
                                <tr>
                                    <td colSpan={7}>
                                        <div className="empty">No ingresses match the current filters.</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>

            <div className="mt-6">
                <AutoTable
                    title="Gateway API"
                    icon={Router}
                    items={gateways}
                    emptyHint={gateways === null ? "Loading…" : "No Gateway API objects found (GatewayClass/Gateway/HTTPRoute)."}
                />
            </div>
        </div>
    );
}
