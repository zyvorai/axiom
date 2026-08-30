import { cn } from "@/lib/utils";

// Scaled-up sibling of PageHeader for resource-detail pages (Pod/Node/Workload):
// keeps the existing eyebrow-kind + mono-name identifier pattern, just bigger
// and closed off with the same hairline rule, to match the bolder hero
// treatment every list page now has.
export default function DetailHeader({ eyebrow, name, meta, action, className }) {
    return (
        <div className={cn("flex flex-wrap items-start justify-between gap-6 border-b border-border pb-8", className)}>
            <div className="min-w-0">
                <div className="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">{eyebrow}</div>
                <h1 className="mono truncate text-title font-bold tracking-tight text-foreground" title={name}>
                    {name}
                </h1>
                {meta && <div className="mt-3 flex flex-wrap items-center gap-3 text-sm text-muted-foreground">{meta}</div>}
            </div>
            {action && <div className="flex shrink-0 items-center gap-4">{action}</div>}
        </div>
    );
}
