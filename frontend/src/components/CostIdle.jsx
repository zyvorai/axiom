import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import i18n, { formatCurrency } from "@/lib/i18n";
import { api } from "@/lib/api";
import { Skeleton } from "@/components/ui/skeleton";
import { COST_IDLE as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("errors:costPanels.generic");
const TONE = { high: "var(--green)", medium: "var(--orange)", low: "var(--ink-3)" };
const AGES = [7, 14, 30, 90];

/** Read-only idle/zombie resource findings. Nothing here deletes anything. */
export function CostIdle() {
    const { t } = useTranslation();
    const [minAge, setMinAge] = useState(7);
    const [data, setData] = useState(null);
    const [error, setError] = useState("");

    useEffect(() => {
        let alive = true;
        setData(null);
        api
            .get("/cost/idle", { params: { minAgeDays: minAge } })
            .then((r) => alive && (setData(r.data), setError("")))
            .catch((e) => alive && setError(errText(e)));
        return () => {
            alive = false;
        };
    }, [minAge]);

    if (error) return <div className="notice danger" role="alert" data-testid={T.error}>{error}</div>;
    if (!data) return <Skeleton className="h-32 rounded-2xl bg-muted" />;
    const findings = data.findings || [];
    const s = data.summary || {};

    return (
        <div className="card p-0" data-testid={T.root}>
            <div className="panel-h flex flex-wrap items-center justify-between gap-2">
                <span>{t("pages:costPanels.idle.title")}</span>
                <label className="flex items-center gap-2 text-[12.5px] text-ink-3">
                    {t("pages:costPanels.idle.minAge")}
                    <select className="select" data-testid={T.minAge} value={minAge} onChange={(e) => setMinAge(Number(e.target.value))}>
                        {AGES.map((n) => (
                            <option key={n} value={n}>{t("pages:costPanels.idle.minAgeOption", { n })}</option>
                        ))}
                    </select>
                </label>
            </div>
            <div className="border-b border-border px-4 py-2.5 text-[12.5px] text-ink-3">
                {t("pages:costPanels.idle.intro")} {t("pages:costPanels.idle.costNote")}
            </div>
            <div className="stats c3 !m-3">
                <div className="stat" data-testid={T.total}>
                    <b>{s.total || 0}</b>
                    <span>{t("pages:costPanels.idle.total")}</span>
                </div>
                <div className="stat" data-testid={T.savings}>
                    <b>{formatCurrency(s.monthlySavingsUsd || 0)}</b>
                    <span>{t("pages:costPanels.idle.savings")}</span>
                </div>
            </div>
            {findings.length === 0 ? (
                <div className="empty" data-testid={T.empty}>{t("pages:costPanels.idle.empty")}</div>
            ) : (
                <div className="max-h-[60vh] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:costPanels.idle.title")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("pages:costPanels.idle.colKind")}</th>
                                <th>{t("pages:costPanels.idle.colResource")}</th>
                                <th>{t("pages:costPanels.idle.colReason")}</th>
                                <th>{t("pages:costPanels.idle.colAge")}</th>
                                <th>{t("pages:costPanels.idle.colConfidence")}</th>
                                <th className="text-right">{t("pages:costPanels.idle.colCost")}</th>
                                <th>{t("pages:costPanels.idle.colAction")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {findings.map((f) => (
                                <tr key={`${f.kind}/${f.namespace}/${f.name}`} data-testid={T.row(f.kind, f.namespace, f.name)}>
                                    <td className="text-[12.5px]">{f.kind}</td>
                                    <td className="name">
                                        {f.name}
                                        <small>{f.namespace || t("pages:costPanels.idle.clusterScoped")}</small>
                                    </td>
                                    <td className="text-[12.5px]">{f.reason}</td>
                                    <td className="mono text-[12.5px]">{f.age}</td>
                                    <td>
                                        <span className="pill" data-testid={T.confidence(f.kind, f.namespace, f.name)} style={{ "--c": TONE[f.confidence] || TONE.low }}>
                                            <i aria-hidden="true" />
                                            {t(`pages:costPanels.confidence.${f.confidence}`, { defaultValue: f.confidence })}
                                        </span>
                                    </td>
                                    <td className="mono num text-right">{f.monthlyUsd != null ? formatCurrency(f.monthlyUsd) : "—"}</td>
                                    <td className="text-[12.5px] text-ink-3">{f.suggestion}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
            {data.truncated ? <div className="border-t border-border px-4 py-2.5 text-[12.5px] text-ink-3">{t("pages:costPanels.idle.truncated")}</div> : null}
        </div>
    );
}
