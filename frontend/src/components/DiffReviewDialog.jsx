import { useEffect, useState } from "react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import { DIFF } from "@/constants/testIds";
import DiffReview, { canApply, useManifestReview } from "@/components/DiffReview";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";

/**
 * "Review changes" step for the YAML editors (Pod / Workload detail). Opening it asks the server
 * what applying `manifest` would change and shows the diff; Apply stays disabled until that
 * review succeeded for exactly this manifest. Apply posts to /apply and reports the outcome
 * honestly (a backend that cannot apply says so and nothing is claimed as applied).
 */
export default function DiffReviewDialog({ open, onOpenChange, manifest, fallbackKind = "manifest", onApplied }) {
    const { t } = useTranslation();
    const { review, run, reset } = useManifestReview();
    const [busy, setBusy] = useState(false);
    const [failure, setFailure] = useState(null);

    useEffect(() => {
        if (open) {
            setFailure(null);
            run(manifest);
        } else {
            reset();
        }
    }, [open, manifest, run, reset]);

    const apply = async () => {
        setBusy(true);
        setFailure(null);
        try {
            const { data } = await api.post("/apply", { manifest });
            if (!data.valid) {
                const msg = (data.errors || [data.message]).filter(Boolean).join(", ");
                toast.error(t("toasts:diffReview.applyFailed"), { description: msg });
                setFailure(msg);
                return;
            }
            toast.success(t("toasts:diffReview.applied", { kind: data.kind || fallbackKind }));
            onOpenChange(false);
            onApplied?.(data);
        } catch (e) {
            const msg = e?.response?.data?.error || e.message;
            toast.error(t("toasts:diffReview.applyFailed"), { description: msg });
            setFailure(msg);
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-2xl" data-testid={DIFF.dialog}>
                <DialogHeader>
                    <DialogTitle>{t("dialogs:diffReview.title")}</DialogTitle>
                    <DialogDescription>
                        {t("dialogs:diffReview.description")}
                    </DialogDescription>
                </DialogHeader>
                <DiffReview review={review} />
                {failure && (
                    <div role="alert" className="rounded-lg border border-soft-danger-border bg-soft-danger px-3 py-2 text-xs text-tone-red">
                        {failure}
                    </div>
                )}
                <DialogFooter>
                    <Button variant="outline" onClick={() => onOpenChange(false)} className="rounded-xl">
                        {t("common:actions.cancel")}
                    </Button>
                    <Button
                        variant="brand"
                        data-testid={DIFF.applyButton}
                        onClick={apply}
                        disabled={busy || !canApply(review, manifest)}
                        className="rounded-xl"
                    >
                        {busy ? t("dialogs:diffReview.applying") : t("common:actions.apply")}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}
