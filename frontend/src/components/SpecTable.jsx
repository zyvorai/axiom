import { cn } from "@/lib/utils";
import GlassCard from "@/components/GlassCard";

function splitRows(rows) {
    const mid = Math.ceil(rows.length / 2);
    return [rows.slice(0, mid), rows.slice(mid)];
}

// Apple "Tech Specs" style label/value table: a section title above hairline-
// divided rows, optionally split into two columns the way apple.com/ipad's
// spec tables do. Wraps GlassCard and forwards ...rest (data-testid included)
// the same way GlassCard itself does, so it's a drop-in replacement for the
// scattered small label/value GlassCards on resource-detail pages.
export default function SpecTable({ title, rows = [], columns = 1, className, ...rest }) {
    const groups = columns === 2 ? splitRows(rows) : [rows];
    return (
        <GlassCard className={cn("overflow-hidden p-0", className)} {...rest}>
            {title && <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">{title}</div>}
            <div className={cn("grid", columns === 2 && "md:grid-cols-2 md:divide-x md:divide-white/5")}>
                {groups.map((group, gi) => (
                    <div key={gi} className="divide-y divide-white/5">
                        {group.map((r, i) => (
                            <div key={r.label ?? i} className="flex items-center justify-between gap-4 px-4 py-3 text-sm">
                                <span className="text-muted-foreground">{r.label}</span>
                                <span className={cn("truncate text-right text-foreground", r.mono && "mono")}>
                                    {r.value ?? "—"}
                                </span>
                            </div>
                        ))}
                    </div>
                ))}
            </div>
        </GlassCard>
    );
}
