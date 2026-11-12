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
import { ArrowLeft, Tag, FileJson, Boxes, BellRing, Save, Network, History, Activity, Lock, Unlock } from "lucide-react";
import WorkloadTimeline from "@/components/WorkloadTimeline";
import { TIMELINE } from "@/constants/testIds";
import CodeEditor from "@/components/CodeEditor";
import KvChip, { toneForKey } from "@/components/KvChip";
import FeatureTile from "@/components/FeatureTile";
import DiffReviewDialog from "@/components/DiffReviewDialog";
import ResourceGraph from "@/components/ResourceGraph";
import MetricHistoryChart from "@/components/MetricHistoryChart";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

export default function WorkloadDetail() {
    const { t } = useTranslation();
    const { namespace, kind, name } = useParams();
    const navigate = useNavigate();
    const [detail, setDetail] = useState(null);
    const [error, setError] = useState(null);
    const [manifestText, setManifestText] = useState("");
    const [reviewOpen, setReviewOpen] = useState(false);
    const [summary, setSummary] = useState(null);
    const [guard, setGuard] = useState(null);
    const [guardBusy, setGuardBusy] = useState(false);

    // Replica/status summary comes from the workloads list (the detail
    // endpoint only returns the manifest). Best-effort: the page works without it.
    useEffect(() => {
        let alive = true;
        api
            .get("/workloads")
            .then((r) => {
                if (!alive) return;
                const row = (r.data || []).find((w) => w.namespace === namespace && w.name === name && w.kind === kind);
                setSummary(row || null);
            })
            .catch(() => {});
        return () => {
            alive = false;
        };
    }, [namespace, kind, name]);

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
        api
            .get(`/workloads/${namespace}/${kind}/${encodeURIComponent(name)}/network/guard`)
            .then((r) => alive && setGuard(r.data))
            .catch(() => alive && setGuard(null));
        return () => {
            alive = false;
        };
    }, [namespace, kind, name]);

    const sealToggle = async (seal) => {
        setGuardBusy(true);
        try {
            await api.post(`/workloads/${namespace}/${kind}/${encodeURIComponent(name)}/${seal ? "seal" : "unseal"}`);
            toast.success(t("toasts:workloads.workloadActionRequested", { name, action: seal ? "seal" : "unseal" }));
            const { data } = await api.get(`/workloads/${namespace}/${kind}/${encodeURIComponent(name)}/network/guard`);
            setGuard(data);
        } catch (e) {
            toast.error(t("toasts:workloads.actionFailed", { action: seal ? "seal" : "unseal" }), {
                description: e?.response?.data?.error || e.message,
            });
        } finally {
            setGuardBusy(false);
        }
    };

    const load = () => {
        api.get(`/resources/${namespace}/${kind}/${encodeURIComponent(name)}`).then((r) => {
            setDetail(r.data);
            setManifestText(JSON.stringify(r.data.manifest, null, 2));
        });
    };

    if (error) {
        return (
            <div className="page space-y-6">
                <button onClick={() => navigate(-1)} className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
                    <ArrowLeft className="h-4 w-4" /> {t("common:actions.back")}
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
                <ArrowLeft className="h-4 w-4" /> {t("common:actions.back")}
            </button>

            <DetailHeader
                eyebrow={`${kind} · ${detail.namespace}`}
                name={detail.name}
                meta={
                    summary ? (
                        <>
                            <StatusBadge status={summary.status} />
                            {summary.image && (
                                <>
                                    <span>·</span>
                                    <span className="mono">{summary.image}</span>
                                </>
                            )}
                            <span>·</span>
                            <span>{summary.age}</span>
                        </>
                    ) : (
                        <span>{detail.namespace}</span>
                    )
                }
            />

            {summary && (
                <div className="stats">
                    <FeatureTile size="compact" mono label={t("pages:workloadDetail.stats.desired")} value={summary.desired} />
                    <FeatureTile size="compact" mono label={t("common:fields.ready")} value={`${summary.ready}/${summary.desired}`} />
                    <FeatureTile size="compact" mono label={t("pages:workloadDetail.stats.available")} value={summary.available} />
                    <FeatureTile size="compact" mono label={t("pages:workloadDetail.stats.pods")} value={(detail.pods || []).length} />
                </div>
            )}

            <Tabs defaultValue="overview">
                <TabsList className="tabs">
                    <TabsTrigger value="overview" className="gap-1.5">
                        <Tag className="h-3.5 w-3.5" /> {t("pages:workloadDetail.tabs.overview")}
                    </TabsTrigger>
                    <TabsTrigger value="yaml" className="gap-1.5">
                        <FileJson className="h-3.5 w-3.5" /> YAML
                    </TabsTrigger>
                    <TabsTrigger value="pods" className="gap-1.5">
                        <Boxes className="h-3.5 w-3.5" /> {t("pages:workloadDetail.tabs.pods", { n: (detail.pods || []).length })}
                    </TabsTrigger>
                    <TabsTrigger value="metrics" className="gap-1.5">
                        <Activity className="h-3.5 w-3.5" /> {t("pages:metrics.title")}
                    </TabsTrigger>
                    <TabsTrigger value="events" className="gap-1.5">
                        <BellRing className="h-3.5 w-3.5" /> {t("pages:workloadDetail.tabs.events", { n: (detail.relatedEvents || []).length })}
                    </TabsTrigger>
                    <TabsTrigger value="graph" className="gap-1.5">
                        <Network className="h-3.5 w-3.5" aria-hidden="true" /> {t("pages:workloadDetail.tabs.graph")}
                    </TabsTrigger>
                    <TabsTrigger value="changes" data-testid={TIMELINE.workloadTab} className="gap-1.5">
                        <History className="h-3.5 w-3.5" /> {t("pages:workloadDetail.tabs.changes")}
                    </TabsTrigger>
                </TabsList>

                <TabsContent value="overview" className="mt-4 space-y-4">
                    {guard && (
                        <GlassCard className="flex flex-wrap items-center justify-between gap-3 p-5">
                            <div>
                                <div className="text-sm font-medium">{t("pages:workloadDetail.networkGuard.title")}</div>
                                <div className="mt-1 text-[12.5px] text-ink-3">
                                    {guard.message || t("pages:workloadDetail.networkGuard.flows", { n: guard.flowsCount || 0 })}
                                    {guard.configured === false ? ` · ${t("pages:workloadDetail.networkGuard.notConfigured")}` : null}
                                </div>
                            </div>
                            <div className="flex gap-2">
                                <Button size="sm" variant="outline" disabled={guardBusy} onClick={() => sealToggle(true)}>
                                    <Lock className="mr-1.5 h-3.5 w-3.5" /> {t("pages:workloads.menu.seal")}
                                </Button>
                                <Button size="sm" variant="outline" disabled={guardBusy} onClick={() => sealToggle(false)}>
                                    <Unlock className="mr-1.5 h-3.5 w-3.5" /> {t("pages:workloads.menu.unseal")}
                                </Button>
                            </div>
                        </GlassCard>
                    )}
                    <GlassCard className="p-5">
                        <div className="mb-3 text-sm font-medium">{t("common:fields.labels")}</div>
                        <div className="flex flex-wrap gap-1.5">
                            {Object.entries(detail.labels || {}).map(([k, v]) => (
                                <KvChip key={k} name={k} value={v} tone={toneForKey(k)} />
                            ))}
                            {Object.keys(detail.labels || {}).length === 0 && <span className="text-xs text-muted-foreground">{t("pages:workloadDetail.noLabels")}</span>}
                        </div>
                    </GlassCard>
                    {Object.keys(detail.annotations || {}).length > 0 ? (
                        <SpecTable
                            title={t("common:fields.annotations")}
                            rows={Object.entries(detail.annotations).map(([k, v]) => ({ label: k, value: String(v), mono: true }))}
                        />
                    ) : (
                        <GlassCard className="p-5">
                            <div className="mb-1 text-sm font-medium">{t("common:fields.annotations")}</div>
                            <span className="text-xs text-muted-foreground">{t("pages:workloadDetail.noAnnotations")}</span>
                        </GlassCard>
                    )}
                    {(detail.containerPorts || []).length > 0 && (
                        <GlassCard className="p-5">
                            <div className="mb-3 text-sm font-medium">{t("pages:workloadDetail.containerPorts")}</div>
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
                        <Button variant="brand" onClick={() => setReviewOpen(true)} disabled={!manifestText.trim()} className="rounded-xl">
                            <Save className="mr-1.5 h-3.5 w-3.5" /> {t("common:actions.save")}
                        </Button>
                    </div>
                    <DiffReviewDialog
                        open={reviewOpen}
                        onOpenChange={setReviewOpen}
                        manifest={manifestText}
                        fallbackKind={kind}
                        onApplied={load}
                    />
                </TabsContent>

                <TabsContent value="pods" className="mt-4">
                    <GlassCard className="overflow-hidden">
                        {(detail.pods || []).length === 0 && (
                            <div className="px-4 py-8 text-center text-sm text-muted-foreground">{t("pages:workloadDetail.noPods")}</div>
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

                <TabsContent value="metrics" className="mt-4">
                    <GlassCard className="p-4">
                        <MetricHistoryChart namespace={namespace} name={name} kind="workload" window="1h" />
                    </GlassCard>
                </TabsContent>

                <TabsContent value="events" className="mt-4">
                    <GlassCard className="overflow-hidden">
                        {(detail.relatedEvents || []).length === 0 && (
                            <div className="px-4 py-8 text-center text-sm text-muted-foreground">{t("pages:workloadDetail.noEvents")}</div>
                        )}
                        {(detail.relatedEvents || []).map((e, i) => (
                            <div key={i} className="grid grid-cols-12 items-center gap-3 border-b px-4 py-3 last:border-none">
                                <div className="col-span-2">
                                    <StatusBadge status={e.type} />
                                </div>
                                <div className="col-span-3 mono text-xs text-tone-link">{e.reason}</div>
                                <div className="col-span-7 text-sm text-foreground">{e.message}</div>
                            </div>
                        ))}
                    </GlassCard>
                </TabsContent>
                <TabsContent value="graph" className="mt-4">
                    <ResourceGraph namespace={namespace} kind={kind} name={name} />
                </TabsContent>

                <TabsContent value="changes" className="mt-4">
                    <WorkloadTimeline namespace={namespace} kind={kind} name={name} />
                </TabsContent>
            </Tabs>
        </div>
    );
}
