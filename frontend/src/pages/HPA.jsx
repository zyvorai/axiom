import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
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
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { Gauge, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useConfirm } from "@/components/ConfirmDialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { VpaPanel, KedaPanel } from "@/components/AutoscalingPanels";
import { AUTOSCALE } from "@/constants/testIds";

function CreateHPADialog({ open, onOpenChange, workloads, onCreated }) {
    const [form, setForm] = useState({ namespace: "", targetKind: "", targetName: "", minReplicas: 1, maxReplicas: 5, targetCpuPercent: 80, targetMemoryPercent: "", scaleDownStabilizationSeconds: "" });
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const pickTarget = (value) => {
        const [namespace, kind, name] = value.split("|");
        set({ namespace, targetKind: kind, targetName: name });
    };

    const submit = async () => {
        if (!form.namespace || !form.targetName) return;
        setBusy(true);
        try {
            const body = { ...form, name: `${form.targetName}-hpa` };
            body.targetCpuPercent = Number(form.targetCpuPercent) || 0;
            body.targetMemoryPercent = Number(form.targetMemoryPercent) || 0;
            body.scaleDownStabilizationSeconds = Number(form.scaleDownStabilizationSeconds) || 0;
            await api.post("/hpas", body);
            toast.success(`HPA created for ${form.targetName}`);
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error("Create HPA failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>Create autoscaler</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label htmlFor="hpa-target-workload" className="text-xs text-muted-foreground">Target workload</Label>
                        <Select onValueChange={pickTarget}>
                            <SelectTrigger id="hpa-target-workload" className="bg-muted/30">
                                <SelectValue placeholder="Select a workload" />
                            </SelectTrigger>
                            <SelectContent>
                                {workloads.map((w) => (
                                    <SelectItem key={`${w.namespace}/${w.kind}/${w.name}`} value={`${w.namespace}|${w.kind}|${w.name}`}>
                                        {w.namespace}/{w.kind}/{w.name}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label htmlFor="hpa-min-replicas" className="text-xs text-muted-foreground">Min replicas</Label>
                            <Input id="hpa-min-replicas" type="number" min={1} value={form.minReplicas} onChange={(e) => set({ minReplicas: Number(e.target.value) })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="hpa-max-replicas" className="text-xs text-muted-foreground">Max replicas</Label>
                            <Input id="hpa-max-replicas" type="number" min={1} value={form.maxReplicas} onChange={(e) => set({ maxReplicas: Number(e.target.value) })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="hpa-target-cpu" className="text-xs text-muted-foreground">Target CPU % (blank = ignore)</Label>
                            <Input id="hpa-target-cpu" type="number" min={1} max={100} value={form.targetCpuPercent} onChange={(e) => set({ targetCpuPercent: e.target.value })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="hpa-target-memory" className="text-xs text-muted-foreground">Target memory % (optional)</Label>
                            <Input id="hpa-target-memory" data-testid={AUTOSCALE.createMemory} type="number" min={1} max={100} value={form.targetMemoryPercent} onChange={(e) => set({ targetMemoryPercent: e.target.value })} placeholder="e.g. 75" className="bg-muted/30" />
                        </div>
                        <div className="col-span-2 space-y-1.5">
                            <Label htmlFor="hpa-stabilization" className="text-xs text-muted-foreground">Scale-down stabilization, seconds (optional)</Label>
                            <Input id="hpa-stabilization" data-testid={AUTOSCALE.createStabilization} type="number" min={0} max={3600} value={form.scaleDownStabilizationSeconds} onChange={(e) => set({ scaleDownStabilizationSeconds: e.target.value })} placeholder="Kubernetes default is 300" className="bg-muted/30" />
                        </div>
                    </div>
                    <p className="text-[11px] text-muted-foreground">
                        With both CPU and memory set, the autoscaler scales to whichever needs more replicas.
                    </p>
                </div>
                <DialogFooter>
                    <Button variant="brand" onClick={submit} disabled={busy || !form.targetName || !(Number(form.targetCpuPercent) || Number(form.targetMemoryPercent))} className="rounded-xl">
                        {busy ? "Creating…" : "Create"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function metricLabel(m) {
    const base = m.type === "Resource" || m.type === "ContainerResource" ? m.name : `${m.type.toLowerCase()}: ${m.name}`;
    return `${base} ${m.current || "—"} / ${m.target}`;
}

// A short, honest state for the row: what the autoscaler itself reports.
function hpaState(h) {
    const cond = (t) => (h.conditions || []).find((c) => c.type === t);
    const active = cond("ScalingActive");
    const limited = cond("ScalingLimited");
    if (active && active.status === "False") return { tone: "var(--red)", label: "Not scaling", detail: active.message || active.reason };
    if (limited && limited.status === "True") return { tone: "var(--orange)", label: "At limit", detail: limited.message || limited.reason };
    if (h.desiredReplicas && h.desiredReplicas !== h.currentReplicas) return { tone: "var(--blue)", label: `Scaling to ${h.desiredReplicas}`, detail: "" };
    return { tone: "var(--green)", label: "Stable", detail: "" };
}

function HpaTab() {
    const confirm = useConfirm();
    const [hpas, setHpas] = useState(null);
    const [workloads, setWorkloads] = useState([]);
    const [dialogOpen, setDialogOpen] = useState(false);

    const load = () => api.get("/hpas").then((r) => setHpas(r.data));

    useEffect(() => {
        let alive = true;
        const tick = () => api.get("/hpas").then((r) => alive && setHpas(r.data));
        tick();
        api.get("/workloads").then((r) => alive && setWorkloads(r.data || []));
        const t = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    const remove = async (h) => {
        const ok = await confirm({
            title: `Delete autoscaler ${h.name}?`,
            description: "This cannot be undone.",
            confirmLabel: "Delete",
        });
        if (!ok) return;
        try {
            await api.delete(`/hpas/${h.namespace}/${encodeURIComponent(h.name)}`);
            toast.success(`Deleted ${h.name}`);
            load();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };

    if (!hpas) {
        return (
            <div className="stack">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="stack">
            <div className="flex justify-end">
                <button type="button" className="btn btn-fill" onClick={() => setDialogOpen(true)}>
                    <Plus size={15} /> Create autoscaler
                </button>
            </div>

            <GlassCard className="overflow-hidden">
                <div className="card-h">
                    <h2 className="flex items-center gap-2 text-[15px]">
                        <Gauge size={16} className="text-tone-link" /> HorizontalPodAutoscalers
                    </h2>
                    <span className="r text-[12.5px] text-ink-3">{hpas.length}</span>
                </div>
                {hpas.length === 0 ? (
                    <div className="empty">No HorizontalPodAutoscalers configured.</div>
                ) : (
                    <div className="max-h-[65vh] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label="Autoscalers">
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>Name</th>
                                    <th>Target</th>
                                    <th>Min – Max</th>
                                    <th>Current</th>
                                    <th>Metrics (current / target)</th>
                                    <th>State</th>
                                    <th>Age</th>
                                    <th><span className="sr-only">Actions</span></th>
                                </tr>
                            </thead>
                            <tbody>
                                {hpas.map((h, i) => (
                                    <tr key={i} data-testid={AUTOSCALE.hpaRow(h.namespace, h.name)}>
                                        <td className="name">
                                            {h.name}
                                            <small>{h.namespace}</small>
                                        </td>
                                        <td>{h.targetKind}/{h.targetName}</td>
                                        <td className="mono">{h.minReplicas} – {h.maxReplicas}</td>
                                        <td className="mono num">{h.currentReplicas}</td>
                                        <td data-testid={AUTOSCALE.hpaMetrics(h.namespace, h.name)}>
                                            {(h.metrics || []).length > 0 ? (
                                                <div className="flex flex-wrap gap-1">
                                                    {h.metrics.map((m, k) => (
                                                        <span key={k} className="pill soft mono text-[11px]">{metricLabel(m)}</span>
                                                    ))}
                                                </div>
                                            ) : (
                                                <span className="mono">{h.currentCpuPercent ?? "—"}% / {h.targetCpuPercent ?? "—"}%</span>
                                            )}
                                        </td>
                                        <td data-testid={AUTOSCALE.hpaState(h.namespace, h.name)}>
                                            {(() => {
                                                const st = hpaState(h);
                                                return (
                                                    <span title={st.detail || undefined}>
                                                        <span className="pill" style={{ "--c": st.tone }}>
                                                            <i aria-hidden="true" />
                                                            {st.label}
                                                        </span>
                                                    </span>
                                                );
                                            })()}
                                        </td>
                                        <td className="mono">{h.age}</td>
                                        <td>
                                            <button type="button" className="btn btn-line btn-sm" aria-label={`Delete autoscaler ${h.name}`} title={`Delete autoscaler ${h.name}`} onClick={() => remove(h)}>
                                                <Trash2 size={13} />
                                            </button>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </GlassCard>

            <CreateHPADialog open={dialogOpen} onOpenChange={setDialogOpen} workloads={workloads} onCreated={load} />
        </div>
    );
}

export default function HPA() {
    const [tab, setTab] = useState("hpa");
    return (
        <div className="page">
            <PageHeader
                title="Autoscaling"
                description="Horizontal, vertical and event-driven autoscalers across all namespaces."
            />
            <Tabs value={tab} onValueChange={setTab}>
                <TabsList className="tabs">
                    <TabsTrigger value="hpa" data-testid={AUTOSCALE.tab("hpa")} className="gap-1.5">
                        <Gauge className="h-3.5 w-3.5" /> Horizontal
                    </TabsTrigger>
                    <TabsTrigger value="vpa" data-testid={AUTOSCALE.tab("vpa")} className="gap-1.5">
                        Vertical (VPA)
                    </TabsTrigger>
                    <TabsTrigger value="keda" data-testid={AUTOSCALE.tab("keda")} className="gap-1.5">
                        Event-driven (KEDA)
                    </TabsTrigger>
                </TabsList>
                <TabsContent value="hpa" className="mt-4">
                    <HpaTab />
                </TabsContent>
                <TabsContent value="vpa" className="mt-4">
                    <VpaPanel />
                </TabsContent>
                <TabsContent value="keda" className="mt-4">
                    <KedaPanel />
                </TabsContent>
            </Tabs>
        </div>
    );
}
