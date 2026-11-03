import { useEffect, useId, useState } from "react";
import { ShieldAlert, ShieldCheck, Bug, ClipboardCheck, ScrollText } from "lucide-react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { SECURITY } from "@/constants/testIds";

// /security: read-only aggregation of scanner output already in the cluster
// (Trivy Operator VulnerabilityReports / ConfigAuditReports and wgpolicyk8s.io
// PolicyReports from Kyverno or Gatekeeper). Axiom runs no scanner itself.

// [key, tone, count label key, short pill key]
const SEV = [
    ["critical", "red", "pages:security.sevCount.critical", "pages:security.sevPill.critical"],
    ["high", "orange", "pages:security.sevCount.high", "pages:security.sevPill.high"],
    ["medium", "amber", "pages:security.sevCount.medium", "pages:security.sevPill.medium"],
    ["low", "blue", "pages:security.sevCount.low", "pages:security.sevPill.low"],
    ["unknown", "ink-3", "pages:security.sevCount.unknown", "pages:security.sevPill.unknown"],
];

// [key, tone, count label key]
const POLICY = [
    ["fail", "red", "pages:security.policyCount.fail"],
    ["warn", "amber", "pages:security.policyCount.warn"],
    ["error", "orange", "pages:security.policyCount.error"],
    ["skip", "ink-3", "pages:security.policyCount.skip"],
    ["pass", "green", "pages:security.policyCount.pass"],
];

function SevPill({ tone, children }) {
    return (
        <span className="pill" style={{ "--c": `var(--${tone})` }}>
            <i aria-hidden="true" />
            {children}
        </span>
    );
}

function sevTone(sev) {
    return (SEV.find(([k]) => k === String(sev || "").toLowerCase()) || SEV[4])[1];
}

// A compact "C 1 · H 3 · M 5 · L 2" cell; zero counts are dimmed instead of dropped so columns line up.
function SevCounts({ c, label }) {
    const { t } = useTranslation();
    const total = SEV.reduce((n, [k]) => n + (c?.[k] || 0), 0);
    if (!total) return <span className="text-ink-4" aria-label={t("pages:security.countsNone", { label })}>—</span>;
    return (
        <span className="flex flex-wrap gap-1" aria-label={t("pages:security.countsList", { label, list: SEV.filter(([k]) => c[k]).map(([k, , countKey]) => t(countKey, { n: c[k] })).join(", ") })}>
            {SEV.filter(([k]) => c[k] > 0).map(([k, tone, , pillKey]) => (
                <SevPill key={k} tone={tone}>
                    {t(pillKey, { n: c[k] })}
                </SevPill>
            ))}
        </span>
    );
}

function PolicyCounts({ c }) {
    const { t } = useTranslation();
    const interesting = POLICY.filter(([k]) => k !== "pass" && c?.[k] > 0);
    if (interesting.length === 0) return c?.pass ? <SevPill tone="green">{t("pages:security.policyCount.pass", { n: c.pass })}</SevPill> : <span className="text-ink-4">—</span>;
    return (
        <span className="flex flex-wrap gap-1">
            {interesting.map(([k, tone, countKey]) => (
                <SevPill key={k} tone={tone}>
                    {t(countKey, { n: c[k] })}
                </SevPill>
            ))}
        </span>
    );
}

const TONE_TEXT = { red: "text-tone-red", orange: "text-tone-orange", amber: "text-tone-amber", green: "text-tone-green" };

function Stat({ icon: Icon, tone, id, value, label }) {
    return (
        <div className="stat" data-testid={SECURITY.count(id)}>
            <Icon size={18} className={TONE_TEXT[tone]} aria-hidden="true" />
            <b className="mt-3">{value}</b>
            <span>{label}</span>
        </div>
    );
}

const DETAIL_TABS = [
    ["vulns", "pages:security.detail.tabs.vulns"],
    ["config", "pages:security.detail.tabs.config"],
    ["policy", "pages:security.detail.tabs.policy"],
];

