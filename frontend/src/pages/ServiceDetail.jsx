import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import SpecTable from "@/components/SpecTable";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
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
            <div className="space-y-4">
                <Skeleton className="h-20 rounded-2xl bg-muted" />
                <Skeleton className="h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    if (svc === false) {
        return (
            <div className="space-y-4">
                <Button variant="ghost" size="sm" onClick={() => navigate("/services")} className="gap-1.5">
                    <ArrowLeft className="h-3.5 w-3.5" /> Services
                </Button>
                <GlassCard className="p-8 text-center text-sm text-muted-foreground">
                    Service <span className="mono text-foreground">{ns}/{nm}</span> was not found.
                </GlassCard>
            </div>
        );
    }

    return (
        <div className="space-y-6">
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
                    <Button variant="outline" size="sm" onClick={() => navigate("/services")} className="rounded-xl gap-1.5">
                        <ArrowLeft className="h-3.5 w-3.5" /> All services
                    </Button>
                }
            />

            <SpecTable
                title="Ports"
                columns={1}
                rows={(svc.ports || []).map((p, i) => ({
                    label: p.name || `port-${i}`,
                    value: `${p.port}${p.targetPort ? ` → ${p.targetPort}` : ""} / ${p.protocol || "TCP"}`,
                    mono: true,
                }))}
            />

            {svc.selector && Object.keys(svc.selector).length > 0 && (
                <SpecTable
                    title="Selector"
                    rows={Object.entries(svc.selector).map(([k, v]) => ({ label: k, value: v, mono: true }))}
                />
            )}

            <GlassCard className="overflow-hidden p-0">
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

            <GlassCard className="overflow-hidden p-0">
                <div className="border-b px-4 py-3 text-sm font-medium">Matching pods ({matchingPods.length})</div>
                {matchingPods.length === 0 ? (
                    <div className="px-4 py-8 text-center text-sm text-muted-foreground">No pods match this selector.</div>
                ) : (
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead>Name</TableHead>
                                <TableHead>Status</TableHead>
                                <TableHead>IP</TableHead>
                                <TableHead>Node</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {matchingPods.map((p) => (
                                <TableRow
                                    key={p.name}
                                    className="cursor-pointer hover:bg-muted"
                                    onClick={() => navigate(`/pods/${encodeURIComponent(ns)}/${encodeURIComponent(p.name)}`)}
                                >
                                    <TableCell className="mono text-sm">{p.name}</TableCell>
                                    <TableCell>
                                        <StatusBadge status={p.status} />
                                    </TableCell>
                                    <TableCell className="mono text-xs text-muted-foreground">{p.ip || "—"}</TableCell>
                                    <TableCell className="mono text-xs text-muted-foreground">{p.node || "—"}</TableCell>
                                </TableRow>
                            ))}
                        </TableBody>
                    </Table>
                )}
            </GlassCard>
        </div>
    );
}
