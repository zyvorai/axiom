import { useEffect } from "react";

const BASE = "Axiom";

/**
 * Sets the browser tab/history title to "<page> · <section> · Axiom" (no-op for
 * non-string titles). `section` disambiguates same-named pages (e.g. a service
 * and a workload both called "checkout", or an Ops tab).
 */
/** Fallback title for a route ("Jobs & CronJobs · Axiom"); pages refine it with usePageTitle. */
export function defaultTitle(pathname, navItems) {
    const match = [...navItems]
        .filter((n) => pathname === n.to || pathname.startsWith(n.to + "/"))
        .sort((a, b) => b.to.length - a.to.length)[0];
    return match ? `${match.label} · ${BASE}` : BASE;
}

export function usePageTitle(title, section) {
    useEffect(() => {
        if (typeof title !== "string" || !title.trim()) return undefined;
        const sec = typeof section === "string" && section.trim() ? ` · ${section.trim()}` : "";
        document.title = `${title}${sec} · ${BASE}`;
        return undefined;
    }, [title, section]);
}