function WorkloadDetailDialog({ target, onClose }) {
    const { t } = useTranslation();
    const [d, setD] = useState(null);
    const [error, setError] = useState(null);
    const [tab, setTab] = useState("vulns");

    useEffect(() => {
        if (!target) return undefined;
        let alive = true;
        setD(null);
        setError(null);
        setTab("vulns");
        api
            .get(`/security/reports/${encodeURIComponent(target.namespace)}/${encodeURIComponent(target.kind)}/${encodeURIComponent(target.name)}`)
            .then((r) => alive && setD(r.data))
            .catch((e) => alive && setError(e?.response?.data?.error || e.message));
        return () => {
            alive = false;
        };
    }, [target]);

    return (
        <Dialog open={!!target} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="max-w-4xl" data-testid={SECURITY.detail}>
                <DialogHeader>
                    <DialogTitle>
                        {target?.name} <span className="text-[13px] font-normal text-ink-3">{target?.kind} · {target?.namespace}</span>
                    </DialogTitle>
                </DialogHeader>
                {error && <div className="empty">{error}</div>}
                {!d && !error && <Skeleton className="h-48 rounded-2xl bg-muted" />}
                {d && (
                    <div className="stack">
                        {d.images && d.images.length > 0 && (
                            <div className="mono wrap text-[12px] text-ink-3">{d.images.join(" · ")}</div>
                        )}
                        <div className="seg" role="group" aria-label={t("pages:security.detail.reportType")}>
                            {DETAIL_TABS.map(([k, labelKey]) => (
                                <button key={k} type="button" aria-pressed={tab === k} data-testid={SECURITY.detailTab(k)} onClick={() => setTab(k)}>
                                    {t(labelKey)}
                                </button>
                            ))}
                        </div>
                        {tab === "vulns" &&
                            (d.vulnerabilities.length === 0 ? (
                                <div className="empty">{t("pages:security.detail.noVulns")}</div>
                            ) : (
                                <div className="max-h-[420px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:security.detail.tabs.vulns")}>
                                    <table className="tbl">
                                        <thead>
                                            <tr>
                                                <th>{t("pages:security.detail.columns.cve")}</th>
                                                <th>{t("pages:security.detail.columns.severity")}</th>
                                                <th>{t("pages:security.detail.columns.package")}</th>
                                                <th>{t("pages:security.detail.columns.installed")}</th>
                                                <th>{t("pages:security.detail.columns.fixedIn")}</th>
                                            </tr>
                                        </thead>
                                        <tbody>
                                            {d.vulnerabilities.map((v) => (
                                                <tr key={`${v.id}|${v.package}|${v.installed}|${v.image}`} data-testid={SECURITY.vulnRow(v.id)}>
                                                    <td className="name">
                                                        {v.link ? (
                                                            <a href={v.link} target="_blank" rel="noreferrer noopener" className="mono">
                                                                {v.id}
                                                            </a>
                                                        ) : (
                                                            <span className="mono">{v.id}</span>
                                                        )}
                                                        {v.title && <small>{v.title}</small>}
                                                    </td>
                                                    <td>
                                                        <SevPill tone={sevTone(v.severity)}>{v.severity}</SevPill>
                                                    </td>
                                                    <td className="mono wrap">{v.package}</td>
                                                    <td className="mono">{v.installed}</td>
                                                    <td className="mono">{v.fixed || t("pages:security.detail.noFix")}</td>
                                                </tr>
                                            ))}
                                        </tbody>
                                    </table>
                                </div>
                            ))}
                        {tab === "config" &&
                            (d.misconfigurations.length === 0 ? (
                                <div className="empty">{t("pages:security.detail.noMisconfig")}</div>
                            ) : (
                                <div className="max-h-[420px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:security.detail.tabs.config")}>
                                    <table className="tbl">
                                        <thead>
                                            <tr>
                                                <th>{t("pages:security.detail.columns.check")}</th>
                                                <th>{t("pages:security.detail.columns.severity")}</th>
                                            </tr>
                                        </thead>
                                        <tbody>
                                            {d.misconfigurations.map((c) => (
                                                <tr key={c.id}>
                                                    <td className="name">
                                                        <span className="mono">{c.id}</span> {c.title}
                                                        {(c.messages || []).map((m) => (
                                                            <small key={m}>{m}</small>
                                                        ))}
                                                    </td>
                                                    <td>
                                                        <SevPill tone={sevTone(c.severity)}>{c.severity}</SevPill>
                                                    </td>
                                                </tr>
                                            ))}
                                        </tbody>
                                    </table>
                                </div>
                            ))}
                        {tab === "policy" &&
                            (d.policyResults.length === 0 ? (
                                <div className="empty">
                                    {d.policyCounts.pass ? t("pages:security.detail.noPolicyFailsPassing", { n: d.policyCounts.pass }) : t("pages:security.detail.noPolicyFails")}
                                </div>
                            ) : (
                                <div className="max-h-[420px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:security.detail.policyResults")}>
                                    <table className="tbl">
                                        <thead>
                                            <tr>
                                                <th>{t("pages:security.detail.columns.policyRule")}</th>
                                                <th>{t("pages:security.detail.columns.result")}</th>
                                                <th>{t("common:fields.message")}</th>
                                            </tr>
                                        </thead>
                                        <tbody>
                                            {d.policyResults.map((r, i) => (
                                                <tr key={i}>
                                                    <td className="name">
                                                        {r.policy}
                                                        {r.rule && <small>{r.rule}</small>}
                                                    </td>
                                                    <td>
                                                        <SevPill tone={(POLICY.find(([k]) => k === r.result) || POLICY[3])[1]}>{r.result}</SevPill>
                                                    </td>
                                                    <td className="text-[12.5px] text-ink-3">{r.message}</td>
                                                </tr>
                                            ))}
                                        </tbody>
                                    </table>
                                </div>
                            ))}
                        {d.truncated && <p className="text-[12px] text-ink-3">{t("pages:security.detail.truncated")}</p>}
                    </div>
                )}
            </DialogContent>
        </Dialog>
    );
}

