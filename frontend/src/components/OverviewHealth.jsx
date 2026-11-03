import { Link } from "react-router-dom";
import { ArrowRight, CheckCircle2 } from "lucide-react";
import { HealthFinding, useClusterHealth } from "@/components/HealthFindings";
import { HealthStatusBadge } from "@/components/SeverityBadge";
import { HEALTH } from "@/constants/testIds";
import { useTranslation } from "react-i18next";

const TOP = 5;

/** Overview section: the worst few findings from the deterministic cluster health check. */
export default function OverviewHealth() {
    const { t } = useTranslation();
    const { report, error } = useClusterHealth("", 30000);
    if (error && !report) return null; // never block the overview on an optional panel

    const findings = report?.findings || [];
    const c = report?.counts || {};

    return (
        <section data-testid={HEALTH.panel} aria-labelledby="overview-health-h">
            <div className="section-h">
                <h2 id="overview-health-h">{t("pages:overviewHealth.title")}</h2>
                <p>{t("pages:overviewHealth.description")}</p>
            </div>
            <div className="card media">
                {!report ? (
                    <div className="text-sm text-ink-3">{t("pages:overviewHealth.analyzing")}</div>
                ) : (
                    <>
                        <div className="flex flex-wrap items-center gap-3 border-b border-border pb-3 text-sm">
                            <HealthStatusBadge status={report.status} />
                            <span className="text-ink-2">
                                {t("pages:overviewHealth.counts", { critical: c.critical || 0, warning: c.warning || 0, info: c.info || 0 })}
                            </span>
                            <Link to="/health-check" data-testid={HEALTH.panelLink} className="ml-auto flex items-center gap-1 text-tone-link hover:underline">
                                {t("pages:overviewHealth.open")} <ArrowRight size={14} />
                            </Link>
                        </div>
                        {findings.length === 0 ? (
                            <div className="flex items-center gap-2 pt-3 text-sm text-ink-2">
                                <CheckCircle2 size={16} className="text-tone-green" aria-hidden="true" /> {t("pages:overviewHealth.noProblems")}
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
