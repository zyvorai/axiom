import { cn } from "@/lib/utils";

/** Flat key=value text chip (no border box). */
export default function KvChip({ name, value, tone = "sky", className }) {
    const tones = {
        sky: "text-sky-700 dark:text-sky-300",
        sage: "text-emerald-700 dark:text-emerald-300",
        gold: "text-amber-700 dark:text-amber-300",
        lavender: "text-violet-700 dark:text-violet-300",
        rose: "text-rose-700 dark:text-rose-300",
        muted: "text-muted-foreground",
    };
    return (
        <span className={cn("inline-flex max-w-full items-baseline gap-0.5 mono text-[11px]", tones[tone] || tones.sky, className)}>
            <span className="shrink-0 opacity-70">{name}</span>
            <span className="opacity-40">=</span>
            <span className="min-w-0 truncate font-medium text-foreground">{value}</span>
        </span>
    );
}

export function toneForKey(name) {
    const n = String(name || "").toLowerCase();
    if (/(password|secret|token|key|auth)/.test(n)) return "rose";
    if (/(cpu|memory|mem|resource)/.test(n)) return "gold";
    if (/(image|container|app|name)/.test(n)) return "sage";
    if (/(url|host|addr|port|dns)/.test(n)) return "sky";
    if (/(mode|policy|class)/.test(n)) return "lavender";
    return "sky";
}
