import { useEffect, useMemo, useRef, useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { api, withAccessToken } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
    ArrowLeft,
    Cpu,
    MemoryStick,
    ExternalLink,
    Trash2,
    Terminal,
    ScrollText,
    BellRing,
    Tag,
    Box,
    FileJson,
    HardDrive,
    Save,
    Activity,
    AlertTriangle,
    Plus,
    X,
    ArrowRight,
    Copy,
} from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import PodTerminal from "@/components/PodTerminal";
import DebugContainerPanel from "@/components/DebugContainerPanel";
import { copyText, portForwardCommand } from "@/lib/troubleshoot";
import { PORTFWD } from "@/constants/testIds";
import Sparkline from "@/components/Sparkline";
import FeatureTile from "@/components/FeatureTile";
import SpecTable from "@/components/SpecTable";
import CodeEditor from "@/components/CodeEditor";
import MacTerm from "@/components/MacTerm";
import DiffReviewDialog from "@/components/DiffReviewDialog";
import KvChip, { toneForKey } from "@/components/KvChip";
import { maskEnvDisplay } from "@/lib/sensitive";
import { useConfirm } from "@/components/ConfirmDialog";
import { ExplainWithAiButton } from "@/components/AiAssistantDrawer";
import { AI } from "@/constants/testIds";

