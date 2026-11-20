import { cn } from "@/lib/utils";

/** Flat key=value text chip (no border box). */
export default function KvChip({ name, value, tone = "sky", className }) {
    const tones = {
        sky: "text-tone-link dark:text-tone-link",
        sage: "text-tone-green dark:text-tone-green",
        gold: "text-tone-amber dark:text-tone-amber",
        lavender: "text-tone-purple dark:text-tone-purple",
        rose: "text-tone-red dark:text-tone-red",
        muted: "text-muted-foreground",
    };
    return (
        <span className={cn("inline-flex max-w-full items-baseline gap-0.5 mono text-[11px]", tones[tone] || tones.sky, className)}>
            <span className="shrink-0">{name}</span>
            <span className="text-muted-foreground">=</span>
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
