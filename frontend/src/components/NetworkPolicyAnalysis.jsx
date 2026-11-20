import { Fragment, useEffect, useId, useState } from "react";
import { ChevronDown, ChevronRight, ShieldAlert, ShieldCheck } from "lucide-react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { NETPOL } from "@/constants/testIds";
import { useTranslation, Trans } from "react-i18next";

// Per-pod effective NetworkPolicy position, computed server-side from the
// selectors (GET /networking/policies/{namespace}/analysis). Read-only.

function Pill({ tone, children }) {
    return (
        <span className="pill" style={{ "--c": `var(--${tone})` }}>
            <i aria-hidden="true" />
            {children}
        </span>
    );
}

function Direction({ isolated }) {
    const { t } = useTranslation();
    return isolated ? <Pill tone="green">{t("pages:networkPolicy.isolated")}</Pill> : <Pill tone="orange">{t("pages:networkPolicy.open")}</Pill>;
}

function AllowList({ title, isolated, allows }) {
    const { t } = useTranslation();
    return (
        <div>
            <div className="mb-1 text-[11px] font-[500] uppercase tracking-widest text-ink-3">{title}</div>
            {!isolated ? (
                <div className="text-[12.5px] text-ink-3">{t("pages:networkPolicy.notIsolated")}</div>
            ) : allows.length === 0 ? (
                <div className="text-[12.5px] text-ink-3">{t("pages:networkPolicy.denyAll")}</div>
            ) : (
                <ul className="space-y-1.5">
                    {allows.map((a, i) => (
                        <li key={i} className="text-[12.5px]">
                            <span className="mono">{a.peer}</span>
                            {a.pods && a.pods.length > 0 && (
                                <span className="text-ink-3">
                                    {" "}
                                    → {a.pods.join(", ")}
                                    {a.podCount > a.pods.length ? ` ${t("pages:networkPolicy.more", { n: a.podCount - a.pods.length })}` : ""}
                                </span>
                            )}
                            {a.namespaces && a.namespaces.length > 0 && a.kind === "namespace" && (
                                <span className="text-ink-3"> {t("pages:networkPolicy.toNamespaces", { list: a.namespaces.join(", ") })}</span>
                            )}
                            <span className="mono text-ink-3"> · {a.ports.join(", ")}</span>
                            <small className="block text-ink-4">{t("pages:networkPolicy.policy", { name: a.policy })}</small>
                        </li>
                    ))}
                </ul>
            )}
        </div>
    );
}

