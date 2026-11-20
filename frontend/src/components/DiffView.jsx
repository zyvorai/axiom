import { useMemo } from "react";
import { DIFF } from "@/constants/testIds";
import { diffLines, diffStats, withContext } from "@/lib/diff";
import { useTranslation } from "react-i18next";

const STYLE = {
    add: "bg-soft-success text-tone-green",
    del: "bg-soft-danger text-tone-red",
    same: "text-muted-foreground",
};
const MARK = { add: "+", del: "-", same: " " };
// Screen-reader prefixes (translated at render).
const SR_KEYS = { add: "pages:diffReview.srAdded", del: "pages:diffReview.srRemoved" };

/** A unified line diff of two texts. Unchanged runs far from a change are collapsed. */
export default function DiffView({ before, after, label }) {
    const { t } = useTranslation();
    const lines = useMemo(() => diffLines(before, after), [before, after]);
    const stats = useMemo(() => diffStats(lines), [lines]);
    const rows = useMemo(() => withContext(lines, 3), [lines]);

    return (
        <div>
            <div data-testid={DIFF.stats} className="mb-1.5 flex items-center gap-3 text-[12px] text-ink-3">
                <span className="text-tone-green">+{stats.added}</span>
                <span className="text-tone-red">-{stats.removed}</span>
                {!stats.changed && <span>{t("pages:diffReview.noChanges")}</span>}
            </div>
            <div
                data-testid={DIFF.view}
                role="region"
                aria-label={label ?? t("pages:diffReview.changes")}
                tabIndex={0}
                className="max-h-[320px] overflow-auto rounded-md border border-border scrollbar-thin"
            >
                <pre className="mono m-0 min-w-max py-1 text-[12px] leading-[1.55]">
                    {rows.map((r, i) =>
                        r.type === "skip" ? (
                            <div key={i} className="px-3 text-ink-3">
                                {t("pages:diffReview.unchangedLines", { count: r.count })}
                            </div>
                        ) : (
                            <div key={i} data-diff={r.type} className={`px-3 ${STYLE[r.type]}`}>
                                <span aria-hidden="true">{MARK[r.type]} </span>
                                <span className="sr-only">{SR_KEYS[r.type] ? t(SR_KEYS[r.type]) : ""}</span>
                                {r.text || " "}
                            </div>
                        )
                    )}
                </pre>
            </div>
        </div>
    );
}
