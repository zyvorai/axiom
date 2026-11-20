import { useEffect, useState } from "react";
import { api } from "@/lib/api";

/** Live active warnings from /alerts (the same feed the Alerts page shows), polled. */
export function useAlertWarnings(intervalMs = 6000) {
    const [warnings, setWarnings] = useState([]);

    useEffect(() => {
        let alive = true;
        const load = () =>
            api
                .get("/alerts")
                .then((r) => {
                    if (alive) setWarnings(r.data.warnings || []);
                })
                .catch(() => {});
        load();
        const t = setInterval(load, intervalMs);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, [intervalMs]);

    return warnings;
}

/** Live unresolved-alert count, polled the same way Overview.jsx always has. */
export function useAlertsCount(intervalMs = 6000) {
    const [count, setCount] = useState(0);

    useEffect(() => {
        let alive = true;
        const load = () =>
            api
                .get("/alerts")
                .then((r) => {
                    if (alive) setCount((r.data.warnings || []).length);
                })
                .catch(() => {});
        load();
        const t = setInterval(load, intervalMs);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, [intervalMs]);

    return count;
}
