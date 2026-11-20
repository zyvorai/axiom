import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
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
import MacTerm from "@/components/MacTerm";
import DiffReview, { canApply, useManifestReview } from "@/components/DiffReview";
import { DIFF } from "@/constants/testIds";

// Shared "apply a raw manifest" modal — used by the Workloads launch wizard
// and the NetworkPolicy "create via YAML" flow. Hits the generic
// POST /api/v1/apply endpoint (real server-side apply, kubernetes-tagged
// builds only; demo mode returns a clear "not available" message).
//
// Two steps: "Review changes" asks POST /apply/diff what the manifest would change and shows the
// diff (plus a warning when Argo CD / Flux / Helm also manage the object); Apply is only enabled
// once that review succeeded for exactly the current text, so an edit invalidates it.
export default function ApplyManifestDialog({ open, onOpenChange, title, initialManifest = "", onApplied }) {
    const { t } = useTranslation();
    const [manifest, setManifest] = useState(initialManifest);
    const [result, setResult] = useState(null);
    const [busy, setBusy] = useState(false);
    const { review, run, reset } = useManifestReview();

    useEffect(() => {
        if (open) {
            setManifest(initialManifest);
            setResult(null);
            reset();
        }
    }, [open, initialManifest, reset]);

    const apply = async () => {
        setBusy(true);
        try {
            const { data } = await api.post("/apply", { manifest });
            setResult(data);
            if (data.valid) {
                const kind = data.kind || t("dialogs:applyManifest.manifestWord");
                toast.success(data.name ? t("toasts:applyManifest.appliedNamed", { kind, name: data.name }) : t("toasts:applyManifest.applied", { kind }));
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
            <DialogContent className="max-w-2xl">
                <DialogHeader>
                    <DialogTitle>{title ?? t("dialogs:applyManifest.title")}</DialogTitle>
                </DialogHeader>
                <MacTerm title="manifest.yaml" bodyClassName="!p-0">
                    <Textarea
                        value={manifest}
                        onChange={(e) => setManifest(e.target.value)}
                        placeholder={"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: example\n..."}
                        className="macos-term-body mono min-h-[220px] resize-none rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0"
                        spellCheck={false}
                    />
                </MacTerm>
                <DiffReview review={review && review.manifest === manifest ? review : null} />
                {result && (
                    <div className={`rounded-lg border px-3 py-2 text-xs ${result.valid ? "border-soft-success-border bg-soft-success text-tone-green" : "border-soft-danger-border bg-soft-danger text-tone-red"}`}>
                        {result.valid
                            ? result.name
                                ? t("dialogs:applyManifest.appliedNamed", { kind: result.kind || t("dialogs:applyManifest.manifestWord"), name: `${result.namespace ? `${result.namespace}/` : ""}${result.name}` })
                                : t("dialogs:applyManifest.applied", { kind: result.kind || t("dialogs:applyManifest.manifestWord") })
                            : (result.errors || [result.message]).filter(Boolean).join(", ")}
                    </div>
                )}
                <DialogFooter>
                    <Button
                        variant={canApply(review, manifest) ? "outline" : "brand"}
                        data-testid={DIFF.reviewButton}
                        onClick={() => {
                            setResult(null);
                            run(manifest);
                        }}
                        disabled={busy || !manifest || review?.status === "loading"}
                        className="rounded-xl"
                    >
                        {review?.status === "loading" ? t("dialogs:applyManifest.reviewing") : canApply(review, manifest) ? t("dialogs:applyManifest.reviewAgain") : t("dialogs:applyManifest.reviewChanges")}
                    </Button>
                    <Button
                        variant="brand"
                        data-testid={DIFF.applyButton}
                        onClick={apply}
                        disabled={busy || !manifest || !canApply(review, manifest)}
                        className="rounded-xl"
                    >
                        {busy ? t("dialogs:applyManifest.applying") : t("common:actions.apply")}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
