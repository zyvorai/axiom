import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import StatusBadge from "@/components/StatusBadge";
import FeatureTile from "@/components/FeatureTile";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { ArrowLeft, Box, Network, Layers } from "lucide-react";
import { useTranslation, Trans } from "react-i18next";

function Empty({ hint }) {
    return <div className="px-4 py-10 text-center text-sm text-muted-foreground">{hint}</div>;
}

export default function NamespaceDetail() {
    const { t } = useTranslation();
    const { name } = useParams();
    const navigate = useNavigate();
    const ns = decodeURIComponent(name || "");

    const [meta, setMeta] = useState(null);
    const [pods, setPods] = useState(null);
    const [workloads, setWorkloads] = useState(null);
    const [services, setServices] = useState(null);

    useEffect(() => {
        let alive = true;
        const q = `?namespace=${encodeURIComponent(ns)}`;
        const load = () => {
            Promise.all([
                api.get("/namespaces").catch(() => ({ data: [] })),
                api.get(`/pods${q}`).catch(() => ({ data: [] })),
                api.get(`/workloads${q}`).catch(() => ({ data: [] })),
                api.get(`/services${q}`).catch(() => ({ data: [] })),
            ]).then(([nsRes, podRes, wlRes, svcRes]) => {
                if (!alive) return;
                const found = (nsRes.data || []).find((n) => n.name === ns) || null;
                setMeta(found === null ? false : found);
                setPods(podRes.data || []);
                setWorkloads(wlRes.data || []);
                setServices(svcRes.data || []);
            });
        };
        load();
        const timer = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, [ns]);

    const runningPods = useMemo(
        () => (pods || []).filter((p) => (p.status || "").toLowerCase() === "running").length,
        [pods]
    );

    if (meta === null || pods === null || workloads === null || services === null) {
        return (
            <div className="page space-y-4">
                <Skeleton className="h-20 rounded-2xl bg-muted" />
                <Skeleton className="h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    if (meta === false) {
        return (
            <div className="page space-y-6">
                <Button variant="ghost" size="sm" onClick={() => navigate("/namespaces")} className="gap-1.5">
                    <ArrowLeft className="h-3.5 w-3.5" /> {t("pages:namespaces.title")}
                </Button>
                <GlassCard className="p-8 text-center text-sm text-muted-foreground">
                    <Trans i18nKey="pages:namespaceDetail.notFound" values={{ name: ns }} components={{ mono: <span className="mono text-foreground" /> }} />
                </GlassCard>
            </div>
        );
    }

    return (
        <div className="page space-y-6">
            <DetailHeader
                eyebrow={t("common:fields.namespace")}
                name={ns}
                meta={
                    <>
                        <StatusBadge status={meta.status || "Active"} />
                        <span>·</span>
                        <span>{meta.age || "—"}</span>
                        <span>·</span>
                        <span>
                            {t("pages:namespaceDetail.podsRunning", { running: runningPods, total: pods.length })}
                        </span>
                    </>
                }
                action={
                    <Button variant="outline" size="sm" onClick={() => navigate("/namespaces")} className="rounded-xl gap-1.5">
                        <ArrowLeft className="h-3.5 w-3.5" /> {t("pages:namespaceDetail.allNamespaces")}
                    </Button>
                }
            />

            <div className="stats c3">
                <FeatureTile icon={Box} label={t("pages:namespaces.specs.pods")} value={pods.length} />
                <FeatureTile icon={Layers} label={t("pages:namespaceDetail.workloads")} value={workloads.length} />
                <FeatureTile icon={Network} label={t("pages:namespaces.specs.services")} value={services.length} />
            </div>

            <Tabs defaultValue="pods">
                <TabsList className="tabs">
                    <TabsTrigger value="pods" className="gap-1.5">
                        <Box className="h-3.5 w-3.5" /> {t("pages:namespaceDetail.tabs.pods", { n: pods.length })}
                    </TabsTrigger>
                    <TabsTrigger value="workloads" className="gap-1.5">
                        <Layers className="h-3.5 w-3.5" /> {t("pages:namespaceDetail.tabs.workloads", { n: workloads.length })}
                    </TabsTrigger>
                    <TabsTrigger value="services" className="gap-1.5">
                        <Network className="h-3.5 w-3.5" /> {t("pages:namespaceDetail.tabs.services", { n: services.length })}
                    </TabsTrigger>
                </TabsList>

                <TabsContent value="pods" className="mt-4">
                    <GlassCard className="overflow-hidden">
                        {pods.length === 0 ? (
                            <Empty hint={t("pages:namespaceDetail.noPods")} />
                        ) : (
                            <Table>
                                <TableHeader>
                                    <TableRow>
                                        <TableHead>{t("common:fields.name")}</TableHead>
                                        <TableHead>{t("common:fields.status")}</TableHead>
                                        <TableHead>{t("common:fields.ready")}</TableHead>
                                        <TableHead>{t("common:fields.node")}</TableHead>
                                        <TableHead>{t("common:fields.age")}</TableHead>
                                    </TableRow>
                                </TableHeader>
                                <TableBody>
                                    {pods.map((p) => (
                                        <TableRow
                                            key={p.name}
                                            className="cursor-pointer hover:bg-muted"
                                            onClick={() => navigate(`/pods/${encodeURIComponent(ns)}/${encodeURIComponent(p.name)}`)}
                                        >
                                            <TableCell className="mono text-sm">{p.name}</TableCell>
                                            <TableCell>
                                                <StatusBadge status={p.status} />
                                            </TableCell>
                                            <TableCell className="mono text-xs text-muted-foreground">{p.ready || "—"}</TableCell>
                                            <TableCell className="mono text-xs text-muted-foreground">{p.node || "—"}</TableCell>
                                            <TableCell className="text-xs text-muted-foreground">{p.age || "—"}</TableCell>
                                        </TableRow>
                                    ))}
                                </TableBody>
                            </Table>
                        )}
                    </GlassCard>
                </TabsContent>

                <TabsContent value="workloads" className="mt-4">
                    <GlassCard className="overflow-hidden">
                        {workloads.length === 0 ? (
                            <Empty hint={t("pages:namespaceDetail.noWorkloads")} />
                        ) : (
                            <Table>
                                <TableHeader>
                                    <TableRow>
                                        <TableHead>{t("common:fields.name")}</TableHead>
                                        <TableHead>{t("common:fields.kind")}</TableHead>
                                        <TableHead>{t("common:fields.status")}</TableHead>
                                        <TableHead>{t("common:fields.ready")}</TableHead>
                                        <TableHead>{t("pages:namespaceDetail.columns.desired")}</TableHead>
                                        <TableHead>{t("common:fields.age")}</TableHead>
                                    </TableRow>
                                </TableHeader>
                                <TableBody>
                                    {workloads.map((w) => (
                                        <TableRow
                                            key={`${w.kind}/${w.name}`}
                                            className="cursor-pointer hover:bg-muted"
                                            onClick={() =>
                                                navigate(
                                                    `/workloads/${encodeURIComponent(ns)}/${encodeURIComponent(w.kind)}/${encodeURIComponent(w.name)}`
                                                )
                                            }
                                        >
                                            <TableCell className="mono text-sm">{w.name}</TableCell>
                                            <TableCell>
                                                <Badge variant="secondary" className="font-mono text-[11px]">
                                                    {w.kind}
                                                </Badge>
                                            </TableCell>
                                            <TableCell>
                                                <StatusBadge status={w.status} />
                                            </TableCell>
                                            <TableCell className="mono text-xs text-muted-foreground">{w.ready ?? "—"}</TableCell>
                                            <TableCell className="mono text-xs text-muted-foreground">{w.desired ?? "—"}</TableCell>
                                            <TableCell className="text-xs text-muted-foreground">{w.age || "—"}</TableCell>
                                        </TableRow>
                                    ))}
                                </TableBody>
                            </Table>
                        )}
                    </GlassCard>
                </TabsContent>

                <TabsContent value="services" className="mt-4">
                    <GlassCard className="overflow-hidden">
                        {services.length === 0 ? (
                            <Empty hint={t("pages:namespaceDetail.noServices")} />
                        ) : (
                            <Table>
                                <TableHeader>
                                    <TableRow>
                                        <TableHead>{t("common:fields.name")}</TableHead>
                                        <TableHead>{t("common:fields.type")}</TableHead>
                                        <TableHead>{t("pages:services.columns.clusterIp")}</TableHead>
                                        <TableHead>{t("pages:services.columns.ports")}</TableHead>
                                        <TableHead>{t("common:fields.age")}</TableHead>
                                    </TableRow>
                                </TableHeader>
                                <TableBody>
                                    {services.map((s) => (
                                        <TableRow
                                            key={s.name}
                                            className="cursor-pointer hover:bg-muted"
                                            onClick={() => navigate(`/services/${encodeURIComponent(ns)}/${encodeURIComponent(s.name)}`)}
                                        >
                                            <TableCell className="mono text-sm">
                                                <Link
                                                    to={`/services/${encodeURIComponent(ns)}/${encodeURIComponent(s.name)}`}
                                                    className="text-tone-link hover:underline"
                                                    onClick={(e) => e.stopPropagation()}
                                                >
                                                    {s.name}
                                                </Link>
                                            </TableCell>
                                            <TableCell>
                                                <Badge variant="secondary">{s.type || "—"}</Badge>
                                            </TableCell>
                                            <TableCell className="mono text-xs text-muted-foreground">{s.clusterIP || "—"}</TableCell>
                                            <TableCell>
                                                <div className="flex flex-wrap gap-1">
                                                    {(s.ports || []).map((p, i) => (
                                                        <Badge key={i} variant="outline" className="mono text-[11px] bg-muted/30">
                                                            {p.port}
                                                            {p.targetPort ? `→${p.targetPort}` : ""}/{p.protocol || "TCP"}
                                                        </Badge>
                                                    ))}
                                                    {(s.ports || []).length === 0 && <span className="text-xs text-muted-foreground">—</span>}
                                                </div>
                                            </TableCell>
                                            <TableCell className="text-xs text-muted-foreground">{s.age || "—"}</TableCell>
                                        </TableRow>
                                    ))}
                                </TableBody>
                            </Table>
                        )}
                    </GlassCard>
                    <p className="mt-3 text-xs text-muted-foreground">
                        {t("pages:namespaceDetail.preferClusterWide")}{" "}
                        <Link to="/services" className="text-tone-link underline-offset-2 hover:underline">
                            {t("pages:namespaceDetail.openServices")}
                        </Link>
                    </p>
                </TabsContent>
            </Tabs>
        </div>
    );
}
