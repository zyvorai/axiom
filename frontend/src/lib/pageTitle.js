import { useEffect } from "react";

const BASE = "Axiom";

/**
 * Sets the browser tab/history title to "<page> · <section> · Axiom" (no-op for
 * non-string titles). `section` disambiguates same-named pages (e.g. a service
 * and a workload both called "checkout", or an Ops tab).
 */
export function usePageTitle(title, section) {
    useEffect(() => {
        if (typeof title !== "string" || !title.trim()) return undefined;
        const sec = typeof section === "string" && section.trim() ? ` · ${section.trim()}` : "";
        document.title = `${title}${sec} · ${BASE}`;
        return undefined;
    }, [title, section]);
}