function ExecPanel({ namespace, name, containers }) {
    const { t } = useTranslation();
    const [container, setContainer] = useState(containers[0] || "");
    const [command, setCommand] = useState("ls -la");
    const [output, setOutput] = useState("");
    const [busy, setBusy] = useState(false);

    const run = async () => {
        setBusy(true);
        setOutput("");
        try {
            const { data } = await api.post(`/pods/${namespace}/${encodeURIComponent(name)}/exec`, { container, command });
            setOutput(data.output || "");
        } catch (e) {
            setOutput(e?.response?.data?.error || e.message);
        } finally {
            setBusy(false);
        }
    };

    return (
        <GlassCard className="p-5 space-y-3">
            <div className="flex gap-2">
                {containers.length > 1 && (
                    <Select value={container} onValueChange={setContainer}>
                        <SelectTrigger className="w-40 bg-muted/30">
                            <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                            {containers.map((c) => (
                                <SelectItem key={c} value={c}>
                                    {c}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                )}
                <Input
                    value={command}
                    onChange={(e) => setCommand(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && run()}
                    className="mono bg-muted/30"
                />
                <Button variant="brand" onClick={run} disabled={busy} className="shrink-0 rounded-xl">
                    {busy ? t("pages:podDetail.exec.running") : t("pages:podDetail.exec.run")}
                </Button>
            </div>
            <MacTerm
                title={`exec — ${command || "command"}`}
                bodyClassName="h-72 overflow-auto whitespace-pre-wrap px-4 py-3 text-[12px] leading-[1.55] text-term-plain"
            >
                {output || <span className="text-term-dim">{t("pages:podDetail.exec.outputPlaceholder")}</span>}
            </MacTerm>
        </GlassCard>
    );
}

function probeSummary(p) {
    if (!p) return null;
    if (p.httpGet) return `HTTP GET ${p.httpGet.path || "/"}:${p.httpGet.port}`;
    if (p.tcpSocket) return `TCP :${p.tcpSocket.port}`;
    if (p.exec) return `exec ${(p.exec.command || []).join(" ")}`;
    return "configured";
}

function ContainerSpecCard({ c }) {
    const { t } = useTranslation();
    const resources = c.resources || {};
    return (
        <div className="border-t border-border py-5 first:border-t-0 first:pt-0">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
                <div className="mono text-sm font-medium tracking-[-0.01em]">{c.name}</div>
                <div className="mono truncate text-[12px] text-muted-foreground">{c.image}</div>
            </div>
            <div className="mt-4 grid grid-cols-2 gap-x-8 gap-y-3 text-xs md:grid-cols-4">
                <div>
                    <div className="text-[11px] tracking-[-0.01em] text-muted-foreground">{t("pages:podDetail.container.cpu")}</div>
                    <div className="mono mt-0.5 text-sm">
                        {resources.requests?.cpu || "—"} / {resources.limits?.cpu || "—"}
                    </div>
                </div>
                <div>
                    <div className="text-[11px] tracking-[-0.01em] text-muted-foreground">{t("pages:podDetail.container.memory")}</div>
                    <div className="mono mt-0.5 text-sm">
                        {resources.requests?.memory || "—"} / {resources.limits?.memory || "—"}
                    </div>
                </div>
                <div>
                    <div className="text-[11px] tracking-[-0.01em] text-muted-foreground">{t("pages:podDetail.container.liveness")}</div>
                    <div className="mono mt-0.5 truncate text-sm">{probeSummary(c.livenessProbe) || "—"}</div>
                </div>
                <div>
                    <div className="text-[11px] tracking-[-0.01em] text-muted-foreground">{t("pages:podDetail.container.readiness")}</div>
                    <div className="mono mt-0.5 truncate text-sm">{probeSummary(c.readinessProbe) || "—"}</div>
                </div>
            </div>
            {(c.env || []).length > 0 && (
                <div className="mt-4">
                    <div className="mb-1.5 text-[11px] tracking-[-0.01em] text-muted-foreground">{t("pages:podDetail.container.environment")}</div>
                    <div className="flex flex-wrap gap-x-3 gap-y-1">
                        {c.env.map((e, i) => (
                            <KvChip
                                key={i}
                                name={e.name}
                                value={maskEnvDisplay(e.name, e.value, e.valueFrom)}
                                tone={toneForKey(e.name)}
                            />
                        ))}
                    </div>
                </div>
            )}
            {(c.volumeMounts || []).length > 0 && (
                <div className="mt-4">
                    <div className="mb-1.5 text-[11px] tracking-[-0.01em] text-muted-foreground">{t("pages:podDetail.container.mounts")}</div>
                    <div className="space-y-1">
                        {(c.volumeMounts || []).map((m, i) => (
                            <div key={i} className="mono text-[12px] text-muted-foreground">
                                {m.name} → {m.mountPath}
                                {m.readOnly ? " (ro)" : ""}
                            </div>
                        ))}
                    </div>
                </div>
            )}
        </div>
    );
}

function MetricsPanel({ namespace, name }) {
    const { t } = useTranslation();
    const [series, setSeries] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () =>
            api
                .get(`/pods/${namespace}/${encodeURIComponent(name)}/metrics`)
                .then((r) => alive && setSeries(r.data))
                .catch(() => alive && setSeries(false));
        load();
        const timer = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, [namespace, name]);

    if (series === null) return <Skeleton className="h-40 rounded-2xl bg-muted" />;
    if (series === false) {
        return <GlassCard className="p-6 text-center text-sm text-muted-foreground">{t("pages:podDetail.metrics.unavailable")}</GlassCard>;
    }

    const cpuPoints = (series.points || []).map((p) => p.cpuMillicores);
    const memPoints = (series.points || []).map((p) => p.memoryBytes / (1024 * 1024));

    return (
        <div className="space-y-4">
            <div className="border-y border-border px-3 py-2 text-xs text-muted-foreground">
                {t("pages:podDetail.metrics.window")}
            </div>
            <div className="grid gap-4 md:grid-cols-2">
                <GlassCard className="p-5">
                    <div className="mb-3 flex items-center justify-between">
                        <div className="flex items-center gap-1.5 text-sm font-medium">
                            <Cpu className="h-3.5 w-3.5 text-tone-link" /> {t("common:fields.cpu")}
                        </div>
                        <span className="mono text-xs text-muted-foreground">{series.cpuMillicores}m</span>
                    </div>
                    <Sparkline data={cpuPoints} color="var(--apple-blue)" formatter={(v) => `${v}m`} domain={["auto", "auto"]} />
                </GlassCard>
                <GlassCard className="p-5">
                    <div className="mb-3 flex items-center justify-between">
                        <div className="flex items-center gap-1.5 text-sm font-medium">
                            <MemoryStick className="h-3.5 w-3.5 text-tone-green" /> {t("common:fields.memory")}
                        </div>
                        <span className="mono text-xs text-muted-foreground">{(series.memoryBytes / (1024 * 1024)).toFixed(0)} MiB</span>
                    </div>
                    <Sparkline data={memPoints} color="var(--green)" formatter={(v) => `${v.toFixed(0)} MiB`} domain={["auto", "auto"]} />
                </GlassCard>
            </div>
            {(series.containers || []).length > 0 && (
                <GlassCard className="overflow-hidden">
                    <div className="border-b px-4 py-3 text-sm font-medium">{t("pages:podDetail.metrics.perContainer")}</div>
                    {series.containers.map((c) => (
                        <div key={c.name} className="flex items-center justify-between border-b px-4 py-3 last:border-none text-xs">
                            <span className="mono">{c.name}</span>
                            <span className="mono text-muted-foreground">
                                {c.cpuMillicores}m · {(c.memoryBytes / (1024 * 1024)).toFixed(0)} MiB
                            </span>
                        </div>
                    ))}
                </GlassCard>
            )}
        </div>
    );
}

function TerminalSessions({ namespace, name, containers }) {
    const { t } = useTranslation();
    const [sessions, setSessions] = useState([{ id: 1 }]);
    const [active, setActive] = useState("1");
    const nextId = useRef(2);

    const addSession = () => {
        const id = nextId.current++;
        setSessions((s) => [...s, { id }]);
        setActive(String(id));
    };

    const closeSession = (id) => {
        setSessions((s) => {
            const next = s.filter((sess) => sess.id !== id);
            if (String(active) === String(id) && next.length > 0) {
                setActive(String(next[next.length - 1].id));
            }
            return next;
        });
    };

    return (
        <Tabs value={active} onValueChange={setActive}>
            <div className="flex items-center gap-1">
                <TabsList>
                    {sessions.map((s) => (
                        <TabsTrigger key={s.id} value={String(s.id)} className="gap-1.5">
                            {t("pages:podDetail.shell", { id: s.id })}
                            {sessions.length > 1 && (
                                <span
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        closeSession(s.id);
                                    }}
                                    className="ml-1 rounded-full p-0.5 opacity-60 hover:opacity-100 hover:bg-muted"
                                >
                                    <X className="h-3 w-3" />
                                </span>
                            )}
                        </TabsTrigger>
                    ))}
                </TabsList>
                <Button size="sm" variant="ghost" onClick={addSession} className="h-8 w-8 p-0 text-muted-foreground hover:text-foreground" title={t("pages:podDetail.newShell")}>
                    <Plus className="h-3.5 w-3.5" />
                </Button>
            </div>
            {sessions.map((s) => (
                <TabsContent key={s.id} value={String(s.id)} className="mt-3">
                    <PodTerminal namespace={namespace} name={name} containers={containers} />
                </TabsContent>
            ))}
        </Tabs>
    );
}

export default function PodDetail() {
    const { t } = useTranslation();
    const { namespace, name } = useParams();
    const navigate = useNavigate();
    const confirm = useConfirm();
    const [pods, setPods] = useState(null);
    const [events, setEvents] = useState(null);
    const [capabilities, setCapabilities] = useState(null);
    const [detail, setDetail] = useState(null);
    const [manifestText, setManifestText] = useState("");
    const [reviewOpen, setReviewOpen] = useState(false);
    const [diagnosis, setDiagnosis] = useState(null);

    useEffect(() => {
        let alive = true;
        api
            .get(`/pods/${namespace}/${encodeURIComponent(name)}/diagnosis`)
            .then((r) => alive && setDiagnosis(r.data))
            .catch(() => alive && setDiagnosis(null));
        return () => {
            alive = false;
        };
    }, [namespace, name]);

    useEffect(() => {
        let alive = true;
        api.get("/pods", { params: { namespace } }).then((r) => alive && setPods(r.data));
        api.get("/events", { params: { namespace } }).then((r) => alive && setEvents(r.data));
        api
            .get("/realtime/capabilities")
            .then((r) => alive && setCapabilities(r.data))
            .catch(() => alive && setCapabilities({ interactiveExec: false }));
        api
            .get(`/resources/${namespace}/pod/${encodeURIComponent(name)}`)
            .then((r) => {
                if (!alive) return;
                setDetail(r.data);
                setManifestText(JSON.stringify(r.data.manifest, null, 2));
            })
            .catch(() => alive && setDetail(false));
        return () => {
            alive = false;
        };
    }, [namespace, name]);

    // A browser cannot open a local listener, so TCP port-forwarding stays a
    // kubectl job; hand the user the exact command instead.
    const copyPortForward = async (port) => {
        const cmd = portForwardCommand(namespace, name, port);
        if (await copyText(cmd)) toast.success(t("common:actions.copied"), { description: cmd });
        else toast.error(t("toasts:podDetail.copyFailed"), { description: cmd });
    };

    const pod = useMemo(() => (pods || []).find((p) => p.name === name), [pods, name]);
    const podEvents = useMemo(() => (events || []).filter((e) => e.object === `Pod/${name}`), [events, name]);
    const spec = detail && detail.manifest ? detail.manifest.spec : null;
    const status = detail && detail.manifest ? detail.manifest.status : null;
    const owner = (detail?.manifest?.metadata?.ownerReferences || [])[0];

    const evict = async () => {
        const ok = await confirm({
            title: t("dialogs:podDetail.evict.title", { name }),
            description: t("dialogs:podDetail.evict.description"),
            confirmLabel: t("dialogs:podDetail.evict.confirm"),
        });
        if (!ok) return;
        try {
            await api.post(`/pods/${namespace}/${encodeURIComponent(name)}/evict`);
            toast.success(t("toasts:podDetail.evicted", { name }));
            navigate("/pods");
        } catch (e) {
            toast.error(t("toasts:podDetail.evictFailed"), { description: e?.response?.data?.error || e.message });
        }
    };

    if (pods === null) return (
        <div className="page">
            <Skeleton className="h-96 rounded-2xl bg-muted" />
        </div>
    );

    if (!pod) {
        return (
            <div className="page space-y-6">
                <button onClick={() => navigate(-1)} className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
                    <ArrowLeft className="h-4 w-4" /> {t("common:actions.back")}
                </button>
                <GlassCard className="p-6 text-center text-sm text-muted-foreground">{t("pages:podDetail.notFound", { namespace, name })}</GlassCard>
            </div>
        );
    }

    const containers = pod.containers && pod.containers.length ? pod.containers : [""];

    return (
        <div className="page space-y-6">
            <button onClick={() => navigate(-1)} className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
                <ArrowLeft className="h-4 w-4" /> {t("common:actions.back")}
            </button>

            <DetailHeader
                eyebrow={`Pod · ${pod.namespace}`}
                name={pod.name}
                meta={
                    <>
                        <StatusBadge status={pod.status} />
                        <span>·</span>
                        <span>{pod.node}</span>
                        <span>·</span>
                        <span className="mono">{pod.ip}</span>
                        <span>·</span>
                        <span>{pod.age}</span>
                        {owner && (
                            <>
                                <span>·</span>
                                <span>
                                    {t("pages:podDetail.owner")} <span className="mono">{owner.kind}/{owner.name}</span>
                                </span>
                            </>
                        )}
                    </>
                }
                action={
                    <>
                        <Link
                            to={`/logs?pod=${pod.namespace}/${encodeURIComponent(pod.name)}`}
                            className="flex items-center gap-1 text-sm text-tone-link hover:underline"
                        >
                            <ScrollText className="h-3.5 w-3.5" /> {t("pages:podDetail.logs")} <ArrowRight className="h-3 w-3" />
                        </Link>
                        <Button variant="outline" onClick={evict} className="rounded-xl border-soft-danger-border bg-soft-danger text-tone-red hover:bg-soft-danger">
                            <Trash2 className="mr-1.5 h-3.5 w-3.5" /> {t("pages:podDetail.evict")}
                        </Button>
                    </>
                }
            />

            {diagnosis && !diagnosis.scheduled && (
                <GlassCard className="flex items-start gap-3 p-4">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-tone-amber" />
                    <div className="min-w-0">
                        <div className="text-sm font-medium">{t("pages:podDetail.notScheduled")}</div>
                        <div className="mt-0.5 text-sm text-muted-foreground">{diagnosis.diagnosis}</div>
                    </div>
                    <ExplainWithAiButton testId={AI.explainPod} className="ml-auto" target={{ namespace, kind: "pod", name, label: `Pod ${namespace}/${name}` }} />
                </GlassCard>
            )}
            {diagnosis && diagnosis.scheduled && diagnosis.phase !== "Running" && diagnosis.phase !== "Succeeded" && (
                <div className="flex items-start gap-3 border-y border-border px-4 py-3">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                    <div className="min-w-0">
                        <div className="text-sm font-medium">{diagnosis.phase}</div>
                        <div className="mt-0.5 text-sm text-muted-foreground">{diagnosis.diagnosis}</div>
                    </div>
                    <ExplainWithAiButton testId={AI.explainPod} className="ml-auto" target={{ namespace, kind: "pod", name, label: `Pod ${namespace}/${name}` }} />
                </div>
            )}

            <div className="stats">
                <FeatureTile size="compact" mono label={t("common:fields.ready")} value={pod.ready} />
                <FeatureTile
                    size="compact"
                    mono
                    label={t("common:fields.restarts")}
                    value={<span className={pod.restarts > 0 ? "text-tone-amber" : ""}>{pod.restarts}</span>}
                />
                <FeatureTile size="compact" mono icon={Cpu} label={t("pages:podDetail.stats.cpuLimit")} value={pod.cpuLimit || "—"} />
                <FeatureTile size="compact" mono icon={MemoryStick} label={t("pages:podDetail.stats.memoryLimit")} value={pod.memLimit || "—"} />
            </div>

            <Tabs defaultValue="overview">
                <TabsList className="tabs">
                    <TabsTrigger value="overview" className="gap-1.5">
                        <Box className="h-3.5 w-3.5" /> {t("pages:podDetail.tabs.details")}
                    </TabsTrigger>
                    <TabsTrigger value="terminal" className="gap-1.5">
                        <Terminal className="h-3.5 w-3.5" /> {t("pages:podDetail.tabs.terminal")}
                    </TabsTrigger>
                    <TabsTrigger value="yaml" className="gap-1.5">
                        <FileJson className="h-3.5 w-3.5" /> YAML
                    </TabsTrigger>
                    <TabsTrigger value="metrics" className="gap-1.5">
                        <Activity className="h-3.5 w-3.5" /> {t("pages:podDetail.tabs.metrics")}
                    </TabsTrigger>
                    <TabsTrigger value="events" className="gap-1.5">
                        <BellRing className="h-3.5 w-3.5" /> {t("pages:podDetail.tabs.events", { n: podEvents.length })}
                    </TabsTrigger>
                    <TabsTrigger value="labels" className="gap-1.5">
                        <Tag className="h-3.5 w-3.5" /> {t("common:fields.labels")}
                    </TabsTrigger>
                </TabsList>

                <TabsContent value="overview" className="mt-4 space-y-4">
                    {spec ? (
                        <>
                            <GlassCard>
                                <SpecTable
                                    title={t("pages:podDetail.spec.title")}
                                    columns={2}
                                    rows={[
                                        { label: t("pages:podDetail.spec.serviceAccount"), value: spec.serviceAccountName || "default", mono: true },
                                        { label: t("pages:podDetail.spec.restartPolicy"), value: spec.restartPolicy || "Always", mono: true },
                                    ]}
                                />
                            </GlassCard>

                            <div className="space-y-3">
                                <div className="text-sm font-medium">{t("pages:podDetail.containers")}</div>
                                {(spec.containers || []).map((c) => (
                                    <ContainerSpecCard key={c.name} c={c} />
                                ))}
                            </div>

                            {(spec.initContainers || []).length > 0 && (
                                <div className="space-y-3">
                                    <div className="text-sm font-medium">{t("pages:podDetail.initContainers")}</div>
                                    {spec.initContainers.map((c) => (
                                        <ContainerSpecCard key={c.name} c={c} />
                                    ))}
                                </div>
                            )}

                            {(spec.volumes || []).length > 0 && (
                                <GlassCard className="overflow-hidden p-0">
                                    <div className="flex items-center gap-2 border-b border-border px-4 py-3 text-sm font-medium">
                                        <HardDrive className="h-4 w-4 text-tone-link" /> {t("pages:podDetail.volumes")}
                                    </div>
                                    {spec.volumes.map((v, i) => {
                                        const kind = Object.keys(v).find((k) => k !== "name") || "—";
                                        return (
                                            <div key={i} className="flex items-center justify-between border-b border-border px-4 py-3 text-xs last:border-none">
                                                <span className="mono text-tone-purple">{v.name}</span>
                                                <Badge variant="outline" className="border-border bg-muted text-[11px] text-tone-purple">
                                                    {kind}
                                                </Badge>
                                            </div>
                                        );
                                    })}
                                </GlassCard>
                            )}

                            {(status?.conditions || []).length > 0 && (
                                <GlassCard className="overflow-hidden p-0">
                                    <div className="border-b border-border px-4 py-3 text-sm font-medium">{t("pages:podDetail.conditions")}</div>
                                    {status.conditions.map((c, i) => (
                                        <div key={i} className="flex items-center justify-between border-b border-border px-4 py-3 text-xs last:border-none">
                                            <span className="text-foreground">{c.type}</span>
                                            <StatusBadge status={c.status === "True" ? "Ready" : "Pending"} />
                                        </div>
                                    ))}
                                </GlassCard>
                            )}

                            {Object.keys(spec.nodeSelector || {}).length > 0 && (
                                <GlassCard className="p-4">
                                    <div className="mb-2 text-sm font-medium">{t("pages:podDetail.nodeSelector")}</div>
                                    <div className="flex flex-wrap gap-1.5">
                                        {Object.entries(spec.nodeSelector).map(([k, v]) => (
                                            <KvChip key={k} name={k} value={v} tone="gold" />
                                        ))}
                                    </div>
                                </GlassCard>
                            )}

                            {(spec.tolerations || []).length > 0 && (
                                <GlassCard className="p-4">
                                    <div className="mb-2 text-sm font-medium">{t("pages:podDetail.tolerations")}</div>
                                    <div className="space-y-1 text-xs">
                                        {spec.tolerations.map((tol, i) => (
                                            <div key={i} className="mono text-tone-amber">
                                                {tol.key || "*"}
                                                {tol.operator ? ` ${tol.operator}` : ""}
                                                {tol.value ? ` ${tol.value}` : ""} · {tol.effect || "any"}
                                            </div>
                                        ))}
                                    </div>
                                </GlassCard>
                            )}
                        </>
                    ) : detail === null ? (
                        <Skeleton className="h-64 rounded-2xl bg-muted" />
                    ) : (
                        <div className="space-y-4">
                            <GlassCard className="overflow-hidden">
                                <div className="border-b px-4 py-3 text-sm font-medium">{t("pages:podDetail.containers")}</div>
                                {(pod.containerInfo || pod.containers || []).map((c, i) => {
                                    const info = typeof c === "string" ? { name: c } : c;
                                    return (
                                        <div key={i} className="flex items-center justify-between border-b px-4 py-3 last:border-none">
                                            <div className="mono text-sm">
                                                {info.name}
                                                {info.init && <Badge variant="outline" className="ml-2 bg-muted/30 text-[11px]">init</Badge>}
                                            </div>
                                            {info.state && <StatusBadge status={info.ready ? "Running" : info.state} />}
                                        </div>
                                    );
                                })}
                            </GlassCard>
                        </div>
                    )}

                    {(pod.ports || []).length > 0 && (
                        <GlassCard className="overflow-hidden p-0">
                            <div className="border-b border-border px-4 py-3 text-sm font-medium">{t("pages:podDetail.ports")}</div>
                            {pod.ports.map((p, i) => (
                                <div key={i} className="flex items-center justify-between border-b border-border px-4 py-3 last:border-none">
                                    <div className="mono text-sm text-tone-link">
                                        {p.container} · {p.containerPort}/{p.protocol || "TCP"}
                                    </div>
                                    <div className="flex items-center gap-2">
                                        <button
                                            type="button"
                                            data-testid={PORTFWD.copy(p.containerPort)}
                                            onClick={() => copyPortForward(p.containerPort)}
                                            title={portForwardCommand(pod.namespace, pod.name, p.containerPort)}
                                            className="flex items-center gap-1 rounded-lg border border-border px-2.5 py-1 text-xs text-muted-foreground hover:text-foreground"
                                        >
                                            <Copy className="h-3 w-3" aria-hidden="true" /> {t("pages:podDetail.copyPortForward")}
                                        </button>
                                        <a
                                            data-testid={PORTFWD.open(p.containerPort)}
                                            href={withAccessToken(`/pods/${pod.namespace}/${encodeURIComponent(pod.name)}/window/${p.containerPort}/`)}
                                            target="_blank"
                                            rel="noreferrer"
                                            className="flex items-center gap-1 rounded-lg border border-soft-info-border bg-soft-info px-2.5 py-1 text-xs text-tone-link hover:bg-soft-info"
                                        >
                                            {t("common:actions.open")} <ExternalLink className="h-3 w-3" />
                                        </a>
                                    </div>
                                </div>
                            ))}
                        </GlassCard>
                    )}
                </TabsContent>

                <TabsContent value="terminal" className="mt-4">
                    {capabilities?.interactiveExec ? (
                        <TerminalSessions namespace={pod.namespace} name={pod.name} containers={containers} />
                    ) : (
                        <div className="space-y-3">
                            <div className="border-y border-border px-3 py-2 text-xs text-muted-foreground">
                                {t("pages:podDetail.interactiveUnavailable")}
                            </div>
                            <ExecPanel namespace={pod.namespace} name={pod.name} containers={containers} />
                        </div>
                    )}
                    <div className="mt-4">
                        <DebugContainerPanel
                            namespace={pod.namespace}
                            name={pod.name}
                            containers={containers}
                            interactive={!!capabilities?.interactiveExec}
                        />
                    </div>
                </TabsContent>

                <TabsContent value="yaml" className="mt-4 space-y-3">
                    {detail && detail.manifest ? (
                        <CodeEditor value={manifestText} onChange={setManifestText} />
                    ) : (
                        <GlassCard className="overflow-hidden">
                            <div className="px-4 py-8 text-center text-sm text-muted-foreground">
                                {detail === false ? t("pages:podDetail.manifestUnavailable") : t("common:states.loading")}
                            </div>
                        </GlassCard>
                    )}
                    {detail && detail.manifest && (
                        <DiffReviewDialog open={reviewOpen} onOpenChange={setReviewOpen} manifest={manifestText} fallbackKind="Pod" />
                    )}
                    {detail && detail.manifest && (
                        <div className="flex justify-end">
                            <Button variant="brand" onClick={() => setReviewOpen(true)} disabled={!manifestText.trim()} className="rounded-xl">
                                <Save className="mr-1.5 h-3.5 w-3.5" /> {t("common:actions.save")}
                            </Button>
                        </div>
                    )}
                </TabsContent>

                <TabsContent value="metrics" className="mt-4">
                    <MetricsPanel namespace={pod.namespace} name={pod.name} />
                </TabsContent>

                <TabsContent value="events" className="mt-4">
                    <GlassCard className="overflow-hidden">
                        {podEvents.length === 0 && <div className="px-4 py-8 text-center text-sm text-muted-foreground">{t("pages:podDetail.noEvents")}</div>}
                        {podEvents.map((e, i) => (
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

                <TabsContent value="labels" className="mt-4">
                    <GlassCard className="p-5">
                        <div className="flex flex-wrap gap-1.5">
                            {Object.entries(pod.labels || {}).map(([k, v]) => (
                                <KvChip key={k} name={k} value={v} tone={toneForKey(k)} />
                            ))}
                            {Object.keys(pod.labels || {}).length === 0 && <span className="text-xs text-muted-foreground">{t("pages:podDetail.noLabels")}</span>}
                        </div>
                    </GlassCard>
                </TabsContent>
            </Tabs>
        </div>
    );
}
