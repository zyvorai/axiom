import { cn } from "@/lib/utils";
import { usePageTitle } from "@/lib/pageTitle";

// Flat detail-page header — see .detail-head in axiom-shell.css. Bigger
// sibling of PageHeader for resource-detail pages (Pod/Node/Workload/etc):
// a small kind eyebrow, a big mono resource name, and a meta row.
export default function DetailHeader({ eyebrow, name, meta, action, className }) {
    // eyebrow is e.g. "NODE · SERVER" — use its first part ("Node") as the kind
    const kind = typeof eyebrow === "string" ? eyebrow.split("·")[0].trim().toLowerCase() : "";
    usePageTitle(name, kind && kind[0].toUpperCase() + kind.slice(1));
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
