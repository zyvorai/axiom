import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import ApplyManifestDialog from "@/components/ApplyManifestDialog";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Switch } from "@/components/ui/switch";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import {
    DropdownMenu,
    DropdownMenuTrigger,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import {
    Search,
    MoreHorizontal,
    Play,
    Square,
    RotateCw,
    Lock,
    Unlock,
    Trash2,
    Rocket,
    GitCommitHorizontal,
    Plus,
} from "lucide-react";
import { toast } from "sonner";

function RolloutDialog({ workload, onOpenChange }) {
    const [status, setStatus] = useState(null);
    const [busy, setBusy] = useState("");

    const load = () =>
        api
            .get(`/workloads/${workload.namespace}/${workload.kind}/${encodeURIComponent(workload.name)}/rollout`)
            .then((r) => setStatus(r.data))
            .catch(() => setStatus(false));

    useEffect(() => {
        load();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [workload]);

    const act = async (action) => {
        setBusy(action);
        try {
            await api.post(`/workloads/${workload.namespace}/${workload.kind}/${encodeURIComponent(workload.name)}/rollout/${action}`);
            toast.success(`${action} requested`);
            load();
        } catch (e) {
            toast.error(`${action} failed`, { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    return (
        <Dialog open onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-lg border-white/10">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">Rollout · {workload.name}</DialogTitle>
                </DialogHeader>
                {status === null && <Skeleton className="h-40 rounded-xl bg-white/[0.04]" />}
                {status === false && <div className="py-8 text-center text-sm text-muted-foreground">Rollout not supported for {workload.kind}.</div>}
                {status && (
                    <div className="space-y-4">
                        <div className="grid grid-cols-4 gap-3 text-center text-xs">
                            {[
                                ["Status", status.status],
                                ["Replicas", status.replicas],
                                ["Ready", status.ready],
                                ["Available", status.available],
                            ].map(([label, value]) => (
                                <div key={label} className="rounded-lg border border-white/5 bg-white/[0.02] p-2.5">
                                    <div className="text-[10px] uppercase tracking-widest text-muted-foreground">{label}</div>
                                    <div className="mono mt-1 text-sm">{value}</div>
                                </div>
                            ))}
                        </div>
                        {(status.history || []).length > 0 && (
                            <div className="rounded-xl border border-white/5">
                                {status.history.map((h) => (
                                    <div key={h.revision} className="flex items-center justify-between border-b border-white/5 px-3 py-2 text-xs last:border-none">
                                        <span className="mono">rev {h.revision}</span>
                                        <span className="text-muted-foreground">{h.status}</span>
                                        <span className="mono text-muted-foreground">{h.age}</span>
                                    </div>
                                ))}
                            </div>
                        )}
                        <div className="flex flex-wrap gap-2">
                            {["restart", "pause", "resume", "undo"].map((a) => (
                                <Button
                                    key={a}
                                    size="sm"
                                    variant="outline"
                                    disabled={!!busy}
                                    onClick={() => act(a)}
                                    className="rounded-xl border-white/10 bg-white/[0.03] capitalize"
                                >
                                    {a}
                                </Button>
                            ))}
                        </div>
                    </div>
                )}
            </DialogContent>
        </Dialog>
    );
}

function ScaleDialog({ workload, onOpenChange, onDone }) {
    const [replicas, setReplicas] = useState(String(workload.desired ?? workload.ready ?? 1));
    const [busy, setBusy] = useState(false);

    const submit = async () => {
        setBusy(true);
        try {
            await api.post(`/workloads/${workload.namespace}/${workload.kind}/${encodeURIComponent(workload.name)}/scale`, {
                replicas: Number(replicas),
            });
            toast.success(`${workload.name} scaled to ${replicas}`);
            onOpenChange(false);
            onDone();
        } catch (e) {
            toast.error("Scale failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-xs border-white/10">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">Scale · {workload.name}</DialogTitle>
                </DialogHeader>
                <Input type="number" min={0} value={replicas} onChange={(e) => setReplicas(e.target.value)} className="border-white/10 bg-white/[0.03]" />
                <DialogFooter>
                    <Button onClick={submit} disabled={busy} className="rounded-xl">
                        {busy ? "Scaling…" : "Scale"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function WorkloadActions({ w, onChanged, onScale, onRollout }) {
    const [busy, setBusy] = useState("");
    // A DaemonSet has no replica count in Kubernetes' own model — one Pod
    // per matching node, not a scalable target — so Start/Stop/Scale/
    // Rollout/Seal/Unseal all correctly 400 server-side. Restart and
    // Delete do apply and stay available.
    const isDaemonSet = w.kind === "DaemonSet";

    const act = async (action) => {
        setBusy(action);
        try {
            await api.post(`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}/${action}`);
            toast.success(`${w.name}: ${action} requested`);
            onChanged();
        } catch (e) {
            toast.error(`${action} failed`, { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const remove = async () => {
        setBusy("delete");
        try {
            await api.delete(`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}`);
            toast.success(`${w.name} deleted`);
            onChanged();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    return (
        <DropdownMenu>
            <DropdownMenuTrigger asChild>
                <Button size="sm" variant="ghost" disabled={!!busy} className="h-7 w-7 p-0 text-muted-foreground">
                    <MoreHorizontal className="h-4 w-4" />
                </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-44">
                {!isDaemonSet && (
                    <>
                        <DropdownMenuItem onClick={() => act("start")}>
                            <Play className="mr-2 h-3.5 w-3.5" /> Start
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => act("stop")}>
                            <Square className="mr-2 h-3.5 w-3.5" /> Stop
                        </DropdownMenuItem>
                    </>
                )}
                <DropdownMenuItem onClick={() => act("restart")}>
                    <RotateCw className="mr-2 h-3.5 w-3.5" /> Restart
                </DropdownMenuItem>
                {!isDaemonSet && (
                    <>
                        <DropdownMenuItem onClick={onScale}>
                            <GitCommitHorizontal className="mr-2 h-3.5 w-3.5" /> Scale…
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={onRollout}>
                            <Rocket className="mr-2 h-3.5 w-3.5" /> Rollout…
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem onClick={() => act("seal")}>
                            <Lock className="mr-2 h-3.5 w-3.5" /> Seal
                        </DropdownMenuItem>
                        <DropdownMenuItem onClick={() => act("unseal")}>
                            <Unlock className="mr-2 h-3.5 w-3.5" /> Unseal
                        </DropdownMenuItem>
                    </>
                )}
                <DropdownMenuSeparator />
                <DropdownMenuItem onClick={remove} className="text-rose-400 focus:text-rose-400">
                    <Trash2 className="mr-2 h-3.5 w-3.5" /> Delete
                </DropdownMenuItem>
            </DropdownMenuContent>
        </DropdownMenu>
    );
}

function LaunchWizard({ open, onOpenChange, onLaunched }) {
    const [form, setForm] = useState({
        namespace: "default",
        name: "",
        image: "",
        replicas: 1,
        port: 8080,
        cpu: "250m",
        memory: "256Mi",
        expose: false,
    });
    const [intent, setIntent] = useState("");
    const [intentPreview, setIntentPreview] = useState(null);
    const [manifest, setManifest] = useState("");
    const [validation, setValidation] = useState(null);
    const [applyOpen, setApplyOpen] = useState(false);
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const launchForm = async () => {
        setBusy(true);
        try {
            await api.post("/workloads", form);
            toast.success(`${form.name} launched`);
            onOpenChange(false);
            onLaunched();
        } catch (e) {
            toast.error("Launch failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    const parseIntent = async () => {
        setBusy(true);
        try {
            const { data } = await api.post("/intent/parse", { intent });
            setIntentPreview(data);
        } catch (e) {
            toast.error("Parse failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    const placeIntent = async () => {
        setBusy(true);
        try {
            await api.post("/intent/place", { intent });
            toast.success("Workload placed from intent");
            onOpenChange(false);
            onLaunched();
        } catch (e) {
            toast.error("Place failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    const validate = async () => {
        setBusy(true);
        try {
            const { data } = await api.post("/validate", { manifest });
            setValidation(data);
        } catch (e) {
            setValidation({ valid: false, errors: [e?.response?.data?.error || e.message] });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-lg border-white/10">
                <DialogHeader>
                    <DialogTitle>Run workload</DialogTitle>
                </DialogHeader>
                <Tabs defaultValue="form">
                    <TabsList className="bg-white/[0.03] p-1">
                        <TabsTrigger value="form">Form</TabsTrigger>
                        <TabsTrigger value="intent">Intent</TabsTrigger>
                        <TabsTrigger value="yaml">Validate YAML</TabsTrigger>
                    </TabsList>

                    <TabsContent value="form" className="mt-4 space-y-3">
                        <div className="grid grid-cols-2 gap-3">
                            <div className="space-y-1.5">
                                <Label className="text-xs text-muted-foreground">Namespace</Label>
                                <Input value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} className="border-white/10 bg-white/[0.03]" />
                            </div>
                            <div className="space-y-1.5">
                                <Label className="text-xs text-muted-foreground">Name</Label>
                                <Input value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="checkout-worker" className="border-white/10 bg-white/[0.03]" />
                            </div>
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Image</Label>
                            <Input value={form.image} onChange={(e) => set({ image: e.target.value })} placeholder="ghcr.io/zyvorai/checkout:v1" className="border-white/10 bg-white/[0.03]" />
                        </div>
                        <div className="grid grid-cols-3 gap-3">
                            <div className="space-y-1.5">
                                <Label className="text-xs text-muted-foreground">Replicas</Label>
                                <Input type="number" min={1} value={form.replicas} onChange={(e) => set({ replicas: Number(e.target.value) })} className="border-white/10 bg-white/[0.03]" />
                            </div>
                            <div className="space-y-1.5">
                                <Label className="text-xs text-muted-foreground">CPU</Label>
                                <Input value={form.cpu} onChange={(e) => set({ cpu: e.target.value })} className="border-white/10 bg-white/[0.03]" />
                            </div>
                            <div className="space-y-1.5">
                                <Label className="text-xs text-muted-foreground">Memory</Label>
                                <Input value={form.memory} onChange={(e) => set({ memory: e.target.value })} className="border-white/10 bg-white/[0.03]" />
                            </div>
                        </div>
                        <div className="flex items-center justify-between rounded-xl border border-white/10 bg-white/[0.03] px-3 py-2.5">
                            <div className="space-y-1.5">
                                <Label className="text-xs text-muted-foreground">Port</Label>
                                <Input type="number" value={form.port} onChange={(e) => set({ port: Number(e.target.value) })} className="h-7 w-24 border-white/10 bg-white/[0.03]" />
                            </div>
                            <div className="flex items-center gap-2">
                                <Label className="text-xs text-muted-foreground">Expose service</Label>
                                <Switch checked={form.expose} onCheckedChange={(v) => set({ expose: v })} />
                            </div>
                        </div>
                        <DialogFooter>
                            <Button variant="brand" onClick={launchForm} disabled={busy || !form.name || !form.image} className="rounded-xl">
                                {busy ? "Launching…" : "Launch"}
                            </Button>
                        </DialogFooter>
                    </TabsContent>

                    <TabsContent value="intent" className="mt-4 space-y-3">
                        <Textarea
                            value={intent}
                            onChange={(e) => setIntent(e.target.value)}
                            placeholder="a small stateless api on 2 replicas running ghcr.io/zyvorai/checkout:v1 exposed on port 8080"
                            className="min-h-[100px] border-white/10 bg-white/[0.03] mono text-xs"
                        />
                        {intentPreview && (
                            <pre className="mono max-h-40 overflow-auto whitespace-pre-wrap rounded-lg border border-white/10 bg-black/30 p-3 text-[11px] text-muted-foreground">
                                {intentPreview.receipt}
                            </pre>
                        )}
                        <DialogFooter className="gap-2">
                            <Button variant="outline" onClick={parseIntent} disabled={busy || !intent} className="rounded-xl border-white/10 bg-white/[0.03]">
                                Preview
                            </Button>
                            <Button onClick={placeIntent} disabled={busy || !intent} className="rounded-xl">
                                {busy ? "Placing…" : "Place"}
                            </Button>
                        </DialogFooter>
                    </TabsContent>

                    <TabsContent value="yaml" className="mt-4 space-y-3">
                        <Textarea
                            value={manifest}
                            onChange={(e) => setManifest(e.target.value)}
                            placeholder={"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: example\n..."}
                            className="min-h-[140px] border-white/10 bg-white/[0.03] mono text-xs"
                        />
                        {validation && (
                            <div className={`rounded-lg border px-3 py-2 text-xs ${validation.valid ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300" : "border-rose-500/30 bg-rose-500/10 text-rose-300"}`}>
                                {validation.valid ? `Valid ${validation.kind || "manifest"}` : (validation.errors || [validation.message]).join(", ")}
                            </div>
                        )}
                        <DialogFooter className="gap-2">
                            <Button variant="outline" onClick={validate} disabled={busy || !manifest} className="rounded-xl border-white/10 bg-white/[0.03]">
                                Validate
                            </Button>
                            <Button
                                onClick={() => setApplyOpen(true)}
                                disabled={!validation?.valid}
                                className="rounded-xl"
                            >
                                Apply
                            </Button>
                        </DialogFooter>
                    </TabsContent>
                </Tabs>
            </DialogContent>
            <ApplyManifestDialog
                open={applyOpen}
                onOpenChange={setApplyOpen}
                title="Apply workload manifest"
                initialManifest={manifest}
                onApplied={() => {
                    setApplyOpen(false);
                    onOpenChange(false);
                    onLaunched();
                }}
            />
        </Dialog>
    );
}

export default function Workloads() {
    const [data, setData] = useState(null);
    const [q, setQ] = useState("");
    const [ns, setNs] = useState("all");
    const [kind, setKind] = useState("all");
    const [scaleTarget, setScaleTarget] = useState(null);
    const [rolloutTarget, setRolloutTarget] = useState(null);
    const [launchOpen, setLaunchOpen] = useState(false);
    const [selected, setSelected] = useState(new Set());
    const [bulkBusy, setBulkBusy] = useState(false);

    const load = () => api.get("/workloads").then((r) => setData(r.data));

    useEffect(() => {
        let alive = true;
        const tick = () => api.get("/workloads").then((r) => alive && setData(r.data));
        tick();
        const t = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    const { namespaces, kinds, rows } = useMemo(() => {
        if (!data) return { namespaces: [], kinds: [], rows: [] };
        const namespaces = [...new Set(data.map((w) => w.namespace))].sort();
        const kinds = [...new Set(data.map((w) => w.kind))].sort();
        const rows = data.filter(
            (w) =>
                (ns === "all" || w.namespace === ns) &&
                (kind === "all" || w.kind === kind) &&
                (q === "" ||
                    w.name.toLowerCase().includes(q.toLowerCase()) ||
                    w.image.toLowerCase().includes(q.toLowerCase()))
        );
        return { namespaces, kinds, rows };
    }, [data, q, ns, kind]);

    const key = (w) => `${w.namespace}/${w.kind}/${w.name}`;
    const toggle = (w, checked) => {
        setSelected((s) => {
            const next = new Set(s);
            if (checked) next.add(key(w));
            else next.delete(key(w));
            return next;
        });
    };
    const toggleAll = (checked) => setSelected(checked ? new Set(rows.map(key)) : new Set());

    const bulkAction = async (action) => {
        const targets = rows.filter((w) => selected.has(key(w)));
        // rows reflects the last poll, which can be stale relative to
        // `selected` — a selected row may have been deleted elsewhere (or
        // filtered out) in the up-to-10s since it was picked. Track that
        // gap explicitly rather than silently acting on fewer items than
        // the user actually selected.
        const skipped = selected.size - targets.length;
        setBulkBusy(true);
        try {
            const results = await Promise.allSettled(
                targets.map((w) =>
                    action === "delete"
                        ? api.delete(`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}`)
                        : api.post(`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}/${action}`)
                )
            );
            const failed = results.filter((r) => r.status === "rejected").length;
            const skippedNote = skipped > 0 ? ` (${skipped} selected item${skipped === 1 ? "" : "s"} no longer listed, skipped)` : "";
            if (failed) toast.error(`${failed} of ${targets.length} ${action}s failed${skippedNote}`);
            else toast.success(`${action === "delete" ? "Deleted" : "Restarted"} ${targets.length} workload${targets.length === 1 ? "" : "s"}${skippedNote}`);
            setSelected(new Set());
            load();
        } finally {
            setBulkBusy(false);
        }
    };

    if (!data) {
        return (
            <div className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-96 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    return (
        <div className="space-y-4">
            <div className="flex items-end justify-between">
                <div>
                    <h1 className="text-3xl font-semibold tracking-tight">Workloads</h1>
                    <p className="text-sm text-muted-foreground">
                        Deployments, StatefulSets, DaemonSets, Pods and CronJobs.
                    </p>
                </div>
                <div className="flex items-center gap-2">
                    {selected.size > 0 && (
                        <div className="flex items-center gap-2 rounded-xl border border-white/10 bg-white/[0.03] px-3 py-1.5">
                            <span className="text-xs text-muted-foreground">{selected.size} selected</span>
                            <Button size="sm" variant="outline" disabled={bulkBusy} onClick={() => bulkAction("restart")} className="h-7 rounded-lg border-white/10 bg-white/[0.03] text-xs">
                                <RotateCw className="mr-1 h-3 w-3" /> Restart
                            </Button>
                            <Button size="sm" variant="outline" disabled={bulkBusy} onClick={() => bulkAction("delete")} className="h-7 rounded-lg border-rose-500/30 bg-rose-500/10 text-xs text-rose-300 hover:bg-rose-500/20">
                                <Trash2 className="mr-1 h-3 w-3" /> Delete
                            </Button>
                        </div>
                    )}
                    <Button variant="brand" size="sm" onClick={() => setLaunchOpen(true)} className="rounded-xl">
                        <Plus className="mr-1.5 h-3.5 w-3.5" /> Run workload
                    </Button>
                </div>
            </div>

            <GlassCard className="p-3">
                <div className="flex flex-wrap items-center gap-2">
                    <div className="relative flex-1 min-w-[220px]">
                        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                        <Input
                            placeholder="Search name or image"
                            value={q}
                            onChange={(e) => setQ(e.target.value)}
                            className="border-white/10 bg-white/[0.03] pl-9"
                        />
                    </div>
                    <Select value={ns} onValueChange={setNs}>
                        <SelectTrigger className="w-[180px] border-white/10 bg-white/[0.03]">
                            <SelectValue placeholder="Namespace" />
                        </SelectTrigger>
                        <SelectContent>
                            <SelectItem value="all">All namespaces</SelectItem>
                            {namespaces.map((n) => (
                                <SelectItem key={n} value={n}>
                                    {n}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                    <Select value={kind} onValueChange={setKind}>
                        <SelectTrigger className="w-[160px] border-white/10 bg-white/[0.03]">
                            <SelectValue placeholder="Kind" />
                        </SelectTrigger>
                        <SelectContent>
                            <SelectItem value="all">All kinds</SelectItem>
                            {kinds.map((k) => (
                                <SelectItem key={k} value={k}>
                                    {k}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                </div>
            </GlassCard>

            <GlassCard className="p-0 overflow-hidden">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin">
                    <Table>
                        <TableHeader className="sticky top-0 z-10 backdrop-blur-xl bg-background/60">
                            <TableRow className="border-white/5 hover:bg-transparent">
                                <TableHead className="w-10">
                                    <Checkbox
                                        checked={rows.length > 0 && selected.size === rows.length}
                                        onCheckedChange={toggleAll}
                                    />
                                </TableHead>
                                <TableHead>Name</TableHead>
                                <TableHead>Namespace</TableHead>
                                <TableHead>Kind</TableHead>
                                <TableHead>Desired</TableHead>
                                <TableHead>Ready</TableHead>
                                <TableHead>Available</TableHead>
                                <TableHead>Status</TableHead>
                                <TableHead>Age</TableHead>
                                <TableHead />
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((w) => (
                                <TableRow
                                    key={`${w.namespace}/${w.name}`}
                                    className="border-white/5 hover:bg-white/[0.03]"
                                >
                                    <TableCell>
                                        <Checkbox checked={selected.has(key(w))} onCheckedChange={(v) => toggle(w, v)} />
                                    </TableCell>
                                    <TableCell className="max-w-xs">
                                        <Link
                                            to={`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}`}
                                            className="block truncate font-medium hover:underline"
                                            title={w.name}
                                        >
                                            {w.name}
                                        </Link>
                                        <div className="mono truncate text-[11px] text-muted-foreground">{w.image}</div>
                                    </TableCell>
                                    <TableCell className="mono text-xs">{w.namespace}</TableCell>
                                    <TableCell className="text-xs">{w.kind}</TableCell>
                                    <TableCell className="mono tabular-nums">{w.desired}</TableCell>
                                    <TableCell className="mono tabular-nums">{w.ready}</TableCell>
                                    <TableCell className="mono tabular-nums">{w.available}</TableCell>
                                    <TableCell>
                                        <StatusBadge status={w.status} />
                                    </TableCell>
                                    <TableCell className="mono text-xs">{w.age}</TableCell>
                                    <TableCell>
                                        <WorkloadActions
                                            w={w}
                                            onChanged={load}
                                            onScale={() => setScaleTarget(w)}
                                            onRollout={() => setRolloutTarget(w)}
                                        />
                                    </TableCell>
                                </TableRow>
                            ))}
                            {rows.length === 0 && (
                                <TableRow>
                                    <TableCell colSpan={10} className="py-10 text-center text-sm text-muted-foreground">
                                        No workloads match the current filters.
                                    </TableCell>
                                </TableRow>
                            )}
                        </TableBody>
                    </Table>
                </div>
            </GlassCard>

            {scaleTarget && <ScaleDialog workload={scaleTarget} onOpenChange={(o) => !o && setScaleTarget(null)} onDone={load} />}
            {rolloutTarget && <RolloutDialog workload={rolloutTarget} onOpenChange={(o) => !o && setRolloutTarget(null)} />}
            <LaunchWizard open={launchOpen} onOpenChange={setLaunchOpen} onLaunched={load} />
        </div>
    );
}
