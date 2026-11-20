import { useEffect, useState } from "react";
import { CheckCircle2, RefreshCw, ShieldAlert } from "lucide-react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { HealthStatusBadge } from "@/components/SeverityBadge";
import { HealthFinding, useClusterHealth } from "@/components/HealthFindings";
import { Skeleton } from "@/components/ui/skeleton";
import { HEALTH } from "@/constants/testIds";
import { formatDistanceToNowStrict } from "date-fns";
import { useTranslation } from "react-i18next";
import { dateFnsLocale } from "@/lib/i18n";
import { toast } from "sonner";

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true, locale: dateFnsLocale() });
    } catch {
        return String(iso ?? "");
    }
}

const SEVERITIES = [
    { key: "critical", labelKey: "pages:healthCheck.severities.critical" },
    { key: "warning", labelKey: "pages:healthCheck.severities.warning" },
    { key: "info", labelKey: "pages:healthCheck.severities.info" },
];

/** Deterministic cluster health check: rule-based analyzers with a suggested fix per finding. */
export default function HealthCheck() {
    const { t } = useTranslation();
    const [namespace, setNamespace] = useState("");
    const [namespaces, setNamespaces] = useState([]);
    const [sev, setSev] = useState("");
    const [upgrade, setUpgrade] = useState(null);
    const [upgradeBusy, setUpgradeBusy] = useState(false);
    const { report, error, loading, reload } = useClusterHealth(namespace, 30000);

    const loadUpgrade = () => {
        api.get("/upgrade-risk").then((r) => setUpgrade(r.data)).catch(() => setUpgrade(null));
    };

    useEffect(() => {
        let alive = true;
        api
            .get("/namespaces")
            .then((r) => alive && setNamespaces((r.data || []).map((n) => n.name)))
            .catch(() => {});
        loadUpgrade();
        return () => {
            alive = false;
        };
    }, []);

    const runUpgradeAgent = async () => {
        setUpgradeBusy(true);
        try {
            await api.post("/upgrade-risk/agent-run");
            toast.success(t("toasts:ops.agents.created"));
        } catch (e) {
            toast.error(t("toasts:ops.agents.createFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setUpgradeBusy(false);
        }
    };

    const all = report?.findings || [];
    const shown = sev ? all.filter((f) => f.severity === sev) : all;

    return (
        <div data-testid={HEALTH.root} className="page">
            <PageHeader
                title={t("pages:healthCheck.title")}
                description={t("pages:healthCheck.description")}
                action={
                    <button type="button" data-testid={HEALTH.refresh} className="btn btn-line" onClick={reload} disabled={loading}>
                        <RefreshCw size={15} /> {loading ? t("pages:healthCheck.analyzing") : t("pages:healthCheck.runAgain")}
                    </button>
                }
                tools={
                    <select
                        data-testid={HEALTH.namespace}
                        aria-label={t("common:fields.namespace")}
                        className="select"
                        value={namespace}
                        onChange={(e) => setNamespace(e.target.value)}
                    >
                        <option value="">{t("pages:healthCheck.allNamespaces")}</option>
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

            {upgrade && (
                <GlassCard className="space-y-3 p-4">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                        <div className="flex items-center gap-2 text-sm font-medium">
                            <ShieldAlert size={15} /> {t("pages:healthCheck.upgradeRisk.title")}
                            <span className="pill" style={{ "--c": upgrade.status === "ready" ? "var(--green)" : upgrade.status === "blocked" ? "var(--red)" : "var(--amber)" }}>
                                <i aria-hidden="true" />
                                {upgrade.status}
                            </span>
                        </div>
                        <div className="flex gap-2">
                            <button type="button" className="btn btn-line btn-sm" onClick={loadUpgrade}>{t("pages:healthCheck.runAgain")}</button>
                            <button type="button" className="btn btn-line btn-sm" disabled={upgradeBusy} onClick={runUpgradeAgent}>
                                {upgradeBusy ? t("common:actions.saving") : t("pages:ops.agents.runAgent")}
                            </button>
                        </div>
                    </div>
                    <p className="text-[13px] text-ink-3">{upgrade.summary}</p>
                    <div className="text-[12px] text-ink-3 mono">
                        {upgrade.cluster?.kubernetesVersion || "—"} · {upgrade.cluster?.nodesReady}/{upgrade.cluster?.nodeCount} nodes ready
                    </div>
                    {(upgrade.findings || []).length > 0 && (
                        <ul className="space-y-2">
                            {upgrade.findings.slice(0, 8).map((f, i) => (
                                <li key={`${f.code}-${i}`} className="rounded-xl border border-border px-3 py-2 text-[13px]">
                                    <span className="mono text-tone-link">{f.severity}</span> · {f.title}
                                    <div className="text-[12px] text-ink-3">{f.detail}</div>
                                    {f.hint ? <div className="text-[12px] text-ink-3">{f.hint}</div> : null}
                                </li>
                            ))}
                        </ul>
                    )}
                </GlassCard>
            )}

            {!report && !error && <Skeleton className="h-64 rounded-2xl bg-muted" />}

            {report && (
                <>
                    <div className="stats">
                        <div className="stat media" data-testid={HEALTH.status}>
                            <b className="!text-[20px]">
                                <HealthStatusBadge status={report.status} />
                            </b>
                            <span>{t("pages:healthCheck.overall", { scope: namespace || t("pages:healthCheck.allNamespacesLower") })}</span>
                        </div>
                        {SEVERITIES.map((s) => (
                            <button
                                key={s.key}
                                type="button"
                                data-testid={HEALTH.count(s.key)}
                                aria-pressed={sev === s.key}
                                aria-label={t("pages:healthCheck.countAria", { label: t(s.labelKey), n: report.counts?.[s.key] || 0 })}
                                onClick={() => setSev(sev === s.key ? "" : s.key)}
                                className={`stat media text-left ${sev === s.key ? "ring-1 ring-primary" : ""}`}
                            >
                                <b>{report.counts?.[s.key] || 0}</b>
                                <span>{t(s.labelKey)}</span>
                            </button>
                        ))}
                    </div>

                    {report.errors?.length > 0 && (
                        <div className="notice" role="status" data-testid={HEALTH.errors}>
                            {t("pages:healthCheck.analyzerErrors", { errors: report.errors.join("; ") })}
                        </div>
                    )}

                    <GlassCard className="overflow-hidden p-0">
                        {shown.length === 0 ? (
                            <div className="empty" data-testid={HEALTH.clean}>
                                <CheckCircle2 size={20} className="mx-auto mb-2 text-tone-green" aria-hidden="true" />
                                {all.length === 0 ? t("pages:healthCheck.noProblems") : t("pages:healthCheck.noFindingsAtSeverity")}
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
                        {t("pages:healthCheck.checkedBy", { n: report.analyzers?.length || 0, list: (report.analyzers || []).join(", "), time: relTime(report.generatedAt) })}
                    </p>
                </>
            )}
        </div>
    );
}
