import { useEffect, useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { ArrowLeft, Tag, FileJson, Boxes, BellRing } from "lucide-react";

export default function WorkloadDetail() {
    const { namespace, kind, name } = useParams();
    const navigate = useNavigate();
    const [detail, setDetail] = useState(null);
    const [error, setError] = useState(null);

    useEffect(() => {
        let alive = true;
        api
            .get(`/resources/${namespace}/${kind}/${encodeURIComponent(name)}`)
            .then((r) => alive && setDetail(r.data))
            .catch((e) => alive && setError(e?.response?.data?.error || e.message));
        return () => {
            alive = false;
        };
    }, [namespace, kind, name]);

    if (error) {
        return (
            <div className="space-y-4">
                <button onClick={() => navigate(-1)} className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
                    <ArrowLeft className="h-4 w-4" /> Back
                </button>
                <GlassCard className="p-6 text-center text-sm text-muted-foreground">{error}</GlassCard>
            </div>
        );
    }

    if (!detail) return <Skeleton className="h-96 rounded-2xl bg-white/[0.04]" />;

    return (
        <div className="space-y-4">
            <button onClick={() => navigate(-1)} className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
                <ArrowLeft className="h-4 w-4" /> Back
            </button>

            <div>
                <div className="text-xs uppercase tracking-widest text-muted-foreground">{kind}</div>
                <h1 className="mono text-3xl font-semibold tracking-tight">{detail.name}</h1>
                <p className="text-sm text-muted-foreground">{detail.namespace}</p>
            </div>

            <Tabs defaultValue="overview">
                <TabsList className="bg-white/[0.03] p-1">
                    <TabsTrigger value="overview" className="gap-1.5">
                        <Tag className="h-3.5 w-3.5" /> Overview
                    </TabsTrigger>
                    <TabsTrigger value="yaml" className="gap-1.5">
                        <FileJson className="h-3.5 w-3.5" /> YAML
                    </TabsTrigger>
                    <TabsTrigger value="pods" className="gap-1.5">
                        <Boxes className="h-3.5 w-3.5" /> Pods ({(detail.pods || []).length})
                    </TabsTrigger>
                    <TabsTrigger value="events" className="gap-1.5">
                        <BellRing className="h-3.5 w-3.5" /> Events ({(detail.relatedEvents || []).length})
                    </TabsTrigger>
                </TabsList>

                <TabsContent value="overview" className="mt-4 space-y-4">
                    <GlassCard className="p-5">
                        <div className="mb-3 text-sm font-medium">Labels</div>
                        <div className="flex flex-wrap gap-1.5">
                            {Object.entries(detail.labels || {}).map(([k, v]) => (
                                <Badge key={k} variant="outline" className="mono text-[11px] border-white/10 bg-white/[0.03]">
                                    {k}={v}
                                </Badge>
                            ))}
                            {Object.keys(detail.labels || {}).length === 0 && <span className="text-xs text-muted-foreground">No labels.</span>}
                        </div>
                    </GlassCard>
                    <GlassCard className="p-5">
                        <div className="mb-3 text-sm font-medium">Annotations</div>
                        <div className="space-y-1.5">
                            {Object.entries(detail.annotations || {}).map(([k, v]) => (
                                <div key={k} className="mono text-xs">
                                    <span className="text-muted-foreground">{k}:</span> <span className="truncate">{String(v)}</span>
                                </div>
                            ))}
                            {Object.keys(detail.annotations || {}).length === 0 && <span className="text-xs text-muted-foreground">No annotations.</span>}
                        </div>
                    </GlassCard>
                    {(detail.containerPorts || []).length > 0 && (
                        <GlassCard className="p-5">
                            <div className="mb-3 text-sm font-medium">Container ports</div>
                            <div className="flex flex-wrap gap-1.5">
                                {detail.containerPorts.map((p, i) => (
                                    <Badge key={i} variant="outline" className="mono text-[11px] border-white/10 bg-white/[0.03]">
                                        {p.name ? `${p.name}: ` : ""}
                                        {p.containerPort}/{p.protocol || "TCP"}
                                    </Badge>
                                ))}
                            </div>
                        </GlassCard>
                    )}
                </TabsContent>

                <TabsContent value="yaml" className="mt-4">
                    <GlassCard className="p-0 overflow-hidden">
                        <pre className="mono max-h-[65vh] overflow-auto whitespace-pre-wrap p-4 text-[12px] text-foreground/90">
                            {JSON.stringify(detail.manifest, null, 2)}
                        </pre>
                    </GlassCard>
                </TabsContent>

                <TabsContent value="pods" className="mt-4">
                    <GlassCard className="p-0 overflow-hidden">
                        {(detail.pods || []).length === 0 && (
                            <div className="px-4 py-8 text-center text-sm text-muted-foreground">No pods found for this workload.</div>
                        )}
                        {(detail.pods || []).map((p) => (
                            <Link
                                key={p.name}
                                to={`/pods/${p.namespace}/${encodeURIComponent(p.name)}`}
                                className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]"
                            >
                                <div className="col-span-4 mono truncate text-sm">{p.name}</div>
                                <div className="col-span-2 mono text-xs text-muted-foreground">{p.node}</div>
                                <div className="col-span-2 mono text-xs">{p.ready}</div>
                                <div className="col-span-2">
                                    <StatusBadge status={p.status} />
                                </div>
                                <div className="col-span-2 text-right text-xs text-muted-foreground">{p.age}</div>
                            </Link>
                        ))}
                    </GlassCard>
                </TabsContent>

                <TabsContent value="events" className="mt-4">
                    <GlassCard className="p-0 overflow-hidden">
                        {(detail.relatedEvents || []).length === 0 && (
                            <div className="px-4 py-8 text-center text-sm text-muted-foreground">No related events.</div>
                        )}
                        {(detail.relatedEvents || []).map((e, i) => (
                            <div key={i} className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none">
                                <div className="col-span-2">
                                    <StatusBadge status={e.type} />
                                </div>
                                <div className="col-span-3 mono text-xs text-sky-300">{e.reason}</div>
                                <div className="col-span-7 text-sm text-foreground/90">{e.message}</div>
                            </div>
                        ))}
                    </GlassCard>
                </TabsContent>
            </Tabs>
        </div>
    );
}
