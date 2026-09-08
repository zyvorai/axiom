import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Search } from "lucide-react";

export default function Services() {
    const navigate = useNavigate();
    const [data, setData] = useState(null);
    const [q, setQ] = useState("");

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/services").then((r) => alive && setData(r.data));
        load();
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    const rows = useMemo(
        () => (data || []).filter((s) => q === "" || s.name.toLowerCase().includes(q.toLowerCase())),
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
                title="Services"
                description="ClusterIP, NodePort and LoadBalancer services across all namespaces."
                tools={
                    <div className="input">
                        <Search size={15} style={{ color: "var(--ink-3)" }} />
                        <input
                            placeholder="Search service name"
                            value={q}
                            onChange={(e) => setQ(e.target.value)}
                            style={{ background: "transparent", border: "none", outline: "none", color: "inherit", flex: 1, font: "inherit" }}
                        />
                    </div>
                }
            />

            <GlassCard className="overflow-hidden">
                <div style={{ overflowX: "auto" }} className="max-h-[65vh] overflow-y-auto scrollbar-thin">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Namespace</th>
                                <th>Type</th>
                                <th>Cluster IP</th>
                                <th>Ports</th>
                                <th>Endpoints</th>
                                <th>Age</th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((s) => (
                                <tr
                                    key={`${s.namespace}/${s.name}`}
                                    className="cursor-pointer"
                                    onClick={() => navigate(`/services/${encodeURIComponent(s.namespace)}/${encodeURIComponent(s.name)}`)}
                                >
                                    <td className="name">
                                        <Link
                                            to={`/services/${encodeURIComponent(s.namespace)}/${encodeURIComponent(s.name)}`}
                                            className="hover:underline"
                                            onClick={(e) => e.stopPropagation()}
                                        >
                                            {s.name}
                                        </Link>
                                    </td>
                                    <td className="mono">{s.namespace}</td>
                                    <td>{s.type}</td>
                                    <td className="mono">{s.clusterIP}</td>
                                    <td>
                                        <div className="flex flex-wrap gap-1">
                                            {(s.ports || []).map((p, i) => (
                                                <Badge key={i} variant="outline" className="mono text-[10px] bg-muted/30">
                                                    {p.port}
                                                    {p.targetPort ? `→${p.targetPort}` : ""}/{p.protocol || "TCP"}
                                                </Badge>
                                            ))}
                                        </div>
                                    </td>
                                    <td className="mono num">{(s.endpoints || []).length}</td>
                                    <td className="mono">{s.age}</td>
                                </tr>
                            ))}
                            {rows.length === 0 && (
                                <tr>
                                    <td colSpan={7}>
                                        <div className="empty">No services match the current filters.</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>
        </div>
    );
}
