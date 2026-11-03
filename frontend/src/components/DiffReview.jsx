import { useCallback, useRef, useState } from "react";
import { AlertTriangle } from "lucide-react";
import { api } from "@/lib/api";
import { DIFF } from "@/constants/testIds";
import DiffView from "@/components/DiffView";
import { Skeleton } from "@/components/ui/skeleton";
import { useTranslation } from "react-i18next";

/**
 * Change review before apply. `run(manifest)` asks POST /apply/diff what applying the manifest
 * would change; `review` holds the latest answer, tagged with the manifest it was computed for
 * so an edit afterwards makes it stale (see canApply).
 *
 * review.status: "loading" | "ready" (review.result) | "error" (review.error) |
 * "unavailable" (server without /apply/diff — apply is then allowed without a preview).
 */
export function useManifestReview() {
    const [review, setReview] = useState(null);
    const seq = useRef(0);

    const run = useCallback(async (manifest) => {
        const mine = ++seq.current;
        setReview({ manifest, status: "loading" });
        try {
            const { data } = await api.post("/apply/diff", { manifest });
            if (mine === seq.current) setReview({ manifest, status: "ready", result: data });
        } catch (e) {
            if (mine !== seq.current) return;
            const code = e?.response?.status;
            if (code === 404 || code === 405 || code === 501) setReview({ manifest, status: "unavailable" });
            else setReview({ manifest, status: "error", error: e?.response?.data?.error || e.message });
        }
    }, []);

    const reset = useCallback(() => {
        seq.current++;
        setReview(null);
    }, []);

    return { review, run, reset };
}

/** True when `review` is a successful (or unavailable) review of exactly this manifest. */
export function canApply(review, manifest) {
    if (!review || review.manifest !== manifest) return false;
    return review.status === "unavailable" || (review.status === "ready" && review.result?.valid === true);
}

function objectTitle(t, o) {
    const id = `${o.kind} ${o.namespace ? `${o.namespace}/` : ""}${o.name}`;
    if (!o.exists) return t("pages:diffReview.willBeCreated", { id });
    return o.changed ? t("pages:diffReview.willBeChanged", { id }) : t("pages:diffReview.upToDate", { id });
}

export default function DiffReview({ review }) {
    const { t } = useTranslation();
    if (!review) return null;
    if (review.status === "loading") {
        return (
            <div data-testid={DIFF.review} aria-busy="true" aria-live="polite">
                <Skeleton className="h-24 w-full rounded-lg" />
            </div>
        );
    }
    if (review.status === "unavailable") {
        return (
            <div data-testid={DIFF.review} role="status" className="rounded-lg border border-soft-warning-border bg-soft-warning px-3 py-2 text-xs text-soft-warning-text">
                {t("pages:diffReview.unavailable")}
            </div>
        );
    }
    const failure =
        review.status === "error"
            ? [review.error]
            : review.result && !review.result.valid
              ? review.result.errors?.length
                  ? review.result.errors
                  : [review.result.message || t("pages:diffReview.rejected")]
              : null;
    if (failure) {
        return (
            <div data-testid={DIFF.review} role="alert" className="rounded-lg border border-soft-danger-border bg-soft-danger px-3 py-2 text-xs text-tone-red">
                {failure.filter(Boolean).join(", ")}
            </div>
        );
    }

    const { objects = [], warnings = [], message } = review.result;
    return (
        <div data-testid={DIFF.review} className="space-y-3">
            {warnings.map((w, i) => (
                <div
                    key={i}
                    data-testid={DIFF.warning}
                    role="status"
                    className="flex items-start gap-2 rounded-lg border border-soft-warning-border bg-soft-warning px-3 py-2 text-xs text-soft-warning-text"
                >
                    <AlertTriangle size={14} className="mt-0.5 shrink-0" aria-hidden="true" />
                    <span>{w}</span>
                </div>
            ))}
            {objects.map((o) => (
                <section key={`${o.kind}/${o.namespace}/${o.name}`} aria-label={objectTitle(t, o)}>
                    <h3 className="mb-1.5 text-[12.5px] font-medium">{objectTitle(t, o)}</h3>
                    <DiffView before={o.live} after={o.proposed} label={t("pages:diffReview.changesTo", { kind: o.kind, name: o.name })} />
                </section>
            ))}
            {message && <p className="text-[11.5px] text-ink-3">{message}</p>}
        </div>
    );
}
