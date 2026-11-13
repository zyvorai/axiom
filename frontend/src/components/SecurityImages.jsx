import { useEffect, useId, useState } from "react";
import { Boxes, Package } from "lucide-react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { SECURITY_VULNS } from "@/constants/testIds";

// Read-only per-image vulnerability rollup, top CVEs and SBOM summary from
// Trivy Operator VulnerabilityReports / SbomReports (GET /security/vulnerabilities).

const SEV_KEYS = ["critical", "high", "medium", "low"];
const SEV_TONE = { critical: "red", high: "orange", medium: "amber", low: "blue", unknown: "ink-3" };
const SEV_LABEL = {
    critical: "pages:security.vulns.sevCritical",
    high: "pages:security.vulns.sevHigh",
    medium: "pages:security.vulns.sevMedium",
    low: "pages:security.vulns.sevLow",
};

function Pill({ tone, children }) {
    return (
        <span className="pill" style={{ "--c": `var(--${tone})` }}>
            <i aria-hidden="true" />
            {children}
        </span>
    );
}

function Counts({ c }) {
    const { t } = useTranslation();
    const on = SEV_KEYS.filter((k) => c?.[k] > 0);
    if (!on.length) return <span className="text-ink-4">—</span>;
    return (
        <span className="flex flex-wrap gap-1">
            {on.map((k) => (
                <Pill key={k} tone={SEV_TONE[k]}>
                    {t(`pages:security.sevPill.${k}`, { n: c[k] })}
                </Pill>
            ))}
        </span>
    );
}

