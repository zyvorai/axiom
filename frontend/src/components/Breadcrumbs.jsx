import { Link, useLocation } from "react-router-dom";
import { ChevronRight } from "lucide-react";
import { useCluster } from "@/lib/cluster";
import { NAV_ITEMS } from "@/constants/navItems";

const DETAIL_ROOTS = new Set([
    "pods",
    "workloads",
    "services",
    "configmaps",
    "secrets",
    "vms",
    "jobs",
    "cronjobs",
    "namespaces",
    "nodes",
]);

function detailCrumb(pathname) {
    const parts = pathname.split("/").filter(Boolean);
    if (!parts.length || !DETAIL_ROOTS.has(parts[0])) return null;
    if (parts[0] === "namespaces" && parts[1]) {
        return { listTo: "/namespaces", listLabel: "Namespaces", name: decodeURIComponent(parts[1]) };
    }
    if (parts[0] === "nodes" && parts[1]) {
        return { listTo: "/nodes", listLabel: "Nodes", name: decodeURIComponent(parts[1]) };
    }
    if (parts[0] === "workloads" && parts.length >= 4) {
        return {
            listTo: "/workloads",
            listLabel: "Workloads",
            name: `${decodeURIComponent(parts[1])}/${decodeURIComponent(parts[2])}/${decodeURIComponent(parts[3])}`,
        };
    }
    if (parts.length >= 3) {
        const listTo = parts[0] === "cronjobs" ? "/jobs" : `/${parts[0]}`;
        return {
            listTo,
            listLabel: null,
            name: `${decodeURIComponent(parts[1])}/${decodeURIComponent(parts[2])}`,
        };
    }
    return null;
}

export default function Breadcrumbs() {
    const { pathname } = useLocation();
    const { active } = useCluster();

    const parts = pathname.split("/").filter(Boolean);
    const navPath = parts[0] === "cronjobs" ? "/jobs" : pathname;

    const current = [...NAV_ITEMS]
        .filter((item) => navPath === item.to || navPath.startsWith(`${item.to}/`))
        .sort((a, b) => b.to.length - a.to.length)[0];

    const detail = detailCrumb(pathname);

    return (
        <nav aria-label="Breadcrumb" className="mb-4 flex items-center gap-1.5 text-xs text-muted-foreground">
            <span className="text-foreground/70">Clusters</span>
            <ChevronRight className="h-3 w-3 opacity-50" />
            <span className="text-foreground/90">{active?.label || active?.name || "—"}</span>
            {current && (
                <>
                    <ChevronRight className="h-3 w-3 opacity-50" />
                    {detail ? (
                        <Link to={detail.listTo || current.to} className="text-foreground/90 hover:text-primary hover:underline">
                            {current.label}
                        </Link>
                    ) : (
                        <span className="text-foreground/90">{current.label}</span>
                    )}
                </>
            )}
            {detail && (
                <>
                    <ChevronRight className="h-3 w-3 opacity-50" />
                    <span className="mono text-foreground/90">{detail.name}</span>
                </>
            )}
        </nav>
    );
}
