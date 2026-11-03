import { Link } from "react-router-dom";
import { ArrowRight, CheckCircle2 } from "lucide-react";
import { HealthFinding, useClusterHealth } from "@/components/HealthFindings";
import { HealthStatusBadge } from "@/components/SeverityBadge";
import { HEALTH } from "@/constants/testIds";

const TOP = 5;

/** Overview section: the worst few findings from the deterministic cluster health check. */
export default function OverviewHealth() {
    const { report, error } = useClusterHealth("", 30000);
    if (error && !report) return null; // never block the overview on an optional panel

    const findings = report?.findings || [];
    const c = report?.counts || {};

    return (
        <section data-testid={HEALTH.panel} aria-labelledby="overview-health-h">
            <div className="section-h">
                <h2 id="overview-health-h">Health check</h2>
                <p>Automatic analysis of pods, services, volumes, ingresses, autoscalers and nodes.</p>
            </div>
            <div className="card media">
                {!report ? (
                    <div className="text-sm text-ink-3">Analyzing the cluster…</div>
                ) : (
                    <>
                        <div className="flex flex-wrap items-center gap-3 border-b border-border pb-3 text-sm">
                            <HealthStatusBadge status={report.status} />
                            <span className="text-ink-2">
                                {c.critical || 0} critical · {c.warning || 0} warning · {c.info || 0} info
                            </span>
                            <Link to="/health-check" data-testid={HEALTH.panelLink} className="ml-auto flex items-center gap-1 text-tone-link hover:underline">
                                Open health check <ArrowRight size={14} />
                            </Link>
                        </div>
                        {findings.length === 0 ? (
                            <div className="flex items-center gap-2 pt-3 text-sm text-ink-2">
                                <CheckCircle2 size={16} className="text-tone-green" aria-hidden="true" /> No problems found.
                            </div>
                        ) : (
                            <ul className="-mx-4 pt-1">
                                {findings.slice(0, TOP).map((f, i) => (
                                    <HealthFinding key={f.id} finding={f} compact testId={HEALTH.panelFinding(i)} />
                                ))}
                            </ul>
                        )}
                    </>
                )}
            </div>
        </section>
    );
}
