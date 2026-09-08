import { useEffect, useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import StatusBadge from "@/components/StatusBadge";
import SpecTable from "@/components/SpecTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { ArrowLeft, Tag, FileJson, Boxes, BellRing, Save } from "lucide-react";
import { toast } from "sonner";
import CodeEditor from "@/components/CodeEditor";
import KvChip, { toneForKey } from "@/components/KvChip";

export default function WorkloadDetail() {
    const { namespace, kind, name } = useParams();
    const navigate = useNavigate();
    const [detail, setDetail] = useState(null);
    const [error, setError] = useState(null);
    const [manifestText, setManifestText] = useState("");
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        let alive = true;
        api
            .get(`/resources/${namespace}/${kind}/${encodeURIComponent(name)}`)
            .then((r) => {
                if (!alive) return;
                setDetail(r.data);
                setManifestText(JSON.stringify(r.data.manifest, null, 2));
            })
            .catch((e) => alive && setError(e?.response?.data?.error || e.message));
        return () => {
            alive = false;
        };
    }, [namespace, kind, name]);

    const load = () => {
        api.get(`/resources/${namespace}/${kind}/${encodeURIComponent(name)}`).then((r) => {
            setDetail(r.data);
            setManifestText(JSON.stringify(r.data.manifest, null, 2));
        });
    };

    const saveManifest = async () => {
        setSaving(true);
        try {
            const { data } = await api.post("/apply", { manifest: manifestText });
            if (!data.valid) {
                toast.error("Apply failed", { description: (data.errors || [data.message]).filter(Boolean).join(", ") });
                return;
            }
            toast.success(`Applied ${data.kind || kind}`);
            load();
        } catch (e) {
            toast.error("Apply failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setSaving(false);
        }
    };

    if (error) {
        return (
            <div className="page space-y-6">
                <button onClick={() => navigate(-1)} className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
                    <ArrowLeft className="h-4 w-4" /> Back
                </button>
                <GlassCard className="p-6 text-center text-sm text-muted-foreground">{error}</GlassCard>
            </div>
        );
    }

    if (!detail) return (
        <div className="page">
            <Skeleton className="h-96 rounded-2xl bg-muted" />
        </div>
    );

    return (
        <div className="page space-y-6">
            <button onClick={() => navigate(-1)} className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
                <ArrowLeft className="h-4 w-4" /> Back
            </button>

            <DetailHeader eyebrow={kind} name={detail.name} meta={<span>{detail.namespace}</span>} />

            <Tabs defaultValue="overview">
                <TabsList className="tabs">
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
                        <div className="mb-3 text-sm font-medium text-violet-200">Labels</div>
                        <div className="flex flex-wrap gap-1.5">
                            {Object.entries(detail.labels || {}).map(([k, v]) => (
                                <KvChip key={k} name={k} value={v} tone={toneForKey(k)} />
                            ))}
                            {Object.keys(detail.labels || {}).length === 0 && <span className="text-xs text-muted-foreground">No labels.</span>}
                        </div>
                    </GlassCard>
                    {Object.keys(detail.annotations || {}).length > 0 ? (
                        <SpecTable
                            title="Annotations"
                            rows={Object.entries(detail.annotations).map(([k, v]) => ({ label: k, value: String(v), mono: true }))}
                        />
                    ) : (
                        <GlassCard className="p-5">
                            <div className="mb-1 text-sm font-medium">Annotations</div>
                            <span className="text-xs text-muted-foreground">No annotations.</span>
                        </GlassCard>
                    )}
                    {(detail.containerPorts || []).length > 0 && (
                        <GlassCard className="p-5">
                            <div className="mb-3 text-sm font-medium">Container ports</div>
                            <div className="flex flex-wrap gap-1.5">
                                {detail.containerPorts.map((p, i) => (
                                    <Badge key={i} variant="outline" className="mono text-[11px] bg-muted/30">
                                        {p.name ? `${p.name}: ` : ""}
                                        {p.containerPort}/{p.protocol || "TCP"}
                                    </Badge>
                                ))}
                            </div>
                        </GlassCard>
                    )}
                </TabsContent>

                <TabsContent value="yaml" className="mt-4 space-y-3">
                    <CodeEditor value={manifestText} onChange={setManifestText} />
                    <div className="flex justify-end">
                        <Button variant="brand" onClick={saveManifest} disabled={saving} className="rounded-xl">
                            <Save className="mr-1.5 h-3.5 w-3.5" /> {saving ? "Saving…" : "Save"}
                        </Button>
                    </div>
                </TabsContent>

                <TabsContent value="pods" className="mt-4">
                    <GlassCard className="overflow-hidden">
                        {(detail.pods || []).length === 0 && (
                            <div className="px-4 py-8 text-center text-sm text-muted-foreground">No pods found for this workload.</div>
                        )}
                        {(detail.pods || []).map((p) => (
                            <Link
                                key={p.name}
                                to={`/pods/${p.namespace}/${encodeURIComponent(p.name)}`}
                                className="grid grid-cols-12 items-center gap-3 border-b px-4 py-3 last:border-none hover:bg-muted"
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
                    <GlassCard className="overflow-hidden">
                        {(detail.relatedEvents || []).length === 0 && (
                            <div className="px-4 py-8 text-center text-sm text-muted-foreground">No related events.</div>
                        )}
                        {(detail.relatedEvents || []).map((e, i) => (
                            <div key={i} className="grid grid-cols-12 items-center gap-3 border-b px-4 py-3 last:border-none">
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
