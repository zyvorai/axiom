import { useEffect, useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import StatusBadge from "@/components/StatusBadge";
import Sparkline from "@/components/Sparkline";
import FeatureTile from "@/components/FeatureTile";
import SpecTable from "@/components/SpecTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
    Ban,
    CheckCircle2,
    LogOut,
    Box,
    Activity,
    Tag,
    Plus,
    Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { useConfirm } from "@/components/ConfirmDialog";

function MetricsPanel({ name }) {
    const [series, setSeries] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () =>
            api
                .get(`/nodes/${encodeURIComponent(name)}/metrics`)
                .then((r) => alive && setSeries(r.data))
                .catch(() => alive && setSeries(false));
        load();
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, [name]);

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
        </div>
    );
}

function TaintsPanel({ name, taints, onChanged }) {
    const confirm = useConfirm();
    const [key, setKey] = useState("");
    const [value, setValue] = useState("");
    const [effect, setEffect] = useState("NoSchedule");
    const [busy, setBusy] = useState(false);

    const add = async () => {
        if (!key) return;
        setBusy(true);
        try {
            await api.post(`/nodes/${encodeURIComponent(name)}/taints`, { key, value, effect });
            toast.success(`Taint added to ${name}`);
            setKey("");
            setValue("");
            onChanged();
        } catch (e) {
            toast.error("Add taint failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    const remove = async (t) => {
        const ok = await confirm({
            title: `Remove taint ${t.key}?`,
            description: "This cannot be undone.",
            confirmLabel: "Remove",
        });
        if (!ok) return;
        try {
            await api.delete(`/nodes/${encodeURIComponent(name)}/taints/${encodeURIComponent(t.key)}`, {
                params: { effect: t.effect },
            });
            toast.success(`Taint removed from ${name}`);
            onChanged();
        } catch (e) {
            toast.error("Remove taint failed", { description: e?.response?.data?.error || e.message });
        }
    };

    return (
        <div className="space-y-4">
            <GlassCard className="p-0 overflow-hidden">
                <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">Current taints</div>
                {(taints || []).length === 0 && <div className="px-4 py-8 text-center text-sm text-muted-foreground">No taints on this node.</div>}
                {(taints || []).map((t, i) => (
                    <div key={i} className="flex items-center justify-between border-b border-white/5 px-4 py-3 last:border-none">
                        <Badge variant="outline" className="mono border-white/10 bg-white/[0.03] text-xs">
                            {t.key}
                            {t.value ? `=${t.value}` : ""}:{t.effect}
                        </Badge>
                        <Button size="sm" variant="ghost" onClick={() => remove(t)} className="h-7 px-2 text-muted-foreground hover:text-rose-400">
                            <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                    </div>
                ))}
            </GlassCard>
            <GlassCard className="p-4">
                <div className="mb-3 text-sm font-medium">Add taint</div>
                <div className="grid grid-cols-4 gap-3">
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Key</Label>
                        <Input value={key} onChange={(e) => setKey(e.target.value)} placeholder="dedicated" className="border-white/10 bg-white/[0.03]" />
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Value</Label>
                        <Input value={value} onChange={(e) => setValue(e.target.value)} placeholder="gpu" className="border-white/10 bg-white/[0.03]" />
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Effect</Label>
                        <Select value={effect} onValueChange={setEffect}>
                            <SelectTrigger className="border-white/10 bg-white/[0.03]">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="NoSchedule">NoSchedule</SelectItem>
                                <SelectItem value="PreferNoSchedule">PreferNoSchedule</SelectItem>
                                <SelectItem value="NoExecute">NoExecute</SelectItem>
                            </SelectContent>
                        </Select>
                    </div>
                    <div className="flex items-end">
                        <Button variant="brand" onClick={add} disabled={busy || !key} className="w-full rounded-xl">
                            <Plus className="mr-1.5 h-3.5 w-3.5" /> Add
                        </Button>
                    </div>
                </div>
            </GlassCard>
        </div>
    );
}

export default function NodeDetail() {
    const { name } = useParams();
    const navigate = useNavigate();
    const [node, setNode] = useState(null);
    const [busy, setBusy] = useState("");

    const load = () => api.get(`/nodes/${encodeURIComponent(name)}`).then((r) => setNode(r.data));

    useEffect(() => {
        let alive = true;
        api.get(`/nodes/${encodeURIComponent(name)}`).then((r) => alive && setNode(r.data));
        return () => {
            alive = false;
        };
    }, [name]);

    const act = async (action, path) => {
        setBusy(action);
        try {
            await api.post(`/nodes/${encodeURIComponent(name)}/${path}`);
            toast.success(`${name}: ${action} requested`);
            await load();
        } catch (e) {
            toast.error(`${action} failed`, { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    if (!node) return <Skeleton className="h-96 rounded-2xl bg-white/[0.04]" />;

    return (
        <div className="space-y-6">
            <button onClick={() => navigate(-1)} className="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground">
                <ArrowLeft className="h-4 w-4" /> Back
            </button>

            <DetailHeader
                eyebrow={`Node · ${node.role}`}
                name={node.name}
                meta={
                    <>
                        <StatusBadge status={node.status} />
                        <span>·</span>
                        <span className="mono">{node.internalIp}</span>
                        <span>·</span>
                        <span>{node.age}</span>
                    </>
                }
                action={
                    <>
                        {node.status === "SchedulingDisabled" ? (
                            <Button variant="outline" disabled={busy === "uncordon"} onClick={() => act("uncordon", "uncordon")} className="rounded-xl border-white/10 bg-white/[0.03]">
                                <CheckCircle2 className="mr-1.5 h-3.5 w-3.5" /> Uncordon
                            </Button>
                        ) : (
                            <Button variant="outline" disabled={busy === "cordon"} onClick={() => act("cordon", "cordon")} className="rounded-xl border-white/10 bg-white/[0.03]">
                                <Ban className="mr-1.5 h-3.5 w-3.5" /> Cordon
                            </Button>
                        )}
                        <Button variant="outline" disabled={busy === "drain"} onClick={() => act("drain", "drain")} className="rounded-xl border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20">
                            <LogOut className="mr-1.5 h-3.5 w-3.5" /> Drain
                        </Button>
                    </>
                }
            />

            <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
                <FeatureTile size="compact" mono label="Pods" value={`${node.pods}/${node.allocatablePods}`} />
                <FeatureTile size="compact" mono icon={Cpu} label="Allocatable CPU" value={node.allocatableCpu} />
                <FeatureTile size="compact" mono icon={MemoryStick} label="Allocatable Memory" value={node.allocatableMemory} />
                <FeatureTile size="compact" mono label="Version" value={<span className="truncate">{node.version}</span>} />
            </div>

            <Tabs defaultValue="overview">
                <TabsList className="h-auto flex-wrap gap-1 bg-white/[0.03] p-1">
                    <TabsTrigger value="overview" className="gap-1.5">
                        <Box className="h-3.5 w-3.5" /> Details
                    </TabsTrigger>
                    <TabsTrigger value="metrics" className="gap-1.5">
                        <Activity className="h-3.5 w-3.5" /> Metrics
                    </TabsTrigger>
                    <TabsTrigger value="taints" className="gap-1.5">
                        <Tag className="h-3.5 w-3.5" /> Taints ({(node.taints || []).length})
                    </TabsTrigger>
                </TabsList>

                <TabsContent value="overview" className="mt-4 space-y-4">
                    <SpecTable
                        title="System"
                        columns={2}
                        rows={[
                            { label: "OS image", value: node.osImage || "—", mono: true },
                            { label: "Kernel", value: node.kernelVersion || "—", mono: true },
                            { label: "Container runtime", value: node.containerRuntime || "—", mono: true },
                        ]}
                    />
                    {(node.conditions || []).length > 0 && (
                        <GlassCard className="p-0 overflow-hidden">
                            <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">Conditions</div>
                            {node.conditions.map((c, i) => (
                                <div key={i} className="flex items-center justify-between border-b border-white/5 px-4 py-3 last:border-none text-xs">
                                    <span>{c.type}</span>
                                    <StatusBadge status={c.status === "True" ? "Ready" : "Pending"} />
                                </div>
                            ))}
                        </GlassCard>
                    )}
                </TabsContent>

                <TabsContent value="metrics" className="mt-4">
                    <MetricsPanel name={node.name} />
                </TabsContent>

                <TabsContent value="taints" className="mt-4">
                    <TaintsPanel name={node.name} taints={node.taints} onChanged={load} />
                </TabsContent>
            </Tabs>
        </div>
    );
}
