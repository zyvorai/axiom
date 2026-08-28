import { useEffect, useMemo, useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { api, withAccessToken } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
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
import { ArrowLeft, Cpu, MemoryStick, ExternalLink, Trash2, Terminal, ScrollText, BellRing, Tag, Box } from "lucide-react";
import { toast } from "sonner";

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
                <Button onClick={run} disabled={busy} className="shrink-0 rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white">
                    {busy ? "Running…" : "Run"}
                </Button>
            </div>
            <pre className="mono h-72 overflow-auto whitespace-pre-wrap rounded-lg border border-white/10 bg-black/40 p-3 text-[12px] text-foreground/90">
                {output || "Output will appear here."}
            </pre>
        </GlassCard>
    );
}

export default function PodDetail() {
    const { namespace, name } = useParams();
    const navigate = useNavigate();
    const [pods, setPods] = useState(null);
    const [events, setEvents] = useState(null);

    useEffect(() => {
        let alive = true;
        api.get("/pods", { params: { namespace } }).then((r) => alive && setPods(r.data));
        api.get("/events", { params: { namespace } }).then((r) => alive && setEvents(r.data));
        return () => {
            alive = false;
        };
    }, [namespace]);

    const pod = useMemo(() => (pods || []).find((p) => p.name === name), [pods, name]);
    const podEvents = useMemo(() => (events || []).filter((e) => e.object === `Pod/${name}`), [events, name]);

    const evict = async () => {
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
                <TabsList className="bg-white/[0.03] p-1">
                    <TabsTrigger value="overview" className="gap-1.5">
                        <Box className="h-3.5 w-3.5" /> Containers
                    </TabsTrigger>
                    <TabsTrigger value="terminal" className="gap-1.5">
                        <Terminal className="h-3.5 w-3.5" /> Terminal
                    </TabsTrigger>
                    <TabsTrigger value="events" className="gap-1.5">
                        <BellRing className="h-3.5 w-3.5" /> Events ({podEvents.length})
                    </TabsTrigger>
                    <TabsTrigger value="labels" className="gap-1.5">
                        <Tag className="h-3.5 w-3.5" /> Labels
                    </TabsTrigger>
                </TabsList>

                <TabsContent value="overview" className="mt-4 space-y-4">
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
                    <ExecPanel namespace={pod.namespace} name={pod.name} containers={containers} />
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