export default function Security() {
    const { t } = useTranslation();
    const nsId = useId();
    const [namespaces, setNamespaces] = useState([]);
    const [ns, setNs] = useState("all");
    const [data, setData] = useState(null);
    const [error, setError] = useState(null);
    const [target, setTarget] = useState(null);

    useEffect(() => {
        let alive = true;
        api
            .get("/namespaces")
            .then((r) => alive && setNamespaces((r.data || []).map((n) => n.name).sort()))
            .catch(() => {});
        return () => {
            alive = false;
        };
    }, []);

    useEffect(() => {
        let alive = true;
        setData(null);
        setError(null);
        api
            .get("/security/reports", { params: ns === "all" ? {} : { namespace: ns } })
            .then((r) => alive && setData(r.data))
            .catch((e) => alive && setError(e?.response?.data?.error || e.message));
        return () => {
            alive = false;
        };
    }, [ns]);

    return (
        <div data-testid={SECURITY.root} className="page">
            <PageHeader
                title={t("pages:security.title")}
                description={t("pages:security.description")}
                action={
                    <div className="flex items-center gap-2">
                        <label htmlFor={nsId} className="text-[12.5px] text-ink-3">
                            {t("common:fields.namespace")}
                        </label>
                        <select
                            id={nsId}
                            data-testid={SECURITY.namespace}
                            value={ns}
                            onChange={(e) => setNs(e.target.value)}
                            className="h-9 rounded-xl border border-input bg-card px-3 text-sm"
                        >
                            <option value="all">{t("pages:security.allNamespaces")}</option>
                            {namespaces.map((n) => (
                                <option key={n} value={n}>
                                    {n}
                                </option>
                            ))}
                        </select>
                    </div>
                }
            />
            {error && (
                <div className="notice danger" role="alert">
                    {error}
                </div>
            )}
            {!data && !error && <Skeleton className="h-64 rounded-2xl bg-muted" />}
            {data && !data.available && (
                <GlassCard data-testid={SECURITY.empty}>
                    <div className="empty">
                        <ShieldAlert className="mx-auto mb-2" size={22} aria-hidden="true" />
                        <b>{t("pages:security.noReports")}</b>
                        <p className="mx-auto mt-1 max-w-[640px] text-[13px] text-ink-3" data-testid={SECURITY.installHint}>
                            {data.installHint}
                        </p>
                        <ul className="mx-auto mt-3 max-w-[640px] text-left text-[12.5px] text-ink-3">
                            {data.sources.map((s) => (
                                <li key={s.name}>
                                    <span className="mono">{s.group}</span> — {s.name}: {s.message || t("pages:security.unavailable")}
                                </li>
                            ))}
                        </ul>
                    </div>
                </GlassCard>
            )}
            {data && data.available && (
                <>
                    <div className="grid g4">
                        <Stat icon={Bug} tone="red" id="critical" value={data.vulnerabilities.critical} label={t("pages:security.stats.critical")} />
                        <Stat icon={Bug} tone="orange" id="high" value={data.vulnerabilities.high} label={t("pages:security.stats.high")} />
                        <Stat icon={ClipboardCheck} tone="amber" id="misconfig" value={data.misconfigurations.critical + data.misconfigurations.high} label={t("pages:security.stats.misconfig")} />
                        <Stat icon={ShieldCheck} tone="red" id="policy-fail" value={data.policy.fail} label={t("pages:security.stats.policyFail")} />
                    </div>
                    <div className="mt-3 flex flex-wrap gap-2 text-[12.5px] text-ink-3">
                        {data.sources.map((s) => (
                            <span key={s.name} data-testid={SECURITY.source(s.group)} className="flex items-center gap-1.5">
                                <span className={`status-dot ${s.available ? "ok" : "warn"}`} aria-hidden="true" />
                                {s.name}: {s.available ? t("pages:security.reports", { count: s.reports }) : s.message || t("pages:security.unavailable")}
                            </span>
                        ))}
                    </div>
                    <GlassCard className="mt-3.5 overflow-hidden">
                        <div className="card-h">
                            <h2 className="flex items-center gap-2 text-[15px]">
                                <ScrollText size={16} className="text-tone-link" aria-hidden="true" /> {t("pages:security.workloads")}
                            </h2>
                            <span className="r text-[12.5px] text-ink-3">{t("pages:security.withFindings", { n: data.workloads.length })}</span>
                        </div>
                        {data.workloads.length === 0 ? (
                            <div className="empty">{t("pages:security.noFindings")}</div>
                        ) : (
                            <div className="max-h-[560px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:security.workloadFindings")}>
                                <table className="tbl">
                                    <thead>
                                        <tr>
                                            <th>{t("pages:security.columns.workload")}</th>
                                            <th>{t("pages:security.columns.vulns")}</th>
                                            <th>{t("pages:security.columns.misconfig")}</th>
                                            <th>{t("pages:security.columns.policy")}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {data.workloads.map((w) => (
                                            <tr key={`${w.namespace}/${w.kind}/${w.name}`} data-testid={SECURITY.workloadRow(w.name)}>
                                                <td className="name">
                                                    <button
                                                        type="button"
                                                        className="text-left"
                                                        aria-label={t("pages:security.showFindings", { kind: w.kind, name: w.name })}
                                                        onClick={() => setTarget(w)}
                                                    >
                                                        {w.name}
                                                    </button>
                                                    <small>
                                                        {w.kind} · {w.namespace}
                                                    </small>
                                                </td>
                                                <td>
                                                    <SevCounts c={w.vulnerabilities} label={t("pages:security.columns.vulns")} />
                                                </td>
                                                <td>
                                                    <SevCounts c={w.misconfigurations} label={t("pages:security.columns.misconfig")} />
                                                </td>
                                                <td>
                                                    <PolicyCounts c={w.policy} />
                                                </td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>
                        )}
                        {data.truncated && <p className="px-4 pb-3 text-[12px] text-ink-3">{t("pages:security.truncated")}</p>}
                    </GlassCard>
                </>
            )}
            <WorkloadDetailDialog target={target} onClose={() => setTarget(null)} />
        </div>
    );
}