export default function SecurityImages({ namespace }) {
    const { t } = useTranslation();
    const sevId = useId();
    const [sev, setSev] = useState("all");
    const [data, setData] = useState(null);
    const [error, setError] = useState(null);

    useEffect(() => {
        let alive = true;
        setData(null);
        setError(null);
        const params = {};
        if (namespace !== "all") params.namespace = namespace;
        if (sev !== "all") params.severity = sev;
        api
            .get("/security/vulnerabilities", { params })
            .then((r) => alive && setData(r.data))
            .catch((e) => alive && setError(e?.response?.data?.error || e.message));
        return () => {
            alive = false;
        };
    }, [namespace, sev]);

    return (
        <section data-testid={SECURITY_VULNS.root} className="mt-3.5">
            <GlassCard className="overflow-hidden">
                <div className="card-h">
                    <h2 className="flex items-center gap-2 text-[15px]">
                        <Boxes size={16} className="text-tone-link" aria-hidden="true" /> {t("pages:security.vulns.title")}
                    </h2>
                    <span className="r flex items-center gap-2 text-[12.5px] text-ink-3">
                        <label htmlFor={sevId}>{t("pages:security.vulns.severity")}</label>
                        <select
                            id={sevId}
                            data-testid={SECURITY_VULNS.severity}
                            value={sev}
                            onChange={(e) => setSev(e.target.value)}
                            className="h-8 rounded-xl border border-input bg-card px-2 text-sm"
                        >
                            <option value="all">{t("pages:security.vulns.sevAll")}</option>
                            {SEV_KEYS.map((k) => (
                                <option key={k} value={k}>
                                    {t(SEV_LABEL[k])}
                                </option>
                            ))}
                        </select>
                    </span>
                </div>
                {error && (
                    <div className="notice danger m-3" role="alert">
                        {error}
                    </div>
                )}
                {!data && !error && <Skeleton className="m-3 h-40 rounded-2xl bg-muted" />}
                {data && !data.available && (
                    <div className="empty" data-testid={SECURITY_VULNS.empty}>
                        <b>{t("pages:security.vulns.noVulnReports")}</b>
                        {data.installHint && <p className="mx-auto mt-1 max-w-[640px] text-[13px] text-ink-3">{data.installHint}</p>}
                    </div>
                )}
                {data && data.available && (
                    <>
                        <p className="px-4 pb-2 text-[12.5px] text-ink-3">
                            {t("pages:security.vulns.imagesCount", { n: data.images.length })} · {t("pages:security.vulns.fixableOf", { n: data.fixable })}
                        </p>
                        {data.images.length === 0 ? (
                            <div className="empty">{t("pages:security.vulns.noImages")}</div>
                        ) : (
                            <div className="max-h-[560px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:security.vulns.imagesRegion")}>
                                <table className="tbl">
                                    <thead>
                                        <tr>
                                            <th>{t("pages:security.vulns.colImage")}</th>
                                            <th>{t("pages:security.vulns.colCounts")}</th>
                                            <th>{t("pages:security.vulns.colFixable")}</th>
                                            <th>{t("pages:security.vulns.colWorkloads")}</th>
                                            <th>{t("pages:security.vulns.colSbom")}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {data.images.map((im) => (
                                            <tr key={im.image} data-testid={SECURITY_VULNS.imageRow(im.image)}>
                                                <td className="name">
                                                    <span className="mono break-all">{im.image}</span>
                                                    {im.fixes.length > 0 && (
                                                        <details className="mt-1 text-[12px] text-ink-3">
                                                            <summary>{t("pages:security.vulns.fixes")}</summary>
                                                            <ul className="mt-1">
                                                                {im.fixes.map((f) => (
                                                                    <li key={f.package} className="mono">
                                                                        {t("pages:security.vulns.fixLine", { pkg: f.package, installed: f.installed, fixed: f.fixedIn, n: f.fixes })}
                                                                    </li>
                                                                ))}
                                                            </ul>
                                                        </details>
                                                    )}
                                                </td>
                                                <td>
                                                    <Counts c={im.counts} />
                                                </td>
                                                <td>{im.fixable}</td>
                                                <td className="text-[12.5px]">
                                                    {im.workloads.map((w) => (
                                                        <div key={`${w.namespace}/${w.kind}/${w.name}`}>
                                                            {w.name}
                                                            <small className="text-ink-3">
                                                                {" "}
                                                                {w.kind} · {w.namespace}
                                                            </small>
                                                        </div>
                                                    ))}
                                                </td>
                                                <td className="text-[12.5px]">{im.hasSbom ? t("pages:security.vulns.components", { count: im.components }) : "—"}</td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>
                        )}
                        {data.topCves.length > 0 && (
                            <div className="mt-2">
                                <h3 className="px-4 text-[13px] font-semibold">{t("pages:security.vulns.topCves")}</h3>
                                <div className="max-h-[420px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:security.vulns.cvesRegion")} data-testid={SECURITY_VULNS.cves}>
                                    <table className="tbl">
                                        <thead>
                                            <tr>
                                                <th>{t("pages:security.vulns.colCve")}</th>
                                                <th>{t("pages:security.vulns.colSeverity")}</th>
                                                <th>{t("pages:security.vulns.colPackage")}</th>
                                                <th>{t("pages:security.vulns.colInstalled")}</th>
                                                <th>{t("pages:security.vulns.colFixedIn")}</th>
                                                <th>{t("pages:security.vulns.colImages")}</th>
                                            </tr>
                                        </thead>
                                        <tbody>
                                            {data.topCves.map((c) => (
                                                <tr key={`${c.id}/${c.package}`} data-testid={SECURITY_VULNS.cveRow(c.id)}>
                                                    <td className="name">
                                                        {c.link ? (
                                                            <a href={c.link} target="_blank" rel="noreferrer noopener">
                                                                {c.id}
                                                            </a>
                                                        ) : (
                                                            c.id
                                                        )}
                                                        {c.title && <small>{c.title}</small>}
                                                    </td>
                                                    <td>
                                                        <Pill tone={SEV_TONE[String(c.severity).toLowerCase()] || "ink-3"}>{c.severity}</Pill>
                                                    </td>
                                                    <td className="mono">{c.package}</td>
                                                    <td className="mono">{c.installed}</td>
                                                    <td className="mono">{c.fixedIn || t("pages:security.vulns.noFix")}</td>
                                                    <td>{c.images.length}</td>
                                                </tr>
                                            ))}
                                        </tbody>
                                    </table>
                                </div>
                            </div>
                        )}
                    </>
                )}
            </GlassCard>
            {data && data.available && (
                <GlassCard className="mt-3.5 overflow-hidden" data-testid={SECURITY_VULNS.sbom}>
                    <div className="card-h">
                        <h2 className="flex items-center gap-2 text-[15px]">
                            <Package size={16} className="text-tone-link" aria-hidden="true" /> {t("pages:security.vulns.sbom")}
                        </h2>
                        <span className="r text-[12.5px] text-ink-3">{t("pages:security.vulns.sbomCount", { n: data.sbom.reduce((n, s) => n + s.components, 0) })}</span>
                    </div>
                    {data.sbom.length === 0 ? (
                        <div className="empty">{t("pages:security.vulns.noSbom")}</div>
                    ) : (
                        <div className="max-h-[420px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:security.vulns.sbomRegion")}>
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>{t("pages:security.vulns.colImage")}</th>
                                        <th>{t("pages:security.vulns.colSbom")}</th>
                                        <th>{t("pages:security.vulns.licenses")}</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {data.sbom.map((s) => (
                                        <tr key={s.image} data-testid={SECURITY_VULNS.sbomRow(s.image)}>
                                            <td className="name mono break-all">{s.image}</td>
                                            <td>{t("pages:security.vulns.components", { count: s.components })}</td>
                                            <td className="text-[12.5px]">
                                                {s.licenses.length === 0 ? (
                                                    t("pages:security.vulns.noLicenses")
                                                ) : (
                                                    <>
                                                        {s.licenses.map((l) => `${l.license} (${l.count})`).join(", ")}
                                                        {s.unlicensed > 0 && <small className="block text-ink-3">{t("pages:security.vulns.unlicensed", { n: s.unlicensed })}</small>}
                                                    </>
                                                )}
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </GlassCard>
            )}
        </section>
    );
}
