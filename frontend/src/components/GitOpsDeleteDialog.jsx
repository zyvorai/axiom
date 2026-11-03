import { useEffect, useState } from "react";
import { AlertTriangle, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { GITOPS_CREATE as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || "Something went wrong";

/**
 * Delete a GitOps application. The server says what the deletion really does
 * (an Argo CD finalizer, Flux prune and a HelmRelease uninstall all remove the
 * application's resources); when it does, the name has to be typed.
 */
export default function GitOpsDeleteDialog({ app, onClose, onDeleted }) {
    const [preview, setPreview] = useState(null);
    const [error, setError] = useState("");
    const [typed, setTyped] = useState("");
    const [removeSource, setRemoveSource] = useState(false);
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        setPreview(null);
        setError("");
        setTyped("");
        setRemoveSource(false);
        if (!app) return undefined;
        let alive = true;
        api
            .get(`/gitops/${app.kind}/${app.namespace}/${encodeURIComponent(app.name)}/delete-preview`)
            .then((r) => alive && setPreview(r.data))
            .catch((e) => alive && setError(errText(e)));
        return () => {
            alive = false;
        };
    }, [app]);

    const remove = async () => {
        setBusy(true);
        setError("");
        try {
            const { data } = await api.delete(`/gitops/${app.kind}/${app.namespace}/${encodeURIComponent(app.name)}`, {
                params: { removeSource: removeSource ? "true" : "false", confirm: preview.cascade ? typed : undefined },
            });
            toast.success(`Deleted ${app.name}`, { description: (data.warnings || []).join(" ") || undefined });
            onDeleted();
            onClose();
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy(false);
        }
    };

    if (!app) return null;
    const src = preview?.source;
    const shared = src?.sharedWith || [];
    const canRemoveSource = !!src && shared.length === 0 && preview.managedByAxiom;
    const confirmed = !preview?.cascade || typed === app.name;
    return (
        <Dialog open={!!app} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="max-w-lg" data-testid={T.deleteDialog}>
                <DialogHeader>
                    <DialogTitle>Delete {app.name}?</DialogTitle>
                    <DialogDescription>
                        {app.kind} · {app.namespace}
                        {preview?.managedByAxiom ? " · created by Axiom" : ""}
                    </DialogDescription>
                </DialogHeader>

                {!preview && !error ? <Skeleton className="h-20 rounded-xl bg-muted" /> : null}
                {error ? <div className="notice danger" role="alert" data-testid={T.deleteError}>{error}</div> : null}

                {preview && (
                    <div className="space-y-3">
                        <div
                            className={`flex gap-2 rounded-xl border p-3 text-sm ${preview.cascade ? "border-tone-red" : "border-border"}`}
                            data-testid={T.deleteConsequence}
                        >
                            {preview.cascade ? <AlertTriangle size={16} className="mt-0.5 shrink-0 text-tone-red" aria-hidden="true" /> : null}
                            <div>
                                <p>{preview.consequence}</p>
                                {(preview.detail || []).length > 0 && (
                                    <ul className="mono mt-1 text-[11.5px] text-ink-3">
                                        {preview.detail.map((d) => (
                                            <li key={d}>{d}</li>
                                        ))}
                                    </ul>
                                )}
                            </div>
                        </div>

                        {preview.cascade && (
                            <div className="space-y-1.5" data-testid={T.deleteCascade}>
                                <Label htmlFor="gitops-delete-confirm" className="text-xs text-muted-foreground">
                                    This also removes the application&apos;s resources. Type <span className="mono">{app.name}</span> to confirm.
                                </Label>
                                <Input id="gitops-delete-confirm" data-testid={T.deleteConfirmInput} value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" className="bg-muted/30" />
                            </div>
                        )}

                        {src && (
                            <div className="space-y-1">
                                {shared.length > 0 ? (
                                    <p className="text-xs text-ink-3" data-testid={T.deleteSharedNote}>
                                        The source {src.kind}/{src.name} is still used by {shared.map((s) => `${s.kind}/${s.name}`).join(", ")}, so it stays.
                                    </p>
                                ) : canRemoveSource ? (
                                    <label className="flex items-center gap-2 text-sm">
                                        <input type="checkbox" data-testid={T.deleteRemoveSource} checked={removeSource} onChange={(e) => setRemoveSource(e.target.checked)} className="h-4 w-4 rounded border-border accent-primary" />
                                        Also remove the {src.kind} {src.name} (nothing else uses it)
                                    </label>
                                ) : (
                                    <p className="text-xs text-ink-3">The {src.kind} {src.name} was not created by Axiom, so it is left alone.</p>
                                )}
                            </div>
                        )}
                    </div>
                )}

                <DialogFooter>
                    <button type="button" className="btn btn-danger" data-testid={T.deleteConfirm} onClick={remove} disabled={busy || !preview || !confirmed}>
                        <Trash2 size={14} /> {busy ? "Deleting…" : "Delete"}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
