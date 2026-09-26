import { cn } from "@/lib/utils";
import Sparkline from "@/components/Sparkline";

// Netra metric block: 11px uppercase label, big tight number. Sits inside a
// .card/.stat — no nested card chrome of its own.
export default function FeatureTile({
    icon: Icon,
    iconClassName,
    label,
    value,
    sublabel,
    series,
    color = "var(--apple-blue)",
    mono = false,
    size = "default",
    nested = false,
    className,
    ...rest
}) {
    if (size === "compact") {
        return (
            <div className={cn("py-3", className)} {...rest}>
                <div className="flex items-center gap-1 text-[11px] tracking-[-0.01em] text-muted-foreground">
                    {Icon && <Icon className={cn("h-3 w-3 text-tone-link", iconClassName)} />} {label}
                </div>
                <div className={cn("mt-1 text-xl tracking-[-0.02em] text-foreground", mono && "mono")}>{value}</div>
                {sublabel && <div className="mt-0.5 text-xs text-muted-foreground">{sublabel}</div>}
            </div>
        );
    }

    return (
        <div className={cn("py-4", className)} {...rest}>
            <div className="flex items-center justify-between">
                <div className="text-[11px] font-semibold tracking-[0.08em] uppercase text-muted-foreground">{label}</div>
                {Icon && <Icon className={cn("h-3.5 w-3.5 text-tone-link", iconClassName)} />}
            </div>
            <div className={cn("mt-2 text-[clamp(28px,2.6vw,40px)] font-semibold leading-none tracking-[-0.03em] tabular-nums", mono && "mono")}>{value}</div>
            {sublabel && <div className="mt-2 text-xs text-muted-foreground">{sublabel}</div>}
            {series && (
                <div className="mt-3">
                    <Sparkline data={series} color={color} />
                </div>
            )}
        </div>
    );
}
