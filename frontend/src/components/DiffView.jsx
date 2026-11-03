import { useMemo } from "react";
import { DIFF } from "@/constants/testIds";
import { diffLines, diffStats, withContext } from "@/lib/diff";

const STYLE = {
    add: "bg-soft-success text-tone-green",
    del: "bg-soft-danger text-tone-red",
    same: "text-muted-foreground",
};
const MARK = { add: "+", del: "-", same: " " };
const SR = { add: "added: ", del: "removed: ", same: "" };

/** A unified line diff of two texts. Unchanged runs far from a change are collapsed. */
export default function DiffView({ before, after, label = "Changes" }) {
    const lines = useMemo(() => diffLines(before, after), [before, after]);
    const stats = useMemo(() => diffStats(lines), [lines]);
    const rows = useMemo(() => withContext(lines, 3), [lines]);

    return (
        <div>
            <div data-testid={DIFF.stats} className="mb-1.5 flex items-center gap-3 text-[12px] text-ink-3">
                <span className="text-tone-green">+{stats.added}</span>
                <span className="text-tone-red">-{stats.removed}</span>
                {!stats.changed && <span>no changes</span>}
            </div>
            <div
                data-testid={DIFF.view}
                role="region"
                aria-label={label}
                tabIndex={0}
                className="max-h-[320px] overflow-auto rounded-md border border-border scrollbar-thin"
            >
                <pre className="mono m-0 min-w-max py-1 text-[12px] leading-[1.55]">
                    {rows.map((r, i) =>
                        r.type === "skip" ? (
                            <div key={i} className="px-3 text-ink-3">
                                … {r.count} unchanged line{r.count === 1 ? "" : "s"}
                            </div>
                        ) : (
                            <div key={i} data-diff={r.type} className={`px-3 ${STYLE[r.type]}`}>
                                <span aria-hidden="true">{MARK[r.type]} </span>
                                <span className="sr-only">{SR[r.type]}</span>
                                {r.text || " "}
                            </div>
                        )
                    )}
                </pre>
            </div>
        </div>
    );
}
