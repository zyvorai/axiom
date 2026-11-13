import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { RECENT_CHANGES } from "@/constants/testIds";
import { relTime } from "@/lib/troubleshoot";
import { useTranslation } from "react-i18next";
import { formatDate } from "@/lib/i18n";
import { cn } from "@/lib/utils";

const SOURCE_TONES = {
    gitops: "var(--purple)",
    helm: "var(--blue)",
    rollout: "var(--green)",
    config: "var(--orange)",
    node: "var(--red)",
    hpa: "var(--teal)",
    audit: "var(--ink-3)",
};

/**
 * "What changed before this?": ranked recent changes (GitOps, Helm, rollouts,
 * config, nodes, HPA, audit) in a lookback window around a namespace/workload.
 * `at` is an optional RFC 3339 reference time (defaults to now).
 */
export default function RecentChanges({ namespace, workload, at, className }) {
    const { t } = useTranslation();
    const [windowSel, setWindowSel] = useState("60m");
    const [report, setReport] = useState(null);
    const [error, setError] = useState("");

    useEffect(() => {
        let alive = true;
        const load = () => {
            const params = { window: windowSel };
            if (namespace) params.namespace = namespace;
            if (workload) params.workload = workload;
            if (at) params.at = at;
            return api
                .get("/changes", { params })
                .then((r) => {
                    if (!alive) return;
                    setReport(r.data || { changes: [] });
                    setError("");
                })
                .catch((e) => alive && setError(e?.response?.data?.error || e.message));
        };
        load();
        const timer = setInterval(load, 15000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, [namespace, workload, at, windowSel]);

    const changes = report?.changes || [];
    return (
        <GlassCard className={cn("overflow-hidden p-0", className)} data-testid={RECENT_CHANGES.root}>
            <div className="flex items-center justify-between border-b border-border px-4 py-3">
                <div className="text-sm font-medium">{t("pages:recentChanges.title")}</div>
                <select
                    data-testid={RECENT_CHANGES.window}
                    aria-label={t("pages:recentChanges.window")}
                    className="select"
                    value={windowSel}
                    onChange={(e) => setWindowSel(e.target.value)}
                >
                    <option value="15m">{t("pages:recentChanges.windows.m15")}</option>
                    <option value="60m">{t("pages:recentChanges.windows.m60")}</option>
                    <option value="6h">{t("pages:recentChanges.windows.h6")}</option>
                    <option value="24h">{t("pages:recentChanges.windows.h24")}</option>
                </select>
            </div>
            {error && (
                <div className="notice danger m-3" role="alert">
                    {error}
                </div>
            )}
            {!report && !error && <div className="px-4 py-6 text-center text-sm text-muted-foreground">{t("common:states.loading")}</div>}
            {report && changes.length === 0 && (
                <div className="px-4 py-6 text-center text-sm text-muted-foreground" data-testid={RECENT_CHANGES.empty}>
                    {t("pages:recentChanges.empty")}
                </div>
            )}
            {changes.map((c, i) => (
                <div
                    key={`${c.time}-${i}`}
                    data-testid={RECENT_CHANGES.row(i)}
                    className="grid grid-cols-12 items-center gap-3 border-b border-border px-4 py-3 last:border-none"
                >
                    <div className="col-span-2 mono text-xs text-muted-foreground" title={formatDate(c.time)}>
                        {relTime(c.time)}
                    </div>
                    <div className="col-span-2">
                        <span className="pill" style={{ "--c": SOURCE_TONES[c.source] || "var(--ink-3)" }}>
                            <i aria-hidden="true" />
                            {t(`pages:recentChanges.sources.${c.source}`, { defaultValue: c.source })}
                        </span>
                    </div>
                    <div className="col-span-7 text-sm text-foreground">
                        {c.summary}
                        {c.actor && <span className="text-xs text-muted-foreground"> · {t("pages:workloadTimeline.by", { actor: c.actor })}</span>}
                    </div>
                    <div className="col-span-1 text-right mono text-xs text-muted-foreground" title={t("pages:recentChanges.relevance")} data-testid={RECENT_CHANGES.score(i)}>
                        {c.score}
                    </div>
                </div>
            ))}
        </GlassCard>
    );
}
