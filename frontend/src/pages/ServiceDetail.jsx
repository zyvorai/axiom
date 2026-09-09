import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import SpecTable from "@/components/SpecTable";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { ArrowLeft, Network } from "lucide-react";

function labelsMatch(podLabels, selector) {
    if (!selector || !Object.keys(selector).length) return false;
    for (const [k, v] of Object.entries(selector)) {
        if ((podLabels || {})[k] !== v) return false;
    }
    return true;
}

export default function ServiceDetail() {
    const { namespace, name } = useParams();
    const navigate = useNavigate();
    const ns = decodeURIComponent(namespace || "");
    const nm = decodeURIComponent(name || "");
    const [svc, setSvc] = useState(null);
    const [pods, setPods] = useState([]);

    useEffect(() => {
        let alive = true;
        const q = `?namespace=${encodeURIComponent(ns)}`;
        Promise.all([
            api.get(`/services${q}`).catch(() => ({ data: [] })),
            api.get(`/pods${q}`).catch(() => ({ data: [] })),
        ]).then(([sRes, pRes]) => {
            if (!alive) return;
            const found = (sRes.data || []).find((s) => s.name === nm);
            setSvc(found || false);
            setPods(pRes.data || []);
        });
        return () => {
            alive = false;
        };
    }, [ns, nm]);

    const matchingPods = useMemo(() => {
        if (!svc?.selector) return [];
        return pods.filter((p) => labelsMatch(p.labels, svc.selector));
    }, [pods, svc]);

    if (svc === null) {
        return (
            <div className="page space-y-4">
                <Skeleton className="h-20 rounded-2xl bg-muted" />
                <Skeleton className="h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    if (svc === false) {
        return (
            <div className="page space-y-4">
                <button type="button" className="btn btn-line btn-sm" onClick={() => navigate("/services")}>
                    <ArrowLeft className="h-3.5 w-3.5" /> Services
                </button>
                <GlassCard className="empty">
                    Service <span className="mono text-foreground">{ns}/{nm}</span> was not found.
                </GlassCard>
            </div>
        );
    }

    return (
        <div className="page space-y-6">
            <DetailHeader
                eyebrow="Service"
                name={nm}
                meta={
                    <>
                        <Link to={`/namespaces/${encodeURIComponent(ns)}`} className="mono text-primary hover:underline">
                            {ns}
                        </Link>
                        <span>·</span>
                        <Badge variant="secondary">{svc.type}</Badge>
                        <span>·</span>
                        <span className="mono">{svc.clusterIP || "—"}</span>
                        <span>·</span>
                        <span>{svc.age || "—"}</span>
                    </>
                }
                action={
                    <button type="button" className="btn btn-line btn-sm" onClick={() => navigate("/services")}>
                        <ArrowLeft className="h-3.5 w-3.5" /> All services
                    </button>
                }
            />

            <GlassCard>
                <SpecTable
                    title="Ports"
                    columns={1}
                    rows={(svc.ports || []).map((p, i) => ({
                        label: p.name || `port-${i}`,
                        value: `${p.port}${p.targetPort ? ` → ${p.targetPort}` : ""} / ${p.protocol || "TCP"}`,
                        mono: true,
                    }))}
                />
            </GlassCard>

            {svc.selector && Object.keys(svc.selector).length > 0 && (
                <GlassCard>
                    <SpecTable
                        title="Selector"
                        rows={Object.entries(svc.selector).map(([k, v]) => ({ label: k, value: v, mono: true }))}
                    />
                </GlassCard>
            )}

            <GlassCard className="overflow-hidden">
                <div className="flex items-center gap-2 border-b px-4 py-3 text-sm font-medium">
                    <Network className="h-4 w-4 text-i17-cosmic" /> Endpoints ({(svc.endpoints || []).length})
                </div>
                {(svc.endpoints || []).length === 0 ? (
                    <div className="px-4 py-8 text-center text-sm text-muted-foreground">No endpoints.</div>
                ) : (
                    <div className="flex flex-wrap gap-2 p-4">
                        {(svc.endpoints || []).map((ep) => (
                            <Badge key={ep} variant="outline" className="mono text-[11px]">
                                {ep}
                            </Badge>
                        ))}
                    </div>
                )}
            </GlassCard>

            <GlassCard className="overflow-hidden">
                <div className="card-h">
                    <h2 style={{ fontSize: 15 }}>Matching pods ({matchingPods.length})</h2>
                </div>
                {matchingPods.length === 0 ? (
                    <div className="empty">No pods match this selector.</div>
                ) : (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Status</th>
                                <th>IP</th>
                                <th>Node</th>
                            </tr>
                        </thead>
                        <tbody>
                            {matchingPods.map((p) => (
                                <tr
                                    key={p.name}
                                    style={{ cursor: "pointer" }}
                                    onClick={() => navigate(`/pods/${encodeURIComponent(ns)}/${encodeURIComponent(p.name)}`)}
                                >
                                    <td className="name mono">{p.name}</td>
                                    <td>
                                        <StatusBadge status={p.status} />
                                    </td>
                                    <td className="mono num">{p.ip || "—"}</td>
                                    <td className="mono num">{p.node || "—"}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            </GlassCard>
        </div>
    );
}
