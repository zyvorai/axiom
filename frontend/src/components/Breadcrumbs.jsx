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

// Apple.com top-bar breadcrumb trail (.crumbs, see axiom-shell.css) — self-hides
// on the Overview home so the first viewport reads as a marketing band, not an
// ops trail (matches AppShell's old showBreadcrumbs condition).
export default function Breadcrumbs() {
    const { pathname } = useLocation();
    const { active } = useCluster();

    if (pathname === "/overview" || pathname === "/") return null;

    const parts = pathname.split("/").filter(Boolean);
    const navPath = parts[0] === "cronjobs" ? "/jobs" : pathname;

    const current = [...NAV_ITEMS]
        .filter((item) => navPath === item.to || navPath.startsWith(`${item.to}/`))
        .sort((a, b) => b.to.length - a.to.length)[0];

    const detail = detailCrumb(pathname);

    return (
        <div className="crumbs">
            Clusters
            <ChevronRight size={13} />
            {active?.label || active?.name || "—"}
            {current && (
                <>
                    <ChevronRight size={13} />
                    {detail ? (
                        <Link to={detail.listTo || current.to} className="hover:underline">
                            <b>{current.label}</b>
                        </Link>
                    ) : (
                        <b>{current.label}</b>
                    )}
                </>
            )}
            {detail && (
                <>
                    <ChevronRight size={13} />
                    <b className="mono">{detail.name}</b>
                </>
            )}
        </div>
    );
}
