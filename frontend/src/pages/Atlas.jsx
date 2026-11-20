import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
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
    const { t } = useTranslation();
    const [form, setForm] = useState({ name: "", policy: "development", size_gi: 10, namespace: "default", create_pvc: true });
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const submit = async () => {
        if (!form.name) return;
        setBusy(true);
        try {
            await api.post("/atlas/volumes", form);
            toast.success(t("toasts:atlas.volumeRequested", { name: form.name }), { description: t("toasts:atlas.volumeRequestedDescription") });
            onOpenChange(false);
            onCreated?.();
        } catch (e) {
            toast.error(t("toasts:atlas.createFailed"), { description: e?.response?.data?.error?.message || e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>{t("dialogs:atlas.createVolume.title")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label htmlFor="atlas-name" className="text-xs text-muted-foreground">{t("common:fields.name")}</Label>
                        <Input id="atlas-name" value={form.name} onChange={(e) => set({ name: e.target.value })} placeholder="billing-db-root" className="bg-muted/30" />
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="atlas-policy" className="text-xs text-muted-foreground">{t("dialogs:atlas.createVolume.policy")}</Label>
                        <Select value={form.policy} onValueChange={(v) => set({ policy: v })}>
                            <SelectTrigger id="atlas-policy" className="bg-muted/30">
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
                            <Label htmlFor="atlas-size" className="text-xs text-muted-foreground">{t("dialogs:atlas.createVolume.size")}</Label>
                            <Input id="atlas-size" type="number" min={1} value={form.size_gi} onChange={(e) => set({ size_gi: Number(e.target.value) })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="atlas-namespace" className="text-xs text-muted-foreground">{t("common:fields.namespace")}</Label>
                            <Input id="atlas-namespace" value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} className="bg-muted/30" />
                        </div>
                    </div>
                </div>
                <DialogFooter>
                    <Button variant="brand" onClick={submit} disabled={busy || !form.name} className="rounded-xl">
                        {busy ? t("dialogs:atlas.createVolume.requesting") : t("common:actions.create")}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

export default function Atlas() {
    const { t } = useTranslation();
    const [data, setData] = useState(null);
    const [dialogOpen, setDialogOpen] = useState(false);

    const load = () => api.get("/atlas").then((r) => setData(r.data));

    useEffect(() => {
        let alive = true;
        const tick = () => api.get("/atlas").then((r) => alive && setData(r.data));
        tick();
        const timer = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    if (!data) {
        return (
            <div className="page">
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    if (!data.available) {
        return (
            <div className="page">
                <PageHeader
                    title={t("pages:atlas.title")}
                    description={t("pages:atlas.description")}
                />
                <GlassCard className="flex items-center gap-4">
                    <div className="grid h-12 w-12 shrink-0 place-items-center rounded-2xl bg-soft-warning text-tone-amber">
                        <ShieldAlert className="h-5 w-5" />
                    </div>
                    <div>
                        <div className="text-sm font-medium">{t("pages:atlas.notConnected")}</div>
                        <div className="mt-0.5 text-xs text-muted-foreground">{data.reason || t("pages:atlas.configureGateway")}</div>
                    </div>
                    <Link to="/settings" className="btn btn-line ml-auto">
                        {t("pages:atlas.openSettings")}
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
        <div className="page">
            <PageHeader
                title={t("pages:atlas.title")}
                description={t("pages:atlas.description")}
                action={
                    <>
                        {data.consoleURL && (
                            <a href={data.consoleURL} target="_blank" rel="noreferrer" className="btn btn-line">
                                <ExternalLink size={14} /> {t("pages:atlas.console")}
                            </a>
                        )}
                        <button type="button" className="btn btn-fill" onClick={() => setDialogOpen(true)}>
                            <Plus size={15} /> {t("pages:atlas.createVolume")}
                        </button>
                    </>
                }
            />

            {metrics && (
                <div className="grid gap-4 md:grid-cols-3">
                    {[
                        { id: "raw", label: t("pages:atlas.metrics.raw"), value: metrics.raw_capacity_bytes, icon: Server },
                        { id: "used", label: t("pages:atlas.metrics.used"), value: metrics.used_capacity_bytes, icon: HardDrive },
                        { id: "available", label: t("pages:atlas.metrics.available"), value: metrics.available_capacity_bytes, icon: Database },
                    ].map((c) => (
                        <FeatureTile
                            key={c.id}
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
                <AutoTable title={t("pages:atlas.backends")} icon={Server} items={backends} emptyHint={t("pages:atlas.noBackends")} />
                <AutoTable title={t("pages:atlas.pools")} icon={Database} items={pools} emptyHint={t("pages:atlas.noPools")} />
            </div>

            <AutoTable title={t("pages:atlas.volumes")} icon={HardDrive} items={volumes} emptyHint={t("pages:atlas.noVolumes")} />

            <div className="grid gap-4 lg:grid-cols-2">
                <AutoTable title={t("pages:atlas.buckets")} icon={Package} items={buckets} emptyHint={t("pages:atlas.noBuckets")} />
                <AutoTable title={t("pages:atlas.policies")} icon={ShieldAlert} items={policies} emptyHint={t("pages:atlas.noPolicies")} />
            </div>

            {storageClasses.length > 0 && (
                <GlassCard>
                    <div className="text-[15px] font-[600] mb-3">StorageClasses</div>
                    <div className="flex flex-wrap gap-2">
                        {storageClasses.map((sc, i) => (
                            <span key={i} className="pill soft mono">
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
