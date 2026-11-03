import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDistanceToNowStrict } from "date-fns";
import { useTranslation } from "react-i18next";
import { dateFnsLocale } from "@/lib/i18n";
import { AlertTriangle } from "lucide-react";
import { resourceHref } from "@/lib/resourceHref";

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true, locale: dateFnsLocale() });
    } catch {
        return iso;
    }
}

export default function Alerts() {
    const { t } = useTranslation();
    const [d, setD] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/alerts").then((r) => alive && setD(r.data));
        load();
        const timer = setInterval(load, 8000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    if (!d) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    const warnings = d.warnings || [];
    const rules = d.rules || [];

    return (
        <div className="page">
            <PageHeader
                title={t("pages:alerts.title")}
                description={t("pages:alerts.description")}
            />

            <GlassCard className="overflow-hidden p-0">
                <div className="panel-h">{t("pages:alerts.activeWarnings")}</div>
                <div className="max-h-[400px] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:alerts.activeWarnings")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("pages:alerts.columns.warning")}</th>
                                <th>{t("pages:alerts.columns.resource")}</th>
                                <th>{t("common:fields.count")}</th>
                                <th>{t("common:fields.time")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {warnings.map((w, i) => {
                                const href = resourceHref(w.namespace, w.object);
                                return (
                                    <tr key={i}>
                                        <td>
                                            <span className="flex items-start gap-2">
                                                <AlertTriangle size={14} className="text-tone-orange mt-0.5 shrink-0" />
                                                <span>
                                                    <span className="mono text-tone-link">{w.reason}</span> — {w.message}
                                                </span>
                                            </span>
                                        </td>
                                        <td className="mono">
                                            {href ? (
                                                <Link to={href} className="text-tone-link">
                                                    {w.namespace}/{w.object}
                                                </Link>
                                            ) : (
                                                `${w.namespace}/${w.object}`
                                            )}
                                        </td>
                                        <td className="num">×{w.count}</td>
                                        <td className="num">{relTime(w.time)}</td>
                                    </tr>
                                );
                            })}
                            {warnings.length === 0 && (
                                <tr>
                                    <td colSpan={4}>
                                        <div className="empty">{t("pages:alerts.noWarnings")}</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>

            <GlassCard className="overflow-hidden p-0">
                <div className="panel-h">{t("pages:alerts.ruleGroups")}</div>
                <div className="max-h-[300px] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:alerts.ruleGroups")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("common:fields.namespace")}</th>
                                <th>{t("pages:alerts.columns.groups")}</th>
                                <th>{t("common:fields.age")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {rules.map((r, i) => (
                                <tr key={i}>
                                    <td className="name mono">{r.name}</td>
                                    <td className="mono">{r.namespace}</td>
                                    <td className="num">{t("pages:alerts.groupsCount", { n: r.groups })}</td>
                                    <td className="num">{r.age}</td>
                                </tr>
                            ))}
                            {rules.length === 0 && (
                                <tr>
                                    <td colSpan={4}>
                                        <div className="empty">{t("pages:alerts.noRules")}</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>
        </div>
    );
}
