import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import AutoTable from "@/components/AutoTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { Trash2 } from "lucide-react";
import { resolveIcon } from "@/constants/navItems";

function fieldValue(row, path) {
    if (path === "name") return row?.metadata?.name ?? row?.name;
    if (path === "namespace") return row?.metadata?.namespace ?? row?.namespace;
    if (path === "age") return row?.metadata?.creationTimestamp ?? row?.creationTimestamp ?? row?.age;
    return path.split(".").reduce((acc, k) => (acc == null ? undefined : acc[k]), row);
}

function displayFieldValue(row, col) {
    const raw = fieldValue(row, col.path);
    if (col.type === "age") {
        if (!raw) return "—";
        const parsed = Date.parse(raw);
        return Number.isNaN(parsed) ? String(raw) : new Date(parsed).toLocaleString();
    }
    return String(raw ?? "—");
}

function InstanceDetail({ manifest, instance, onOpenChange }) {
    if (!instance) return null;
    const name = fieldValue(instance, "name") || "—";
    return (
        <Dialog open onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-2xl border-white/10">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">
                        {manifest.target.kind} · {name}
                    </DialogTitle>
                </DialogHeader>
                {manifest.columns && manifest.columns.length > 0 && (
                    <div className="grid gap-2 rounded-lg border border-white/10 bg-white/[0.02] p-3 text-xs">
                        {manifest.columns.map((c) => (
                            <div key={c.path} className="flex justify-between gap-4">
                                <span className="text-muted-foreground">{c.label}</span>
                                <span className="mono truncate text-foreground">{displayFieldValue(instance, c)}</span>
                            </div>
                        ))}
                    </div>
                )}
                <pre className="mono max-h-[40vh] overflow-auto whitespace-pre-wrap rounded-lg border border-white/10 bg-black/30 p-3 text-[11px] text-foreground/90">
                    {JSON.stringify(instance, null, 2)}
                </pre>
            </DialogContent>
        </Dialog>
    );
}

export default function PluginView() {
    const { pluginId } = useParams();
    const [manifest, setManifest] = useState(null);
    const [notFound, setNotFound] = useState(false);
    const [instances, setInstances] = useState(null);
    const [instancesError, setInstancesError] = useState(null);
    const [detail, setDetail] = useState(null);

    useEffect(() => {
        setManifest(null);
        setNotFound(false);
        api.get("/plugins").then((r) => {
            const found = (r.data || []).find((m) => m.name === pluginId);
            if (!found) {
                setNotFound(true);
                return;
            }
            setManifest(found);
        });
    }, [pluginId]);

    const load = () => {
        if (!manifest) return;
        setInstances(null);
        setInstancesError(null);
        const { group, version, plural } = manifest.target;
        api
            .get(`/crds/${group}/${version}/${plural}`)
            .then((r) => setInstances(r.data || []))
            .catch((e) => {
                setInstances([]);
                setInstancesError(e?.response?.data?.error || e.message);
            });
    };

    useEffect(load, [manifest]); // eslint-disable-line react-hooks/exhaustive-deps

    const openInstance = (row) => setDetail(row);

    const deleteInstance = async (row) => {
        const { group, version, plural } = manifest.target;
        const ns = fieldValue(row, "namespace");
        const name = fieldValue(row, "name");
        try {
            await api.delete(`/crds/${group}/${version}/${plural}/${encodeURIComponent(name)}`, {
                params: ns ? { namespace: ns } : {},
            });
            toast.success(`Deleted ${name}`);
            load();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };

    if (notFound) {
        return (
            <GlassCard className="flex items-center justify-center p-16 text-sm text-muted-foreground">
                Plugin "{pluginId}" was not found. It may have been removed.
            </GlassCard>
        );
    }

    if (!manifest) {
        return (
            <div className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-96 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    const Icon = resolveIcon(manifest.icon);
    const hasDelete = (manifest.actions || []).some((a) => a.type === "delete");

    return (
        <div className="space-y-4">
            <div>
                <h1 className="text-3xl font-semibold tracking-tight">{manifest.displayName}</h1>
                <p className="text-sm text-muted-foreground">
                    {manifest.target.kind} · {manifest.target.group}/{manifest.target.version}
                </p>
            </div>

            <AutoTable
                title={manifest.displayName}
                icon={Icon}
                items={instances}
                columns={manifest.columns}
                onRowClick={openInstance}
                emptyHint={instances === null ? "Loading…" : instancesError ? `Permission denied: ${instancesError}` : "No instances found."}
                rowActions={
                    hasDelete
                        ? (row) => (
                              <Button
                                  size="sm"
                                  variant="ghost"
                                  onClick={() => deleteInstance(row)}
                                  className="h-7 px-2 text-muted-foreground hover:text-rose-400"
                              >
                                  <Trash2 className="h-3.5 w-3.5" />
                              </Button>
                          )
                        : undefined
                }
            />

            {detail && <InstanceDetail manifest={manifest} instance={detail} onOpenChange={(o) => !o && setDetail(null)} />}
        </div>
    );
}
