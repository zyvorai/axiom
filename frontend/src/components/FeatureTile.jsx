import { cn } from "@/lib/utils";
import GlassCard from "@/components/GlassCard";
import Sparkline from "@/components/Sparkline";

// Apple homepage-grid style stat tile: an icon chip + tracked-caps label,
// an oversized value, and an optional sparkline/sublabel underneath — the
// apple.com/ipad "feature tile" shape scaled down to fit a metrics grid.
// `size="compact"` matches the smaller inline icon+label+mono-value blocks
// already used on Nodes/NodeDetail/PodDetail so those can adopt the same
// component without a layout regression. Wraps GlassCard and forwards
// ...rest (data-testid included) exactly like GlassCard does.
export default function FeatureTile({
    icon: Icon,
    iconClassName,
    label,
    value,
    sublabel,
    series,
    color = "var(--i17-cosmic)",
    mono = false,
    size = "default",
    className,
    ...rest
}) {
    if (size === "compact") {
        return (
            <GlassCard className={cn("p-4", className)} {...rest}>
                <div className="flex items-center gap-1 text-[10px] uppercase tracking-widest text-muted-foreground">
                    {Icon && <Icon className={cn("h-3 w-3", iconClassName)} />} {label}
                </div>
                <div className={cn("mt-1 text-xl", mono && "mono")}>{value}</div>
                {sublabel && <div className="mt-0.5 text-xs text-muted-foreground">{sublabel}</div>}
            </GlassCard>
        );
    }

    return (
        <GlassCard className={cn("p-5", className)} {...rest}>
            <div className="flex items-center justify-between">
                <div className="text-[11px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">{label}</div>
                {Icon && (
                    <div className="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-foreground/[0.06]">
                        <Icon className={cn("h-3.5 w-3.5 text-muted-foreground", iconClassName)} />
                    </div>
                )}
            </div>
            <div className={cn("mt-2 text-3xl font-bold tracking-tight tabular-nums", mono && "mono")}>{value}</div>
            {sublabel && <div className="mt-1 text-xs text-muted-foreground">{sublabel}</div>}
            {series && (
                <div className="mt-3">
                    <Sparkline data={series} color={color} />
                </div>
            )}
        </GlassCard>
    );
}
