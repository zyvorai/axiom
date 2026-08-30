import { useEffect, useMemo, useRef, useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { api, withAccessToken } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
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
    ShieldCheck,
    UserCog,
    Save,
    Activity,
    AlertTriangle,
    Plus,
    X,
} from "lucide-react";
import { toast } from "sonner";
import PodTerminal from "@/components/PodTerminal";
import Sparkline from "@/components/Sparkline";
import { useConfirm } from "@/components/ConfirmDialog";

function ExecPanel({ namespace, name, containers }) {
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
                        <SelectTrigger className="w-40 border-white/10 bg-white/[0.03]">
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
                    className="mono border-white/10 bg-white/[0.03]"
                />
                <Button variant="brand" onClick={run} disabled={busy} className="shrink-0 rounded-xl">
                    {busy ? "Running…" : "Run"}
                </Button>
            </div>
            <pre className="mono h-72 overflow-auto whitespace-pre-wrap rounded-lg border border-white/10 bg-black/40 p-3 text-[12px] text-foreground/90">
                {output || "Output will appear here."}
            </pre>
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
    const resources = c.resources || {};
    return (
        <GlassCard className="p-4">
            <div className="flex items-center justify-between">
                <div className="mono text-sm">{c.name}</div>
                <div className="mono truncate text-[11px] text-muted-foreground">{c.image}</div>
            </div>
            <div className="mt-3 grid grid-cols-2 gap-3 text-xs md:grid-cols-4">
                <div>
                    <div className="text-[10px] uppercase tracking-widest text-muted-foreground">CPU req/lim</div>
                    <div className="mono mt-0.5">
                        {resources.requests?.cpu || "—"} / {resources.limits?.cpu || "—"}
                    </div>
                </div>
                <div>
                    <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Mem req/lim</div>
                    <div className="mono mt-0.5">
                        {resources.requests?.memory || "—"} / {resources.limits?.memory || "—"}
                    </div>
                </div>
                <div>
                    <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Liveness</div>
                    <div className="mono mt-0.5 truncate">{probeSummary(c.livenessProbe) || "—"}</div>
                </div>
                <div>
                    <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Readiness</div>
                    <div className="mono mt-0.5 truncate">{probeSummary(c.readinessProbe) || "—"}</div>
                </div>
            </div>
            {(c.env || []).length > 0 && (
                <div className="mt-3 border-t border-white/5 pt-3">
                    <div className="mb-1.5 text-[10px] uppercase tracking-widest text-muted-foreground">Environment</div>
                    <div className="flex flex-wrap gap-1.5">
                        {c.env.map((e, i) => (
                            <Badge key={i} variant="outline" className="mono text-[10px] border-white/10 bg-white/[0.03]">
                                {e.name}={e.value ?? (e.valueFrom ? "<from ref>" : "")}
                            </Badge>
                        ))}
                    </div>
                </div>
            )}
            {(c.volumeMounts || []).length > 0 && (
                <div className="mt-3 border-t border-white/5 pt-3">
                    <div className="mb-1.5 text-[10px] uppercase tracking-widest text-muted-foreground">Mounts</div>
                    <div className="space-y-1">
                        {c.volumeMounts.map((m, i) => (
                            <div key={i} className="mono text-[11px] text-muted-foreground">
                                {m.name} → {m.mountPath}
                                {m.readOnly ? " (ro)" : ""}
                            </div>
                        ))}
                    </div>
                </div>
            )}
        </GlassCard>
    );
}