export default function NetworkPolicyAnalysis() {
    const { t } = useTranslation();
    const selectId = useId();
    const [namespaces, setNamespaces] = useState(null);
    const [ns, setNs] = useState("");
    const [data, setData] = useState(null);
    const [error, setError] = useState(null);
    const [open, setOpen] = useState(() => new Set());

    // Default to the first namespace that actually has policies.
    useEffect(() => {
        let alive = true;
        Promise.all([api.get("/namespaces"), api.get("/networkpolicies").catch(() => ({ data: [] }))])
            .then(([n, p]) => {
                if (!alive) return;
                const names = (n.data || []).map((x) => x.name).sort();
                const withPolicy = [...new Set((p.data || []).map((x) => x.namespace))].sort();
                setNamespaces(names);
                setNs(withPolicy.find((x) => names.includes(x)) || names[0] || "");
            })
            .catch(() => alive && setNamespaces([]));
        return () => {
            alive = false;
        };
    }, []);

    useEffect(() => {
        if (!ns) return undefined;
        let alive = true;
        setData(null);
        setError(null);
        setOpen(new Set());
        api
            .get(`/networking/policies/${encodeURIComponent(ns)}/analysis`)
            .then((r) => alive && setData(r.data))
            .catch((e) => alive && setError(e?.response?.data?.error || e.message));
        return () => {
            alive = false;
        };
    }, [ns]);

    const toggle = (pod) =>
        setOpen((s) => {
            const next = new Set(s);
            if (next.has(pod)) next.delete(pod);
            else next.add(pod);
            return next;
        });

    return (
        <GlassCard className="overflow-hidden" data-testid={NETPOL.root}>
            <div className="card-h">
                <h2 className="flex items-center gap-2 text-[15px]">
                    <ShieldCheck size={16} className="text-tone-link" aria-hidden="true" /> {t("pages:networkPolicy.title")}
                </h2>
                <div className="r flex items-center gap-2">
                    <label htmlFor={selectId} className="text-[12.5px] text-ink-3">
                        {t("common:fields.namespace")}
                    </label>
                    <select
                        id={selectId}
                        data-testid={NETPOL.namespace}
                        value={ns}
                        onChange={(e) => setNs(e.target.value)}
                        className="h-9 rounded-xl border border-input bg-card px-3 text-sm"
                    >
                        {(namespaces || []).map((n) => (
                            <option key={n} value={n}>
                                {n}
                            </option>
                        ))}
                    </select>
                </div>
            </div>
            <p className="px-4 pb-2 text-[12.5px] text-ink-3">
                {t("pages:networkPolicy.description")}
            </p>
            {!namespaces || (ns && !data && !error) ? (
                <Skeleton className="m-4 h-24 rounded-2xl bg-muted" />
            ) : error ? (
                <div className="empty">{error}</div>
            ) : !data ? (
                <div className="empty">{t("pages:networkPolicy.noNamespaces")}</div>
            ) : (
                <>
                    <div className="flex flex-wrap items-center gap-x-5 gap-y-1 px-4 pb-3 text-[13px]" data-testid={NETPOL.summary}>
                        <span>
                            <Trans i18nKey="pages:networkPolicy.policies" count={data.policies} components={{ b: <b /> }} />
                        </span>
                        <span>
                            <Trans i18nKey="pages:networkPolicy.pods" count={data.pods.length} components={{ b: <b /> }} />
                        </span>
                        {data.defaultDenyIngress && <Pill tone="green">{t("pages:networkPolicy.defaultDenyIngress")}</Pill>}
                        {data.defaultDenyEgress && <Pill tone="green">{t("pages:networkPolicy.defaultDenyEgress")}</Pill>}
                    </div>
                    {data.uncovered.length > 0 && (
                        <div className="notice mx-4 mb-3" role="status" data-testid={NETPOL.uncovered}>
                            <ShieldAlert size={16} aria-hidden="true" />
                            <span>
                                <Trans
                                    i18nKey="pages:networkPolicy.uncovered"
                                    count={data.uncovered.length}
                                    values={{ list: `${data.uncovered.slice(0, 8).join(", ")}${data.uncovered.length > 8 ? ", …" : ""}` }}
                                    components={{ b: <b />, mono: <span className="mono" /> }}
                                />
                            </span>
                        </div>
                    )}
                    {(data.warnings || [])
                        .filter((w) => !/not selected by any NetworkPolicy|No NetworkPolicies/.test(w))
                        .map((w) => (
                            <p key={w} className="px-4 pb-2 text-[12.5px] text-ink-3">
                                {w}
                            </p>
                        ))}
                    {data.pods.length === 0 ? (
                        <div className="empty">{t("pages:networkPolicy.noPods")}</div>
                    ) : (
                        <div className="max-h-[480px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:networkPolicy.region")}>
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>Pod</th>
                                        <th>{t("pages:networkPolicy.columns.ingress")}</th>
                                        <th>{t("pages:networkPolicy.columns.egress")}</th>
                                        <th>{t("pages:networkPolicy.columns.policies")}</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {data.pods.map((p) => (
                                        <Fragment key={p.pod}>
                                            <tr data-testid={NETPOL.podRow(p.pod)}>
                                                <td className="name">
                                                    <button
                                                        type="button"
                                                        className="flex items-center gap-1.5 text-left"
                                                        aria-expanded={open.has(p.pod)}
                                                        aria-label={t("pages:networkPolicy.showTraffic", { pod: p.pod })}
                                                        onClick={() => toggle(p.pod)}
                                                    >
                                                        {open.has(p.pod) ? <ChevronDown size={14} aria-hidden="true" /> : <ChevronRight size={14} aria-hidden="true" />}
                                                        {p.pod}
                                                    </button>
                                                </td>
                                                <td>
                                                    <Direction isolated={p.ingressIsolated} />
                                                </td>
                                                <td>
                                                    <Direction isolated={p.egressIsolated} />
                                                </td>
                                                <td className="mono wrap">{p.policies.join(", ") || "—"}</td>
                                            </tr>
                                            {open.has(p.pod) && (
                                                <tr data-testid={NETPOL.podDetail(p.pod)}>
                                                    <td colSpan={4}>
                                                        <div className="grid g2">
                                                            <AllowList title={t("pages:networkPolicy.allowedIngress")} isolated={p.ingressIsolated} allows={p.ingress} />
                                                            <AllowList title={t("pages:networkPolicy.allowedEgress")} isolated={p.egressIsolated} allows={p.egress} />
                                                        </div>
                                                    </td>
                                                </tr>
                                            )}
                                        </Fragment>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </>
            )}
        </GlassCard>
    );
}
