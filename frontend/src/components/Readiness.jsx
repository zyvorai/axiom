import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import i18n from "@/lib/i18n";
import { api } from "@/lib/api";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { READINESS as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("errors:costPanels.generic");
const TONE = {
    protected: "var(--green)",
    stale: "var(--orange)",
    failing: "var(--red)",
    unprotected: "var(--red)",
    high: "var(--red)",
    medium: "var(--orange)",
    low: "var(--ink-3)",
    removed: "var(--red)",
    deprecated: "var(--orange)",
};
const AGES = [1, 3, 7, 14, 30];

function Pill({ tone, label, testId }) {
    return (
        <span className="pill" data-testid={testId} style={{ "--c": TONE[tone] || TONE.low }}>
            <i aria-hidden="true" />
            {label}
        </span>
    );
}

/** Read-only: which namespaces with data have no recent completed Velero backup. */
function BackupCoverage() {
    const { t } = useTranslation();
    const [maxAge, setMaxAge] = useState(7);
    const [data, setData] = useState(null);
    const [error, setError] = useState("");

    useEffect(() => {
        let alive = true;
        setData(null);
        api
            .get("/backups/coverage", { params: { maxAgeDays: maxAge } })
            .then((r) => alive && (setData(r.data), setError("")))
            .catch((e) => alive && setError(errText(e)));
        return () => {
            alive = false;
        };
    }, [maxAge]);

    if (error) return <div className="notice danger" role="alert" data-testid={T.backupError}>{error}</div>;
    if (!data) return <Skeleton className="h-32 rounded-2xl bg-muted" />;
    const items = data.namespaces || [];
    const by = data.summary?.byStatus || {};

    return (
        <div className="card p-0" data-testid={T.backupRoot}>
            <div className="panel-h flex flex-wrap items-center justify-between gap-2">
                <span>{t("pages:readiness.backup.title")}</span>
                <label className="flex items-center gap-2 text-[12.5px] text-ink-3">
                    {t("pages:readiness.backup.maxAge")}
                    <select className="select" data-testid={T.backupMaxAge} value={maxAge} onChange={(e) => setMaxAge(Number(e.target.value))}>
                        {AGES.map((n) => (
                            <option key={n} value={n}>{t("pages:readiness.backup.maxAgeOption", { n })}</option>
                        ))}
                    </select>
                </label>
            </div>
            <div className="border-b border-border px-4 py-2.5 text-[12.5px] text-ink-3">{t("pages:readiness.backup.intro")}</div>
            <div className="stats c3 !m-3">
                <div className="stat" data-testid={T.backupProtected}>
                    <b>{by.protected || 0}</b>
                    <span>{t("pages:readiness.backup.status.protected")}</span>
                </div>
                <div className="stat" data-testid={T.backupGaps}>
                    <b>{(by.stale || 0) + (by.failing || 0) + (by.unprotected || 0)}</b>
                    <span>{t("pages:readiness.backup.gaps")}</span>
                </div>
                <div className="stat" data-testid={T.backupAtRisk}>
                    <b>{data.summary?.dataAtRiskGi ?? 0} GiB</b>
                    <span>{t("pages:readiness.backup.atRisk")}</span>
                </div>
            </div>
            {items.length === 0 ? (
                <div className="empty" data-testid={T.backupEmpty}>{t("pages:readiness.backup.empty")}</div>
            ) : (
                <div className="max-h-[60vh] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:readiness.backup.title")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("pages:readiness.backup.colNamespace")}</th>
                                <th>{t("pages:readiness.backup.colStatus")}</th>
                                <th>{t("pages:readiness.backup.colData")}</th>
                                <th>{t("pages:readiness.backup.colLastGood")}</th>
                                <th>{t("pages:readiness.backup.colReason")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {items.map((n) => (
                                <tr key={n.namespace} data-testid={T.backupRow(n.namespace)}>
                                    <td className="name">{n.namespace}</td>
                                    <td>
                                        <Pill tone={n.status} label={t(`pages:readiness.backup.status.${n.status}`, { defaultValue: n.status })} testId={T.backupStatus(n.namespace)} />
                                    </td>
                                    <td className="mono text-[12.5px]">{t("pages:readiness.backup.data", { pvcs: n.pvcs, gi: n.capacityGi })}</td>
                                    <td className="mono text-[12.5px]">{n.lastGood || "—"}</td>
                                    <td className="text-[12.5px] text-ink-3">{n.reason}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
            {data.truncated ? <div className="border-t border-border px-4 py-2.5 text-[12.5px] text-ink-3">{t("pages:readiness.truncated")}</div> : null}
        </div>
    );
}

/** Read-only PodDisruptionBudget audit: what would stall a drain. */
function DisruptionAudit() {
    const { t } = useTranslation();
    const [data, setData] = useState(null);
    const [error, setError] = useState("");

    useEffect(() => {
        let alive = true;
        api
            .get("/disruption/audit")
            .then((r) => alive && (setData(r.data), setError("")))
            .catch((e) => alive && setError(errText(e)));
        return () => {
            alive = false;
        };
    }, []);

    if (error) return <div className="notice danger" role="alert" data-testid={T.disruptionError}>{error}</div>;
    if (!data) return <Skeleton className="h-32 rounded-2xl bg-muted" />;
    const findings = data.findings || [];

    return (
        <div className="card p-0" data-testid={T.disruptionRoot}>
            <div className="panel-h">{t("pages:readiness.disruption.title")}</div>
            <div className="border-b border-border px-4 py-2.5 text-[12.5px] text-ink-3">{t("pages:readiness.disruption.intro")}</div>
            {findings.length === 0 ? (
                <div className="empty" data-testid={T.disruptionEmpty}>{t("pages:readiness.disruption.empty")}</div>
            ) : (
                <div className="max-h-[60vh] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:readiness.disruption.title")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("pages:readiness.disruption.colSeverity")}</th>
                                <th>{t("pages:readiness.disruption.colTarget")}</th>
                                <th>{t("pages:readiness.disruption.colReason")}</th>
                                <th>{t("pages:readiness.disruption.colAction")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {findings.map((f) => (
                                <tr key={`${f.kind}/${f.namespace}/${f.name || ""}`} data-testid={T.disruptionRow(f.kind, f.namespace, f.name)}>
                                    <td>
                                        <Pill tone={f.severity} label={t(`pages:readiness.severity.${f.severity}`, { defaultValue: f.severity })} />
                                    </td>
                                    <td className="name">
                                        {f.name || t("pages:readiness.disruption.wholeNamespace")}
                                        <small>{f.namespace}</small>
                                    </td>
                                    <td className="text-[12.5px]">
                                        {f.reason}
                                        {f.workloads?.length ? <small className="block text-ink-3">{f.workloads.join(", ")}</small> : null}
                                    </td>
                                    <td className="text-[12.5px] text-ink-3">{f.suggestion}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
            {data.truncated ? <div className="border-t border-border px-4 py-2.5 text-[12.5px] text-ink-3">{t("pages:readiness.truncated")}</div> : null}
        </div>
    );
}

/** Paste manifests; nothing is applied and no cluster is contacted. */
function ApiDeprecations() {
    const { t } = useTranslation();
    const [text, setText] = useState("");
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const [result, setResult] = useState(null);

    const check = async () => {
        setBusy(true);
        setError("");
        try {
            const r = await api.post("/manifests/deprecations", { manifest: text });
            setResult(r.data);
        } catch (e) {
            setResult(null);
            setError(errText(e));
        } finally {
            setBusy(false);
        }
    };

    return (
        <div className="card p-0" data-testid={T.deprecationsRoot}>
            <div className="panel-h">{t("pages:readiness.deprecations.title")}</div>
            <div className="border-b border-border px-4 py-2.5 text-[12.5px] text-ink-3">{t("pages:readiness.deprecations.intro")}</div>
            <div className="space-y-2 p-4">
                <label htmlFor="readiness-manifest" className="text-xs text-muted-foreground">{t("pages:readiness.deprecations.label")}</label>
                <Textarea
                    id="readiness-manifest"
                    data-testid={T.deprecationsInput}
                    value={text}
                    onChange={(e) => setText(e.target.value)}
                    className="mono min-h-[140px] text-xs"
                    spellCheck={false}
                    maxLength={1048576}
                />
                <Button type="button" data-testid={T.deprecationsCheck} onClick={check} disabled={busy || !text.trim()}>
                    {busy ? t("pages:readiness.deprecations.checking") : t("pages:readiness.deprecations.check")}
                </Button>
            </div>
            {error ? <div className="notice danger m-4" role="alert" data-testid={T.deprecationsError}>{error}</div> : null}
            {result ? (
                <div data-testid={T.deprecationsResult}>
                    <div className="border-t border-border px-4 py-2.5 text-[12.5px]" data-testid={T.deprecationsSummary}>
                        {t("pages:readiness.deprecations.summary", { removed: result.removed, deprecated: result.deprecated, objects: result.objects })}
                    </div>
                    {(result.findings || []).length === 0 ? (
                        <div className="empty" data-testid={T.deprecationsClean}>{t("pages:readiness.deprecations.clean")}</div>
                    ) : (
                        <div className="max-h-[50vh] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:readiness.deprecations.title")}>
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>{t("pages:readiness.deprecations.colObject")}</th>
                                        <th>{t("pages:readiness.deprecations.colStatus")}</th>
                                        <th>{t("pages:readiness.deprecations.colRemoved")}</th>
                                        <th>{t("pages:readiness.deprecations.colReplacement")}</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {result.findings.map((f, i) => (
                                        <tr key={`${f.document}/${f.kind}/${f.name || ""}/${i}`} data-testid={T.deprecationsRow(i)}>
                                            <td className="name">
                                                {f.kind}{f.name ? ` ${f.name}` : ""}
                                                <small>{f.apiVersion}</small>
                                            </td>
                                            <td>
                                                <Pill tone={f.status} label={t(`pages:readiness.deprecations.status.${f.status}`, { defaultValue: f.status })} />
                                            </td>
                                            <td className="mono text-[12.5px]">{f.removedIn || "—"}</td>
                                            <td className="text-[12.5px] text-ink-3">{f.replacement}</td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </div>
            ) : null}
        </div>
    );
}

const THRESHOLDS = [70, 80, 90, 95];

/** Read-only hygiene report: one table per audit endpoint (quota, images, autoscaling, services). */
function HygieneAudit({ kind, endpoint }) {
    const { t } = useTranslation();
    const [threshold, setThreshold] = useState(80);
    const [data, setData] = useState(null);
    const [error, setError] = useState("");

    useEffect(() => {
        let alive = true;
        setData(null);
        api
            .get(`/audits/${endpoint}`, { params: kind === "quota" ? { threshold } : undefined })
            .then((r) => alive && (setData(r.data), setError("")))
            .catch((e) => alive && setError(errText(e)));
        return () => {
            alive = false;
        };
    }, [endpoint, kind, threshold]);

    if (error) return <div className="notice danger" role="alert" data-testid={T.auditError(kind)}>{error}</div>;
    if (!data) return <Skeleton className="h-32 rounded-2xl bg-muted" />;
    const findings = data.findings || [];
    const title = t(`pages:readiness.hygiene.${kind}.title`);

    return (
        <div className="card p-0" data-testid={T.auditRoot(kind)}>
            <div className="panel-h flex flex-wrap items-center justify-between gap-2">
                <span>{title}</span>
                {kind === "quota" ? (
                    <label className="flex items-center gap-2 text-[12.5px] text-ink-3">
                        {t("pages:readiness.hygiene.quota.threshold")}
                        <select className="select" data-testid={T.auditThreshold} value={threshold} onChange={(e) => setThreshold(Number(e.target.value))}>
                            {THRESHOLDS.map((n) => (
                                <option key={n} value={n}>{t("pages:readiness.hygiene.quota.thresholdOption", { n })}</option>
                            ))}
                        </select>
                    </label>
                ) : null}
            </div>
            <div className="border-b border-border px-4 py-2.5 text-[12.5px] text-ink-3">{t(`pages:readiness.hygiene.${kind}.intro`)}</div>
            <div className="border-b border-border px-4 py-2 text-[12.5px]" data-testid={T.auditSummary(kind)}>
                {t("pages:readiness.hygiene.summary", { scanned: data.summary?.scanned ?? 0, total: data.summary?.total ?? 0 })}
            </div>
            {findings.length === 0 ? (
                <div className="empty" data-testid={T.auditEmpty(kind)}>{t("pages:readiness.hygiene.empty")}</div>
            ) : (
                <div className="max-h-[60vh] overflow-auto" tabIndex={0} role="region" aria-label={title}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("pages:readiness.hygiene.colSeverity")}</th>
                                <th>{t("pages:readiness.hygiene.colTarget")}</th>
                                <th>{t("pages:readiness.hygiene.colReason")}</th>
                                <th>{t("pages:readiness.hygiene.colAction")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {findings.map((f) => (
                                <tr key={`${f.kind}/${f.namespace}/${f.name}/${f.detail || ""}`} data-testid={T.auditRow(kind, f.kind, f.namespace, f.name)}>
                                    <td>
                                        <Pill tone={f.severity} label={t(`pages:readiness.severity.${f.severity}`, { defaultValue: f.severity })} />
                                    </td>
                                    <td className="name">
                                        {f.name}
                                        <small>{f.namespace}</small>
                                    </td>
                                    <td className="text-[12.5px]">
                                        {f.reason}
                                        {f.detail ? <small className="mono block text-ink-3">{f.detail}</small> : null}
                                    </td>
                                    <td className="text-[12.5px] text-ink-3">{f.suggestion}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
            {data.truncated ? <div className="border-t border-border px-4 py-2.5 text-[12.5px] text-ink-3">{t("pages:readiness.truncated")}</div> : null}
        </div>
    );
}

export function ReadinessPanel() {
    return (
        <div className="space-y-4">
            <BackupCoverage />
            <DisruptionAudit />
            <ApiDeprecations />
            <HygieneAudit kind="quota" endpoint="quota" />
            <HygieneAudit kind="images" endpoint="images" />
            <HygieneAudit kind="autoscaling" endpoint="autoscaling" />
            <HygieneAudit kind="services" endpoint="services" />
        </div>
    );
}
