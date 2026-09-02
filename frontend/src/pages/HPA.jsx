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

function CreateHPADialog({ open, onOpenChange, workloads, onCreated }) {
    const [form, setForm] = useState({ namespace: "", targetKind: "", targetName: "", minReplicas: 1, maxReplicas: 5, targetCpuPercent: 80 });
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
            await api.post("/hpas", { ...form, name: `${form.targetName}-hpa` });
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
            <DialogContent className="glass max-w-md">
                <DialogHeader>
                    <DialogTitle>Create autoscaler</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Target workload</Label>
                        <Select onValueChange={pickTarget}>
                            <SelectTrigger className="bg-muted/30">
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
                    <div className="grid grid-cols-3 gap-3">
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Min replicas</Label>
                            <Input type="number" min={1} value={form.minReplicas} onChange={(e) => set({ minReplicas: Number(e.target.value) })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Max replicas</Label>
                            <Input type="number" min={1} value={form.maxReplicas} onChange={(e) => set({ maxReplicas: Number(e.target.value) })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Target CPU %</Label>
                            <Input type="number" min={1} max={100} value={form.targetCpuPercent} onChange={(e) => set({ targetCpuPercent: Number(e.target.value) })} className="bg-muted/30" />
                        </div>
                    </div>
                </div>
                <DialogFooter>
                    <Button variant="brand" onClick={submit} disabled={busy || !form.targetName} className="rounded-xl">
                        {busy ? "Creating…" : "Create"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

export default function HPA() {
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
            <div className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="space-y-6">
            <PageHeader
                eyebrow="AUTOSCALING"
                title="Autoscaling"
                description="Horizontal Pod Autoscalers across all namespaces."
                action={
                    <Button size="sm" variant="brand" onClick={() => setDialogOpen(true)} className="rounded-xl">
                        <Plus className="mr-1.5 h-3.5 w-3.5" /> Create autoscaler
                    </Button>
                }
            />

            <GlassCard className="p-0 overflow-hidden">
                <div className="flex items-center justify-between border-b px-4 py-3">
                    <div className="flex items-center gap-2 text-sm font-medium">
                        <Gauge className="h-4 w-4 text-i17-cosmic" /> HorizontalPodAutoscalers
                    </div>
                    <span className="text-xs text-muted-foreground">{hpas.length}</span>
                </div>
                <div className="max-h-[65vh] overflow-auto scrollbar-thin">
                    {hpas.length === 0 && <div className="px-4 py-8 text-center text-sm text-muted-foreground">No HorizontalPodAutoscalers configured.</div>}
                    {hpas.map((h, i) => (
                        <div key={i} className="grid grid-cols-12 items-center gap-3 border-b px-4 py-3 last:border-none hover:bg-muted">
                            <div className="col-span-3 min-w-0">
                                <div className="mono truncate text-sm">{h.name}</div>
                                <div className="text-[11px] text-muted-foreground">{h.namespace}</div>
                            </div>
                            <div className="col-span-2 text-xs text-muted-foreground">
                                {h.targetKind}/{h.targetName}
                            </div>
                            <div className="col-span-2 mono text-xs">
                                {h.minReplicas} – {h.maxReplicas}
                            </div>
                            <div className="col-span-1 mono text-xs tabular-nums">{h.currentReplicas}</div>
                            <div className="col-span-2 mono text-xs">
                                {h.currentCpuPercent ?? "—"}% / {h.targetCpuPercent ?? "—"}%
                            </div>
                            <div className="col-span-1 mono text-xs text-muted-foreground">{h.age}</div>
                            <div className="col-span-1 flex justify-end">
                                <Button size="sm" variant="ghost" onClick={() => remove(h)} className="h-7 px-2 text-muted-foreground hover:text-rose-400">
                                    <Trash2 className="h-3.5 w-3.5" />
                                </Button>
                            </div>
                        </div>
                    ))}
                </div>
            </GlassCard>

            <CreateHPADialog open={dialogOpen} onOpenChange={setDialogOpen} workloads={workloads} onCreated={load} />
        </div>
    );
}
