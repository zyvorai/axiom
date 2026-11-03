import { useEffect, useState } from "react";
import { CheckCircle2, RefreshCw } from "lucide-react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { HealthStatusBadge } from "@/components/SeverityBadge";
import { HealthFinding, useClusterHealth } from "@/components/HealthFindings";
import { Skeleton } from "@/components/ui/skeleton";
import { HEALTH } from "@/constants/testIds";
import { relTime } from "@/lib/troubleshoot";

const SEVERITIES = [
    { key: "critical", label: "Critical" },
    { key: "warning", label: "Warning" },
    { key: "info", label: "Info" },
];

/** Deterministic cluster health check: rule-based analyzers with a suggested fix per finding. */
export default function HealthCheck() {
    const [namespace, setNamespace] = useState("");
    const [namespaces, setNamespaces] = useState([]);
    const [sev, setSev] = useState("");
    const { report, error, loading, reload } = useClusterHealth(namespace, 30000);

    useEffect(() => {
        let alive = true;
        api
            .get("/namespaces")
            .then((r) => alive && setNamespaces((r.data || []).map((n) => n.name)))
            .catch(() => {});
        return () => {
            alive = false;
        };
    }, []);

    const all = report?.findings || [];
    const shown = sev ? all.filter((f) => f.severity === sev) : all;

    return (
        <div data-testid={HEALTH.root} className="page">
            <PageHeader
                title="Health check"
                description="Rule-based analysis of pods, services, volumes, ingresses, autoscalers and nodes, with a suggested fix for every finding. No data leaves the cluster."
                action={
                    <button type="button" data-testid={HEALTH.refresh} className="btn btn-line" onClick={reload} disabled={loading}>
                        <RefreshCw size={15} /> {loading ? "Analyzing…" : "Run again"}
                    </button>
                }
                tools={
                    <select
                        data-testid={HEALTH.namespace}
                        aria-label="Namespace"
                        className="select"
                        value={namespace}
                        onChange={(e) => setNamespace(e.target.value)}
                    >
                        <option value="">All namespaces</option>
                        {namespaces.map((n) => (
                            <option key={n} value={n}>
                                {n}
                            </option>
                        ))}
                    </select>
                }
            />

            {error && (
                <div className="notice danger" role="alert">
                    {error}
                </div>
            )}

            {!report && !error && <Skeleton className="h-64 rounded-2xl bg-muted" />}

            {report && (
                <>
                    <div className="stats">
                        <div className="stat media" data-testid={HEALTH.status}>
                            <b className="!text-[20px]">
                                <HealthStatusBadge status={report.status} />
                            </b>
                            <span>Overall · {namespace || "all namespaces"}</span>
                        </div>
                        {SEVERITIES.map((s) => (
                            <button
                                key={s.key}
                                type="button"
                                data-testid={HEALTH.count(s.key)}
                                aria-pressed={sev === s.key}
                                aria-label={`${s.label}: ${report.counts?.[s.key] || 0} findings. Toggle filter.`}
                                onClick={() => setSev(sev === s.key ? "" : s.key)}
                                className={`stat media text-left ${sev === s.key ? "ring-1 ring-primary" : ""}`}
                            >
                                <b>{report.counts?.[s.key] || 0}</b>
                                <span>{s.label}</span>
                            </button>
                        ))}
                    </div>

                    {report.errors?.length > 0 && (
                        <div className="notice" role="status" data-testid={HEALTH.errors}>
                            Some analyzers could not run, so this report may be incomplete: {report.errors.join("; ")}
                        </div>
                    )}

                    <GlassCard className="overflow-hidden p-0">
                        {shown.length === 0 ? (
                            <div className="empty" data-testid={HEALTH.clean}>
                                <CheckCircle2 size={20} className="mx-auto mb-2 text-tone-green" aria-hidden="true" />
                                {all.length === 0 ? "No problems found." : "No findings at this severity."}
                            </div>
                        ) : (
                            <ul>
                                {shown.map((f, i) => (
                                    <HealthFinding key={f.id} finding={f} testId={HEALTH.finding(i)} />
                                ))}
                            </ul>
                        )}
                    </GlassCard>
                    <p className="text-xs text-ink-3">
                        Checked by {report.analyzers?.length || 0} analyzers ({(report.analyzers || []).join(", ")}) · {relTime(report.generatedAt)}
                    </p>
                </>
            )}
        </div>
    );
}
