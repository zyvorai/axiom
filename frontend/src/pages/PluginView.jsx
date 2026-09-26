import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { toast } from "sonner";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import AutoTable from "@/components/AutoTable";
import { Skeleton } from "@/components/ui/skeleton";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { Trash2 } from "lucide-react";
import { resolveIcon } from "@/constants/navItems";
import { usePageTitle } from "@/lib/pageTitle";
import { useConfirm } from "@/components/ConfirmDialog";
import CodeEditor from "@/components/CodeEditor";

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
            <DialogContent className="max-w-2xl">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">
                        {manifest.target.kind} · {name}
                    </DialogTitle>
                </DialogHeader>
                {manifest.columns && manifest.columns.length > 0 && (
                    <div className="grid gap-2 rounded-lg border border-soft-info-border bg-soft-info p-3 text-xs">
                        {manifest.columns.map((c) => (
                            <div key={c.path} className="flex justify-between gap-4">
                                <span className="text-tone-link">{c.label}</span>
                                <span className="mono truncate text-tone-link">{displayFieldValue(instance, c)}</span>
                            </div>
                        ))}
                    </div>
                )}
                <CodeEditor value={JSON.stringify(instance, null, 2)} editable={false} className="min-h-0 [&_.code-pane]:min-h-[40vh] [&_.code-pane]:max-h-[40vh]" />
            </DialogContent>
        </Dialog>
    );
}

export default function PluginView() {
    const confirm = useConfirm();
    const { pluginId } = useParams();
    const [manifest, setManifest] = useState(null);
    const [notFound, setNotFound] = useState(false);
    const [instances, setInstances] = useState(null);
    const [instancesError, setInstancesError] = useState(null);
    const [detail, setDetail] = useState(null);
    // the not-found/loading states have no PageHeader, so set the title here too
    usePageTitle(notFound ? "Plugin not found" : manifest ? manifest.displayName : "Plugin");

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
        const ok = await confirm({
            title: `Delete ${name}?`,
            description: "This cannot be undone.",
            confirmLabel: "Delete",
        });
        if (!ok) return;
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
            <div className="page">
                <GlassCard>
                    <div className="empty">Plugin "{pluginId}" was not found. It may have been removed.</div>
                </GlassCard>
            </div>
        );
    }

    if (!manifest) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="mt-4 h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    const Icon = resolveIcon(manifest.icon);
    const hasDelete = (manifest.actions || []).some((a) => a.type === "delete");

    return (
        <div className="page">
            <PageHeader
                title={manifest.displayName}
                description={`${manifest.target.kind} · ${manifest.target.group}/${manifest.target.version}`}
            />

            <AutoTable
                title={`${manifest.target.kind} instances`}
                icon={Icon}
                items={instances}
                columns={manifest.columns}
                onRowClick={openInstance}
                emptyHint={instances === null ? "Loading…" : instancesError ? `Permission denied: ${instancesError}` : "No instances found."}
                rowActions={
                    hasDelete
                        ? (row) => (
                              <button
                                  type="button"
                                  className="btn btn-danger btn-sm"
                                  onClick={() => deleteInstance(row)}
                                  aria-label={`Delete ${fieldValue(row, "name") || "item"}`}
                              >
                                  <Trash2 size={13} />
                              </button>
                          )
                        : undefined
                }
            />

            {detail && <InstanceDetail manifest={manifest} instance={detail} onOpenChange={(o) => !o && setDetail(null)} />}
        </div>
    );
}
