import { cn } from "@/lib/utils";
import Sparkline from "@/components/Sparkline";

// Boxed metric tile — the same `.stat` chrome as the Overview (blue top rule,
// big tight number, label below). Put tiles in a `.stats` grid. `size="compact"`
// uses a smaller number for dense detail-page headers.
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
    className,
    ...rest
}) {
    return (
        <div className={cn("stat", size === "compact" && "compact", className)} {...rest}>
            <b className={mono ? "mono" : undefined}>{value}</b>
            <span>
                {Icon && <Icon className={cn("h-3.5 w-3.5 text-tone-link", iconClassName)} aria-hidden="true" />}
                {label}
            </span>
            {sublabel && <em>{sublabel}</em>}
            {series && (
                <div className="mt-3">
                    <Sparkline data={series} color={color} />
                </div>
            )}
        </div>
    );
}
