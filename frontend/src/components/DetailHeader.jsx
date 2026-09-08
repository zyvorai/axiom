import { cn } from "@/lib/utils";

// Flat detail-page header — see .detail-head in axiom-shell.css. Bigger
// sibling of PageHeader for resource-detail pages (Pod/Node/Workload/etc):
// a small kind eyebrow, a big mono resource name, and a meta row.
export default function DetailHeader({ eyebrow, name, meta, action, className }) {
    return (
        <header className={cn("detail-head", className)}>
            <div className="head-row">
                <div className="min-w-0">
                    {eyebrow && <div className="eyebrow">{eyebrow}</div>}
                    <h1 className="name" title={name}>
                        {name}
                    </h1>
                    {meta && <div className="meta">{meta}</div>}
                </div>
                {action && <div className="r">{action}</div>}
            </div>
        </header>
    );
}
