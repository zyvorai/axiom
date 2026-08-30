import { cn } from "@/lib/utils";

// The apple.com-style hero header: a small tracked-caps eyebrow label
// above an oversized, bold, tight-tracking title, closed off with a
// hairline rule beneath it — the same eyebrow/rule/huge-title shape
// apple.com uses to open a section, scaled down to fit inside the app
// shell instead of a full-bleed marketing page.
export default function PageHeader({ eyebrow, title, description, action, className }) {
    return (
        <div className={cn("flex flex-wrap items-end justify-between gap-6 border-b border-border pb-8", className)}>
            <div className="min-w-0">
                {eyebrow && (
                    <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-primary">{eyebrow}</div>
                )}
                <h1 className={cn("text-display font-bold text-foreground", eyebrow && "mt-2")}>
                    {title}
                </h1>
                {description && <p className="mt-3 max-w-2xl text-base text-muted-foreground">{description}</p>}
            </div>
            {action && <div className="flex shrink-0 items-center gap-2">{action}</div>}
        </div>
    );
}
