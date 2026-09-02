import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import AutoTable from "@/components/AutoTable";
import FeatureTile from "@/components/FeatureTile";
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
import { Database, Server, HardDrive, Package, ExternalLink, Plus, ShieldAlert } from "lucide-react";
import { toast } from "sonner";

const POLICIES = ["production", "database", "development", "shared", "ai"];

function CreateVolumeDialog({ open, onOpenChange, onCreated }) {
    const [form, setForm] = useState({ name: "", policy: "development", size_gi: 10, namespace: "default", create_pvc: true });
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const submit = async () => {
        if (!form.name) return;
        setBusy(true);
        try {
            await api.post("/atlas/volumes", form);
            toast.success(`Volume ${form.name} requested`, { description: "Provisioning runs as an async job in Atlas." });
            onOpenChange(false);
            onCreated?.();
        } catch (e) {
            toast.error("Create volume failed", { description: e?.response?.data?.error?.message || e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-md">
                <DialogHeader>
                    <DialogTitle>Create volume</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Name</Label>
                        <Input value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="billing-db-root" className="bg-muted/30" />
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Policy</Label>
                        <Select value={form.policy} onValueChange={(v) => set({ policy: v })}>
                            <SelectTrigger className="bg-muted/30">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                {POLICIES.map((p) => (
                                    <SelectItem key={p} value={p}>
                                        {p}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Size (GiB)</Label>
                            <Input type="number" min={1} value={form.size_gi} onChange={(e) => set({ size_gi: Number(e.target.value) })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Namespace</Label>
                            <Input value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} className="bg-muted/30" />
                        </div>
                    </div>
                </div>
                <DialogFooter>
                    <Button variant="brand" onClick={submit} disabled={busy || !form.name} className="rounded-xl">
                        {busy ? "Requesting…" : "Create"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

export default function Atlas() {
    const [data, setData] = useState(null);
    const [dialogOpen, setDialogOpen] = useState(false);

    const load = () => api.get("/atlas").then((r) => setData(r.data));

    useEffect(() => {
        let alive = true;
        const tick = () => api.get("/atlas").then((r) => alive && setData(r.data));
        tick();
        const t = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    if (!data) return <Skeleton className="h-64 rounded-2xl bg-muted" />;

    if (!data.available) {
        return (
            <div className="space-y-6">
                <PageHeader
                    eyebrow="ATLAS"
                    title="Atlas"
                    description="Storage control plane — Ceph, NFS and ZFS behind one API."
                />
                <GlassCard className="flex items-center gap-4 p-6">
                    <div className="grid h-12 w-12 shrink-0 place-items-center rounded-2xl bg-amber-500/10 text-amber-400">
                        <ShieldAlert className="h-5 w-5" />
                    </div>
                    <div>
                        <div className="text-sm font-medium">Atlas not connected</div>
                        <div className="mt-0.5 text-xs text-muted-foreground">{data.reason || "Configure the Atlas gateway URL."}</div>
                    </div>
                    <Link to="/settings" className="ml-auto">
                        <Button variant="outline" className="rounded-xl bg-muted/30">
                            Open Settings
                        </Button>
                    </Link>
                </GlassCard>
            </div>
        );
    }

    const backends = data.backends || [];
    const pools = data.pools || [];
    const volumes = data.volumes || [];
    const buckets = data.buckets || [];
    const policies = data.policies || [];
    const storageClasses = data.storageClasses || [];
    const metrics = data.metrics;

    return (
        <div className="space-y-6">
            <PageHeader
                eyebrow="ATLAS"
                title="Atlas"
                description="Storage control plane — Ceph, NFS and ZFS behind one API."
                action={
                    <>
                        {data.consoleURL && (
                            <a href={data.consoleURL} target="_blank" rel="noreferrer">
                                <Button variant="outline" className="rounded-xl bg-muted/30">
                                    <ExternalLink className="mr-1.5 h-3.5 w-3.5" /> Console
                                </Button>
                            </a>
                        )}
                        <Button variant="brand" onClick={() => setDialogOpen(true)} className="rounded-xl">
                            <Plus className="mr-1.5 h-3.5 w-3.5" /> Create volume
                        </Button>
                    </>
                }
            />

            {metrics && (
                <div className="grid gap-4 md:grid-cols-3">
                    {[
                        { label: "Raw capacity", value: metrics.raw_capacity_bytes, icon: Server },
                        { label: "Used", value: metrics.used_capacity_bytes, icon: HardDrive },
                        { label: "Available", value: metrics.available_capacity_bytes, icon: Database },
                    ].map((c) => (
                        <FeatureTile
                            key={c.label}
                            label={c.label}
                            icon={c.icon}
                            value={
                                <>
                                    {typeof c.value === "number" ? (c.value / 1024 ** 3).toFixed(1) : "—"}
                                    <span className="text-sm text-muted-foreground"> GiB</span>
                                </>
                            }
                        />
                    ))}
                </div>
            )}

            <div className="grid gap-4 lg:grid-cols-2">
                <AutoTable title="Backends" icon={Server} items={backends} emptyHint="No storage backends registered." />
                <AutoTable title="Pools" icon={Database} items={pools} emptyHint="No pools discovered yet." />
            </div>

            <AutoTable title="Volumes" icon={HardDrive} items={volumes} emptyHint="No volumes provisioned through Atlas yet." />

            <div className="grid gap-4 lg:grid-cols-2">
                <AutoTable title="Buckets" icon={Package} items={buckets} emptyHint="No object buckets provisioned." />
                <AutoTable title="Policies" icon={ShieldAlert} items={policies} emptyHint="No intent policies configured." />
            </div>

            {storageClasses.length > 0 && (
                <GlassCard className="p-4">
                    <div className="mb-3 text-sm font-medium">StorageClasses</div>
                    <div className="flex flex-wrap gap-2">
                        {storageClasses.map((sc, i) => (
                            <span key={i} className="mono border-y border-border px-2.5 py-1 text-xs">
                                {sc.name ?? String(sc)}
                            </span>
                        ))}
                    </div>
                </GlassCard>
            )}

            <CreateVolumeDialog open={dialogOpen} onOpenChange={setDialogOpen} onCreated={load} />
        </div>
    );
}
