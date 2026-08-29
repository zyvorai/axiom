import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { toast } from "sonner";

// Shared "apply a raw manifest" modal — used by the Workloads launch wizard
// and the NetworkPolicy "create via YAML" flow. Hits the generic
// POST /api/v1/apply endpoint (real server-side apply, kubernetes-tagged
// builds only; demo mode returns a clear "not available" message).
export default function ApplyManifestDialog({ open, onOpenChange, title = "Apply manifest", initialManifest = "", onApplied }) {
    const [manifest, setManifest] = useState(initialManifest);
    const [result, setResult] = useState(null);
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        if (open) {
            setManifest(initialManifest);
            setResult(null);
        }
    }, [open, initialManifest]);

    const apply = async () => {
        setBusy(true);
        try {
            const { data } = await api.post("/apply", { manifest });
            setResult(data);
            if (data.valid) {
                toast.success(`Applied ${data.kind || "manifest"}${data.name ? ` ${data.name}` : ""}`);
                onApplied?.();
            }
        } catch (e) {
            setResult({ valid: false, message: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-lg border-white/10">
                <DialogHeader>
                    <DialogTitle>{title}</DialogTitle>
                </DialogHeader>
                <Textarea
                    value={manifest}
                    onChange={(e) => setManifest(e.target.value)}
                    placeholder={"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: example\n..."}
                    className="min-h-[220px] border-white/10 bg-white/[0.03] mono text-xs"
                />
                {result && (
                    <div className={`rounded-lg border px-3 py-2 text-xs ${result.valid ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-300" : "border-rose-500/30 bg-rose-500/10 text-rose-300"}`}>
                        {result.valid
                            ? `Applied ${result.kind || "manifest"}${result.name ? ` ${result.namespace ? `${result.namespace}/` : ""}${result.name}` : ""}`
                            : (result.errors || [result.message]).filter(Boolean).join(", ")}
                    </div>
                )}
                <DialogFooter>
                    <Button variant="brand" onClick={apply} disabled={busy || !manifest} className="rounded-xl">
                        {busy ? "Applying…" : "Apply"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
