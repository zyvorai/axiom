import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useAlertsCount } from "@/lib/alerts";

/** Live sidebar badge counts (workloads/pods/alerts), sourced from the same
 * /overview summary and /alerts poll Overview.jsx already uses — no new
 * per-item endpoints. */
export function useNavCounts(intervalMs = 10000) {
    const [summary, setSummary] = useState(null);
    const alerts = useAlertsCount(intervalMs);

    useEffect(() => {
        let alive = true;
        const load = () =>
            api
                .get("/overview")
                .then((r) => {
                    if (alive) setSummary(r.data);
                })
                .catch(() => {});
        load();
        const t = setInterval(load, intervalMs);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, [intervalMs]);

    return {
        workloads: summary?.workloads,
        pods: summary?.pods,
        alerts,
    };
}