function MetricsPanel({ namespace, name }) {
    const [series, setSeries] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () =>
            api
                .get(`/pods/${namespace}/${encodeURIComponent(name)}/metrics`)
                .then((r) => alive && setSeries(r.data))
                .catch(() => alive && setSeries(false));
        load();
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, [namespace, name]);

    if (series === null) return <Skeleton className="h-40 rounded-2xl bg-white/[0.04]" />;
    if (series === false) {
        return <GlassCard className="p-6 text-center text-sm text-muted-foreground">Metrics unavailable — requires metrics-server on the cluster.</GlassCard>;
    }

    const cpuPoints = (series.points || []).map((p) => p.cpuMillicores);
    const memPoints = (series.points || []).map((p) => p.memoryBytes / (1024 * 1024));

    return (
        <div className="space-y-4">
            <div className="rounded-lg border border-white/10 bg-white/[0.02] px-3 py-2 text-xs text-muted-foreground">
                Live rolling window sampled from metrics.k8s.io — not historical Prometheus data.
            </div>
            <div className="grid gap-4 md:grid-cols-2">
                <GlassCard className="p-5">
                    <div className="mb-3 flex items-center justify-between">
                        <div className="flex items-center gap-1.5 text-sm font-medium">
                            <Cpu className="h-3.5 w-3.5 text-i17-cosmic" /> CPU
                        </div>
                        <span className="mono text-xs text-muted-foreground">{series.cpuMillicores}m</span>
                    </div>
                    <Sparkline data={cpuPoints} color="var(--i17-cosmic)" formatter={(v) => `${v}m`} domain={["auto", "auto"]} />
                </GlassCard>
                <GlassCard className="p-5">
                    <div className="mb-3 flex items-center justify-between">
                        <div className="flex items-center gap-1.5 text-sm font-medium">
                            <MemoryStick className="h-3.5 w-3.5 text-i17-sage" /> Memory
                        </div>
                        <span className="mono text-xs text-muted-foreground">{(series.memoryBytes / (1024 * 1024)).toFixed(0)} MiB</span>
                    </div>
                    <Sparkline data={memPoints} color="var(--i17-sage)" formatter={(v) => `${v.toFixed(0)} MiB`} domain={["auto", "auto"]} />
                </GlassCard>
            </div>
            {(series.containers || []).length > 0 && (
                <GlassCard className="p-0 overflow-hidden">
                    <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">Per-container</div>
                    {series.containers.map((c) => (
                        <div key={c.name} className="flex items-center justify-between border-b border-white/5 px-4 py-3 last:border-none text-xs">
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
                <TabsList className="h-auto flex-wrap gap-1 bg-white/[0.03] p-1">
                    {sessions.map((s) => (
                        <TabsTrigger key={s.id} value={String(s.id)} className="gap-1.5">
                            Shell {s.id}
                            {sessions.length > 1 && (
                                <span
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        closeSession(s.id);
                                    }}
                                    className="ml-1 rounded-full p-0.5 opacity-60 hover:opacity-100 hover:bg-white/10"
                                >
                                    <X className="h-3 w-3" />
                                </span>
                            )}
                        </TabsTrigger>
                    ))}
                </TabsList>
                <Button size="sm" variant="ghost" onClick={addSession} className="h-7 w-7 p-0 text-muted-foreground hover:text-foreground" title="New shell">
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
    const { namespace, name } = useParams();
    const navigate = useNavigate();
    const confirm = useConfirm();
    const [pods, setPods] = useState(null);
    const [events, setEvents] = useState(null);
    const [capabilities, setCapabilities] = useState(null);
    const [detail, setDetail] = useState(null);
    const [manifestText, setManifestText] = useState("");
    const [saving, setSaving] = useState(false);
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

    const saveManifest = async () => {
        setSaving(true);
        try {
            const { data } = await api.post("/apply", { manifest: manifestText });
            if (!data.valid) {
                toast.error("Apply failed", { description: (data.errors || [data.message]).filter(Boolean).join(", ") });
                return;
            }
            toast.success(`Applied ${data.kind || "Pod"}`);
        } catch (e) {
            toast.error("Apply failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setSaving(false);
        }
    };

    const pod = useMemo(() => (pods || []).find((p) => p.name === name), [pods, name]);
    const podEvents = useMemo(() => (events || []).filter((e) => e.object === `Pod/${name}`), [events, name]);
    const spec = detail && detail.manifest ? detail.manifest.spec : null;
    const status = detail && detail.manifest ? detail.manifest.status : null;
    const owner = (detail?.manifest?.metadata?.ownerReferences || [])[0];

    const evict = async () => {
        const ok = await confirm({
            title: `Evict pod ${name}?`,
            description: "This cannot be undone.",
            confirmLabel: "Evict",
        });
        if (!ok) return;
        try {
            await api.post(`/pods/${namespace}/${encodeURIComponent(name)}/evict`);
            toast.success(`Evicted ${name}`);
            navigate("/pods");
        } catch (e) {
            toast.error("Evict failed", { description: e?.response?.data?.error || e.message });
        }
    };

    if (pods === null) return <Skeleton className="h-96 rounded-2xl bg-white/[0.04]" />;

    if (!pod) {
        return (
            <div className="space-y-4">
                <button onClick={() => navigate(-1)} className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
                    <ArrowLeft className="h-4 w-4" /> Back
                </button>
                <GlassCard className="p-6 text-center text-sm text-muted-foreground">Pod {namespace}/{name} not found.</GlassCard>
            </div>
        );
    }

    const containers = pod.containers && pod.containers.length ? pod.containers : [""];

    return (
        <div className="space-y-4">
            <button onClick={() => navigate(-1)} className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
                <ArrowLeft className="h-4 w-4" /> Back
            </button>

            <div className="flex items-start justify-between">
                <div>
                    <div className="text-xs uppercase tracking-widest text-muted-foreground">Pod · {pod.namespace}</div>
                    <h1 className="mono text-3xl font-semibold tracking-tight">{pod.name}</h1>
                    <div className="mt-2 flex items-center gap-3 text-sm text-muted-foreground">
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
                                    owner: <span className="mono">{owner.kind}/{owner.name}</span>
                                </span>
                            </>
                        )}
                    </div>
                </div>
                <div className="flex gap-2">
                    <Link to={`/logs?pod=${pod.namespace}/${encodeURIComponent(pod.name)}`}>
                        <Button variant="outline" className="rounded-xl border-white/10 bg-white/[0.03]">
                            <ScrollText className="mr-1.5 h-3.5 w-3.5" /> Logs
                        </Button>
                    </Link>
                    <Button variant="outline" onClick={evict} className="rounded-xl border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20">
                        <Trash2 className="mr-1.5 h-3.5 w-3.5" /> Evict
                    </Button>
                </div>
            </div>

            {diagnosis && !diagnosis.scheduled && (
                <div className="flex items-start gap-3 rounded-2xl border border-amber-500/30 bg-amber-500/10 px-4 py-3">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-400" />
                    <div className="min-w-0">
                        <div className="text-sm font-medium text-amber-300">Not scheduled</div>
                        <div className="mt-0.5 text-sm text-amber-100/90">{diagnosis.diagnosis}</div>
                    </div>
                </div>
            )}
            {diagnosis && diagnosis.scheduled && diagnosis.phase !== "Running" && diagnosis.phase !== "Succeeded" && (
                <div className="flex items-start gap-3 rounded-2xl border border-white/10 bg-white/[0.03] px-4 py-3">
                    <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
                    <div className="min-w-0">
                        <div className="text-sm font-medium">{diagnosis.phase}</div>
                        <div className="mt-0.5 text-sm text-muted-foreground">{diagnosis.diagnosis}</div>
                    </div>
                </div>
            )}

            <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
                <GlassCard className="p-4">
                    <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Ready</div>
                    <div className="mono mt-1 text-xl">{pod.ready}</div>
                </GlassCard>
                <GlassCard className="p-4">
                    <div className="text-[10px] uppercase tracking-widest text-muted-foreground">Restarts</div>
                    <div className={`mono mt-1 text-xl ${pod.restarts > 0 ? "text-amber-400" : ""}`}>{pod.restarts}</div>
                </GlassCard>
                <GlassCard className="p-4">
                    <div className="flex items-center gap-1 text-[10px] uppercase tracking-widest text-muted-foreground">
                        <Cpu className="h-3 w-3" /> CPU limit
                    </div>
                    <div className="mono mt-1 text-xl">{pod.cpuLimit || "—"}</div>
                </GlassCard>
                <GlassCard className="p-4">
                    <div className="flex items-center gap-1 text-[10px] uppercase tracking-widest text-muted-foreground">
                        <MemoryStick className="h-3 w-3" /> Memory limit
                    </div>
                    <div className="mono mt-1 text-xl">{pod.memLimit || "—"}</div>
                </GlassCard>
            </div>

            <Tabs defaultValue="overview">
                <TabsList className="h-auto flex-wrap gap-1 bg-white/[0.03] p-1">
                    <TabsTrigger value="overview" className="gap-1.5">
                        <Box className="h-3.5 w-3.5" /> Details
                    </TabsTrigger>
                    <TabsTrigger value="terminal" className="gap-1.5">
                        <Terminal className="h-3.5 w-3.5" /> Terminal
                    </TabsTrigger>
                    <TabsTrigger value="yaml" className="gap-1.5">
                        <FileJson className="h-3.5 w-3.5" /> YAML
                    </TabsTrigger>
                    <TabsTrigger value="metrics" className="gap-1.5">
                        <Activity className="h-3.5 w-3.5" /> Metrics
                    </TabsTrigger>
                    <TabsTrigger value="events" className="gap-1.5">
                        <BellRing className="h-3.5 w-3.5" /> Events ({podEvents.length})
                    </TabsTrigger>
                    <TabsTrigger value="labels" className="gap-1.5">
                        <Tag className="h-3.5 w-3.5" /> Labels
                    </TabsTrigger>
                </TabsList>

                <TabsContent value="overview" className="mt-4 space-y-4">
                    {spec ? (
                        <>
                            <div className="grid gap-4 md:grid-cols-2">
                                <GlassCard className="p-4">
                                    <div className="flex items-center gap-1.5 text-[10px] uppercase tracking-widest text-muted-foreground">
                                        <UserCog className="h-3 w-3" /> Service account
                                    </div>
                                    <div className="mono mt-1 text-sm">{spec.serviceAccountName || "default"}</div>
                                </GlassCard>
                                <GlassCard className="p-4">
                                    <div className="flex items-center gap-1.5 text-[10px] uppercase tracking-widest text-muted-foreground">
                                        <ShieldCheck className="h-3 w-3" /> Restart policy
                                    </div>
                                    <div className="mono mt-1 text-sm">{spec.restartPolicy || "Always"}</div>
                                </GlassCard>
                            </div>

                            <div className="space-y-3">
                                <div className="text-sm font-medium">Containers</div>
                                {(spec.containers || []).map((c) => (
                                    <ContainerSpecCard key={c.name} c={c} />
                                ))}
                            </div>

                            {(spec.initContainers || []).length > 0 && (
                                <div className="space-y-3">
                                    <div className="text-sm font-medium">Init containers</div>
                                    {spec.initContainers.map((c) => (
                                        <ContainerSpecCard key={c.name} c={c} />
                                    ))}
                                </div>
                            )}

                            {(spec.volumes || []).length > 0 && (
                                <GlassCard className="p-0 overflow-hidden">
                                    <div className="flex items-center gap-2 border-b border-white/5 px-4 py-3 text-sm font-medium">
                                        <HardDrive className="h-4 w-4 text-i17-lavender" /> Volumes
                                    </div>
                                    {spec.volumes.map((v, i) => {
                                        const kind = Object.keys(v).find((k) => k !== "name") || "—";
                                        return (
                                            <div key={i} className="flex items-center justify-between border-b border-white/5 px-4 py-3 last:border-none text-xs">
                                                <span className="mono">{v.name}</span>
                                                <span className="text-muted-foreground">{kind}</span>
                                            </div>
                                        );
                                    })}
                                </GlassCard>
                            )}

                            {(status?.conditions || []).length > 0 && (
                                <GlassCard className="p-0 overflow-hidden">
                                    <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">Conditions</div>
                                    {status.conditions.map((c, i) => (
                                        <div key={i} className="flex items-center justify-between border-b border-white/5 px-4 py-3 last:border-none text-xs">
                                            <span>{c.type}</span>
                                            <StatusBadge status={c.status === "True" ? "Ready" : "Pending"} />
                                        </div>
                                    ))}
                                </GlassCard>
                            )}

                            {Object.keys(spec.nodeSelector || {}).length > 0 && (
                                <GlassCard className="p-4">
                                    <div className="mb-2 text-sm font-medium">Node selector</div>
                                    <div className="flex flex-wrap gap-1.5">
                                        {Object.entries(spec.nodeSelector).map(([k, v]) => (
                                            <Badge key={k} variant="outline" className="mono text-[11px] border-white/10 bg-white/[0.03]">
                                                {k}={v}
                                            </Badge>
                                        ))}
                                    </div>
                                </GlassCard>
                            )}

                            {(spec.tolerations || []).length > 0 && (
                                <GlassCard className="p-4">
                                    <div className="mb-2 text-sm font-medium">Tolerations</div>
                                    <div className="space-y-1 text-xs">
                                        {spec.tolerations.map((t, i) => (
                                            <div key={i} className="mono text-muted-foreground">
                                                {t.key || "*"}
                                                {t.operator ? ` ${t.operator}` : ""}
                                                {t.value ? ` ${t.value}` : ""} · {t.effect || "any"}
                                            </div>
                                        ))}
                                    </div>
                                </GlassCard>
                            )}
                        </>
                    ) : detail === null ? (
                        <Skeleton className="h-64 rounded-2xl bg-white/[0.04]" />
                    ) : (
                        <div className="space-y-4">
                            <GlassCard className="p-0 overflow-hidden">
                                <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">Containers</div>
                                {(pod.containerInfo || pod.containers || []).map((c, i) => {
                                    const info = typeof c === "string" ? { name: c } : c;
                                    return (
                                        <div key={i} className="flex items-center justify-between border-b border-white/5 px-4 py-3 last:border-none">
                                            <div className="mono text-sm">
                                                {info.name}
                                                {info.init && <Badge variant="outline" className="ml-2 border-white/10 bg-white/[0.03] text-[10px]">init</Badge>}
                                            </div>
                                            {info.state && <StatusBadge status={info.ready ? "Running" : info.state} />}
                                        </div>
                                    );
                                })}
                            </GlassCard>
                        </div>
                    )}

                    {(pod.ports || []).length > 0 && (
                        <GlassCard className="p-0 overflow-hidden">
                            <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">Ports</div>
                            {pod.ports.map((p, i) => (
                                <div key={i} className="flex items-center justify-between border-b border-white/5 px-4 py-3 last:border-none">
                                    <div className="mono text-sm text-muted-foreground">
                                        {p.container} · {p.containerPort}/{p.protocol || "TCP"}
                                    </div>
                                    <a
                                        href={withAccessToken(`/pods/${pod.namespace}/${encodeURIComponent(pod.name)}/window/${p.containerPort}/`)}
                                        target="_blank"
                                        rel="noreferrer"
                                        className="flex items-center gap-1 rounded-lg border border-white/10 bg-white/[0.03] px-2.5 py-1 text-xs hover:bg-white/[0.08]"
                                    >
                                        Open <ExternalLink className="h-3 w-3" />
                                    </a>
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
                            <div className="rounded-lg border border-white/10 bg-white/[0.02] px-3 py-2 text-xs text-muted-foreground">
                                Interactive terminal requires a live Kubernetes connection. Running a one-shot command instead.
                            </div>
                            <ExecPanel namespace={pod.namespace} name={pod.name} containers={containers} />
                        </div>
                    )}
                </TabsContent>

                <TabsContent value="yaml" className="mt-4 space-y-3">
                    <GlassCard className="p-0 overflow-hidden">
                        {detail && detail.manifest ? (
                            <Textarea
                                value={manifestText}
                                onChange={(e) => setManifestText(e.target.value)}
                                className="mono max-h-[65vh] min-h-[65vh] resize-none whitespace-pre rounded-none border-none bg-transparent p-4 text-[12px] text-foreground/90 focus-visible:ring-0"
                                spellCheck={false}
                            />
                        ) : (
                            <div className="px-4 py-8 text-center text-sm text-muted-foreground">
                                {detail === false ? "Manifest unavailable in this mode." : "Loading…"}
                            </div>
                        )}
                    </GlassCard>
                    {detail && detail.manifest && (
                        <div className="flex justify-end">
                            <Button variant="brand" onClick={saveManifest} disabled={saving} className="rounded-xl">
                                <Save className="mr-1.5 h-3.5 w-3.5" /> {saving ? "Saving…" : "Save"}
                            </Button>
                        </div>
                    )}
                </TabsContent>

                <TabsContent value="metrics" className="mt-4">
                    <MetricsPanel namespace={pod.namespace} name={pod.name} />
                </TabsContent>

                <TabsContent value="events" className="mt-4">
                    <GlassCard className="p-0 overflow-hidden">
                        {podEvents.length === 0 && <div className="px-4 py-8 text-center text-sm text-muted-foreground">No events for this pod.</div>}
                        {podEvents.map((e, i) => (
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

                <TabsContent value="labels" className="mt-4">
                    <GlassCard className="p-5">
                        <div className="flex flex-wrap gap-1.5">
                            {Object.entries(pod.labels || {}).map(([k, v]) => (
                                <Badge key={k} variant="outline" className="mono text-[11px] border-white/10 bg-white/[0.03]">
                                    {k}={v}
                                </Badge>
                            ))}
                            {Object.keys(pod.labels || {}).length === 0 && <span className="text-xs text-muted-foreground">No labels.</span>}
                        </div>
                    </GlassCard>
                </TabsContent>
            </Tabs>
        </div>
    );
}
