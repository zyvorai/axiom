import { cn } from "@/lib/utils";
import Sparkline from "@/components/Sparkline";

// Flat apple.com-style metric block — no nested card chrome.
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
    nested = false,
    className,
    ...rest
}) {
    if (size === "compact") {
        return (
            <div className={cn("py-3", className)} {...rest}>
                <div className="flex items-center gap-1 text-[11px] tracking-[-0.01em] text-muted-foreground">
                    {Icon && <Icon className={cn("h-3 w-3 text-i17-sky", iconClassName)} />} {label}
                </div>
                <div className={cn("mt-1 text-xl tracking-[-0.02em] text-foreground", mono && "mono")}>{value}</div>
                {sublabel && <div className="mt-0.5 text-xs text-muted-foreground">{sublabel}</div>}
            </div>
        );
    }

    return (
        <div className={cn("py-4", className)} {...rest}>
            <div className="flex items-center justify-between">
                <div className="text-[11px] font-normal tracking-[0.12em] uppercase text-muted-foreground">{label}</div>
                {Icon && <Icon className={cn("h-3.5 w-3.5 text-i17-sky", iconClassName)} />}
            </div>
            <div className={cn("mt-2 text-3xl font-semibold tracking-[-0.025em] tabular-nums", mono && "mono")}>{value}</div>
            {sublabel && <div className="mt-1 text-xs text-muted-foreground">{sublabel}</div>}
            {series && (
                <div className="mt-3">
                    <Sparkline data={series} color={color} />
                </div>
            )}
        </div>
    );
}
