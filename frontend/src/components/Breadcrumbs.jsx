import { useLocation } from "react-router-dom";
import { ChevronRight } from "lucide-react";
import { useCluster } from "@/lib/cluster";
import { NAV_ITEMS } from "@/constants/navItems";

// zeus-os renders breadcrumbs inside the content area, above the page body,
// rather than in the header (WebShell.tsx) — this mirrors that placement.
export default function Breadcrumbs() {
    const { pathname } = useLocation();
    const { active } = useCluster();

    const current = [...NAV_ITEMS]
        .filter((item) => pathname === item.to || pathname.startsWith(`${item.to}/`))
        .sort((a, b) => b.to.length - a.to.length)[0];

    return (
        <nav aria-label="Breadcrumb" className="mb-4 flex items-center gap-1.5 text-xs text-muted-foreground">
            <span className="text-foreground/70">Clusters</span>
            <ChevronRight className="h-3 w-3 opacity-50" />
            <span className="text-foreground/90">{active?.label || active?.name || "—"}</span>
            {current && (
                <>
                    <ChevronRight className="h-3 w-3 opacity-50" />
                    <span className="text-foreground/90">{current.label}</span>
                </>
            )}
        </nav>
    );
}
