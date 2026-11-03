import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
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
    const { t } = useTranslation();
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
            toast.success(t("toasts:autoscaling.created", { name: form.targetName }));
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error(t("toasts:autoscaling.createFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>{t("dialogs:autoscaling.create.title")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label htmlFor="hpa-target-workload" className="text-xs text-muted-foreground">{t("dialogs:autoscaling.create.targetWorkload")}</Label>
                        <Select onValueChange={pickTarget}>
                            <SelectTrigger id="hpa-target-workload" className="bg-muted/30">
                                <SelectValue placeholder={t("dialogs:autoscaling.create.selectWorkload")} />
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
                            <Label htmlFor="hpa-min-replicas" className="text-xs text-muted-foreground">{t("dialogs:autoscaling.create.minReplicas")}</Label>
                            <Input id="hpa-min-replicas" type="number" min={1} value={form.minReplicas} onChange={(e) => set({ minReplicas: Number(e.target.value) })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="hpa-max-replicas" className="text-xs text-muted-foreground">{t("dialogs:autoscaling.create.maxReplicas")}</Label>
                            <Input id="hpa-max-replicas" type="number" min={1} value={form.maxReplicas} onChange={(e) => set({ maxReplicas: Number(e.target.value) })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="hpa-target-cpu" className="text-xs text-muted-foreground">{t("dialogs:autoscaling.create.targetCpu")}</Label>
                            <Input id="hpa-target-cpu" type="number" min={1} max={100} value={form.targetCpuPercent} onChange={(e) => set({ targetCpuPercent: e.target.value })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="hpa-target-memory" className="text-xs text-muted-foreground">{t("dialogs:autoscaling.create.targetMemory")}</Label>
                            <Input id="hpa-target-memory" data-testid={AUTOSCALE.createMemory} type="number" min={1} max={100} value={form.targetMemoryPercent} onChange={(e) => set({ targetMemoryPercent: e.target.value })} placeholder={t("dialogs:autoscaling.create.targetMemoryPlaceholder")} className="bg-muted/30" />
                        </div>
                        <div className="col-span-2 space-y-1.5">
                            <Label htmlFor="hpa-stabilization" className="text-xs text-muted-foreground">{t("dialogs:autoscaling.create.stabilization")}</Label>
                            <Input id="hpa-stabilization" data-testid={AUTOSCALE.createStabilization} type="number" min={0} max={3600} value={form.scaleDownStabilizationSeconds} onChange={(e) => set({ scaleDownStabilizationSeconds: e.target.value })} placeholder={t("dialogs:autoscaling.create.stabilizationPlaceholder")} className="bg-muted/30" />
                        </div>
                    </div>
                    <p className="text-[11px] text-muted-foreground">
                        {t("dialogs:autoscaling.create.bothNote")}
                    </p>
                </div>
                <DialogFooter>
                    <Button variant="brand" onClick={submit} disabled={busy || !form.targetName || !(Number(form.targetCpuPercent) || Number(form.targetMemoryPercent))} className="rounded-xl">
                        {busy ? t("dialogs:autoscaling.create.creating") : t("common:actions.create")}
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
    if (active && active.status === "False") return { tone: "var(--red)", labelKey: "pages:autoscaling.state.notScaling", detail: active.message || active.reason };
    if (limited && limited.status === "True") return { tone: "var(--orange)", labelKey: "pages:autoscaling.state.atLimit", detail: limited.message || limited.reason };
    if (h.desiredReplicas && h.desiredReplicas !== h.currentReplicas) return { tone: "var(--blue)", labelKey: "pages:autoscaling.state.scalingTo", labelArgs: { replicas: h.desiredReplicas }, detail: "" };
    return { tone: "var(--green)", labelKey: "pages:autoscaling.state.stable", detail: "" };
}

function HpaTab() {
    const { t } = useTranslation();
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
        const timer = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    const remove = async (h) => {
        const ok = await confirm({
            title: t("dialogs:autoscaling.delete.title", { name: h.name }),
            description: t("dialogs:autoscaling.delete.description"),
            confirmLabel: t("common:actions.delete"),
        });
        if (!ok) return;
        try {
            await api.delete(`/hpas/${h.namespace}/${encodeURIComponent(h.name)}`);
            toast.success(t("toasts:autoscaling.deleted", { name: h.name }));
            load();
        } catch (e) {
            toast.error(t("toasts:autoscaling.deleteFailed"), { description: e?.response?.data?.error || e.message });
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
                    <Plus size={15} /> {t("pages:autoscaling.createAutoscaler")}
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
                    <div className="empty">{t("pages:autoscaling.empty")}</div>
                ) : (
                    <div className="max-h-[65vh] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:autoscaling.region")}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("common:fields.name")}</th>
                                    <th>{t("common:fields.target")}</th>
                                    <th>{t("pages:autoscaling.columns.minMax")}</th>
                                    <th>{t("pages:autoscaling.columns.current")}</th>
                                    <th>{t("pages:autoscaling.columns.metrics")}</th>
                                    <th>{t("pages:autoscaling.columns.state")}</th>
                                    <th>{t("common:fields.age")}</th>
                                    <th><span className="sr-only">{t("common:fields.actions")}</span></th>
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
                                                            {t(st.labelKey, st.labelArgs)}
                                                        </span>
                                                    </span>
                                                );
                                            })()}
                                        </td>
                                        <td className="mono">{h.age}</td>
                                        <td>
                                            <button type="button" className="btn btn-line btn-sm" aria-label={t("pages:autoscaling.deleteAria", { name: h.name })} title={t("pages:autoscaling.deleteAria", { name: h.name })} onClick={() => remove(h)}>
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
    const { t } = useTranslation();
    const [tab, setTab] = useState("hpa");
    return (
        <div className="page">
            <PageHeader
                title={t("pages:autoscaling.title")}
                description={t("pages:autoscaling.description")}
            />
            <Tabs value={tab} onValueChange={setTab}>
                <TabsList className="tabs">
                    <TabsTrigger value="hpa" data-testid={AUTOSCALE.tab("hpa")} className="gap-1.5">
                        <Gauge className="h-3.5 w-3.5" /> {t("pages:autoscaling.tabs.hpa")}
                    </TabsTrigger>
                    <TabsTrigger value="vpa" data-testid={AUTOSCALE.tab("vpa")} className="gap-1.5">
                        {t("pages:autoscaling.tabs.vpa")}
                    </TabsTrigger>
                    <TabsTrigger value="keda" data-testid={AUTOSCALE.tab("keda")} className="gap-1.5">
                        {t("pages:autoscaling.tabs.keda")}
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
