import { cn } from "@/lib/utils";

// Scaled-up sibling of PageHeader for resource-detail pages (Pod/Node/Workload):
// keeps the existing eyebrow-kind + mono-name identifier pattern, just bigger,
// to match the bolder hero treatment every list page now has.
export default function DetailHeader({ eyebrow, name, meta, action, className }) {
    return (
        <div className={cn("flex flex-wrap items-start justify-between gap-4", className)}>
            <div className="min-w-0">
                <div className="text-xs uppercase tracking-widest text-muted-foreground">{eyebrow}</div>
                <h1 className="mono truncate text-[clamp(1.5rem,2vw+1rem,2.25rem)] font-semibold tracking-[-0.035em]" title={name}>
                    {name}
                </h1>
                {meta && <div className="mt-2 flex flex-wrap items-center gap-3 text-sm text-muted-foreground">{meta}</div>}
            </div>
            {action && <div className="flex shrink-0 items-center gap-4">{action}</div>}
        </div>
    );
}
