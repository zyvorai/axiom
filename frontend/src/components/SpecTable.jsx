import { cn } from "@/lib/utils";

function splitRows(rows) {
    const mid = Math.ceil(rows.length / 2);
    return [rows.slice(0, mid), rows.slice(mid)];
}

// Apple tech-specs: section title + hairline rows, no card box.
export default function SpecTable({ title, rows = [], columns = 1, className, ...rest }) {
    const groups = columns === 2 ? splitRows(rows) : [rows];
    return (
        <div className={cn("overflow-hidden", className)} {...rest}>
            {title && <div className="border-b border-border pb-3 text-sm font-medium tracking-[-0.01em]">{title}</div>}
            <div className={cn("grid", columns === 2 && "md:grid-cols-2 md:divide-x md:divide-border")}>
                {groups.map((group, gi) => (
                    <div key={gi} className="divide-y divide-border">
                        {group.map((r, i) => (
                            <div key={r.label ?? i} className="flex items-center justify-between gap-4 py-3 text-sm">
                                <span className="text-muted-foreground">{r.label}</span>
                                <span className={cn("truncate text-right text-foreground", r.mono && "mono")}>
                                    {r.value ?? "—"}
                                </span>
                            </div>
                        ))}
                    </div>
                ))}
            </div>
        </div>
    );
}
