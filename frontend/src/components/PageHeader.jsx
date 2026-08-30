import { cn } from "@/lib/utils";

// The apple.com-style hero header: a small tracked-caps eyebrow label
// above an oversized, tight-tracking title. Overview.jsx originated this
// pattern (wrapped in its own hero GlassCard); this generalizes the same
// eyebrow/title styling into a shared primitive for every other page's
// plain <h1>, deliberately bigger/bolder than the text-3xl baseline those
// pages used before.
export default function PageHeader({ eyebrow, title, description, action, className }) {
    return (
        <div className={cn("flex flex-wrap items-end justify-between gap-4", className)}>
            <div className="min-w-0">
                {eyebrow && (
                    <div className="text-[11px] uppercase tracking-[0.2em] text-i17-cosmic/90">{eyebrow}</div>
                )}
                <h1 className={cn("text-4xl font-semibold tracking-[-0.02em] sm:text-5xl", eyebrow && "mt-1")}>
                    {title}
                </h1>
                {description && <p className="mt-2 text-sm text-muted-foreground">{description}</p>}
            </div>
            {action && <div className="flex shrink-0 items-center gap-2">{action}</div>}
        </div>
    );
}
