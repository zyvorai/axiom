import { cn } from "@/lib/utils";

// A segmented phase bar — solid = complete, diagonal-striped = active/in
// progress, muted = pending — for places that currently show only a flat
// badge or bare percentage (rollout status, node drain, HPA scaling).
// segments: [{ key, label, value, tone: "done" | "active" | "pending" }]
// value is a relative weight (e.g. replica counts); segments render
// proportionally to their share of the total.
export default function SegmentedProgress({ segments, className }) {
    const total = segments.reduce((sum, s) => sum + Math.max(0, s.value), 0) || 1;

    return (
        <div className={cn("space-y-2", className)}>
            <div className="flex h-2.5 w-full overflow-hidden rounded-full bg-white/[0.04]">
                {segments.map((s) => {
                    const pct = (Math.max(0, s.value) / total) * 100;
                    if (pct <= 0) return null;
                    const style = { width: `${pct}%` };
                    if (s.tone === "active") {
                        // Plain CSS var reference rather than a Tailwind
                        // arbitrary-value class: Tailwind's theme() helper
                        // resolves at build time to a fixed hex, which would
                        // freeze this stripe to one theme instead of
                        // tracking the live --i17-cosmic value per light/dark.
                        style.backgroundImage =
                            "repeating-linear-gradient(135deg, var(--i17-cosmic) 0, var(--i17-cosmic) 4px, transparent 4px, transparent 8px)";
                        style.backgroundColor = "rgb(var(--glass-bg))";
                    }
                    return (
                        <div
                            key={s.key}
                            style={style}
                            className={cn(
                                "h-full first:rounded-l-full last:rounded-r-full",
                                s.tone === "done" && "bg-i17-cosmic",
                                s.tone === "pending" && "bg-white/10"
                            )}
                            title={`${s.label}: ${s.value}`}
                        />
                    );
                })}
            </div>
            <div className="flex flex-wrap gap-x-4 gap-y-1 text-[11px] text-muted-foreground">
                {segments.map((s) => (
                    <span key={s.key} className="inline-flex items-center gap-1.5">
                        <span
                            className={cn(
                                "h-2 w-2 rounded-full",
                                s.tone === "done" && "bg-i17-cosmic",
                                s.tone === "active" && "bg-i17-cosmic/50",
                                s.tone === "pending" && "bg-white/15"
                            )}
                        />
                        {s.label} · {s.value}
                    </span>
                ))}
            </div>
        </div>
    );
}
