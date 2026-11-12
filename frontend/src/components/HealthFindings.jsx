import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { Lightbulb } from "lucide-react";
import { api } from "@/lib/api";
import { SeverityBadge } from "@/components/SeverityBadge";
import { objectHref, timelineHref } from "@/lib/troubleshoot";
import { ExplainWithAiButton } from "@/components/AiAssistantDrawer";
import { RunAgentButton } from "@/components/RunAgentButton";
import { AI } from "@/constants/testIds";

// The assistant explains a Pod (kind "pod") or any other object through its
// Warning context (kind "event", name = "Kind/name"). Findings need a namespace.
function explainTarget(f) {
    if (!f.namespace || typeof f.object !== "string" || !f.object.includes("/")) return null;
    const isPod = f.object.startsWith("Pod/");
    return {
        namespace: f.namespace,
        kind: isPod ? "pod" : "event",
        name: isPod ? f.object.slice(4) : f.object,
        reason: f.title,
        label: `${f.title}: ${f.namespace}/${f.object}`,
    };
}

/** Loads GET /diagnose for a namespace ("" or "all" = whole cluster) and refreshes on an interval. */
export function useClusterHealth(namespace = "", refreshMs = 30000) {
    const [report, setReport] = useState(null);
    const [error, setError] = useState("");
    const [loading, setLoading] = useState(true);

    const load = useCallback(() => {
        setLoading(true);
        const params = namespace && namespace !== "all" ? { namespace } : {};
        return api
            .get("/diagnose", { params })
            .then((r) => {
                setReport(r.data);
                setError("");
            })
            .catch((e) => setError(e?.response?.data?.error || e.message))
            .finally(() => setLoading(false));
    }, [namespace]);

    useEffect(() => {
        load();
        if (!refreshMs) return undefined;
        const t = setInterval(load, refreshMs);
        return () => clearInterval(t);
    }, [load, refreshMs]);

    return { report, error, loading, reload: load };
}

/** One finding: severity, what is wrong, where, and the suggested fix. */
export function HealthFinding({ finding, testId, compact = false }) {
    const { t } = useTranslation();
    const href = objectHref(finding.namespace, finding.object);
    return (
        <li data-testid={testId} className="border-b border-border px-4 py-3 last:border-none">
            <div className="flex flex-wrap items-center gap-2">
                <SeverityBadge severity={finding.severity} />
                <span className="text-sm font-medium">{finding.title}</span>
                <span className="mono text-xs text-muted-foreground">
                    {href ? (
                        <Link to={href} className="text-tone-link hover:underline">
                            {finding.namespace ? `${finding.namespace}/` : ""}
                            {finding.object}
                        </Link>
                    ) : (
                        finding.object
                    )}
                </span>
                {!compact && explainTarget(finding) && (
                    <>
                        <ExplainWithAiButton testId={AI.explainFinding} target={explainTarget(finding)} />
                        <RunAgentButton target={{ ...explainTarget(finding), goal: finding.title, reason: finding.message }} />
                    </>
                )}
                {!compact && (
                    <Link
                        to={timelineHref({ namespace: finding.namespace, object: finding.object })}
                        className="ml-auto text-xs text-tone-link hover:underline"
                    >
                        {t("pages:healthFindings.whatChanged")}
                    </Link>
                )}
            </div>
            <p className="mt-1 text-sm text-ink-2">{finding.message}</p>
            {!compact && finding.suggestion && (
                <p className="mt-1 flex items-start gap-1.5 text-sm text-ink-2">
                    <Lightbulb size={14} className="mt-0.5 flex-none text-ink-3" aria-hidden="true" />
                    <span>
                        <b className="font-medium text-ink">{t("pages:healthFindings.suggestedFix")}</b> {finding.suggestion}
                    </span>
                </p>
            )}
        </li>
    );
}
