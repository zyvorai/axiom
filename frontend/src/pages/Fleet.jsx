import { useCallback, useEffect, useState } from "react";
import { RefreshCw, Search, Server } from "lucide-react";
import { resourceHref } from "@/lib/resourceHref";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import { useCluster } from "@/lib/cluster";
import { formatDistanceToNowStrict } from "date-fns";
import { Trans, useTranslation } from "react-i18next";
import i18n, { dateFnsLocale } from "@/lib/i18n";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { FLEET, FLEET_HISTORY as H } from "@/constants/testIds";

const POLL_MS = 30000;

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true, locale: dateFnsLocale() });
    } catch {
        return String(iso ?? "");
    }
}

// How a probe state is shown (tone tokens from the shared pill style).
const STATE = {
    healthy: { tone: "var(--green)", labelKey: "pages:fleet.state.healthy" },
    degraded: { tone: "var(--orange)", labelKey: "pages:fleet.state.degraded" },
    unreachable: { tone: "var(--red)", labelKey: "pages:fleet.state.unreachable" },
    unknown: { tone: "var(--amber)", labelKey: "pages:fleet.state.unknown" },
};

function Meter({ label, value }) {
    const v = Math.max(0, Math.min(100, Math.round(value || 0)));
    return (
        <div>
            <div className="flex items-baseline justify-between text-xs text-ink-3">
                <span>{label}</span>
                <span className="mono">{v}%</span>
            </div>
            <div className="meter !mt-1.5" role="img" aria-label={`${label} ${v}%`}>
                <i style={{ width: `${v}%`, background: v > 80 ? "var(--orange)" : "var(--blue)" }} />
            </div>
        </div>
    );
}

function Metric({ label, value, of }) {
    return (
        <div>
            <div className="text-[11px] uppercase tracking-wider text-ink-3">{label}</div>
            <div className="mono text-sm">
                {value}
                {of != null ? <span className="text-ink-3"> / {of}</span> : null}
            </div>
        </div>
    );
}


const STATE_BAR = { healthy: "var(--green)", degraded: "var(--orange)", unreachable: "var(--red)", unknown: "var(--ink-3)" };

function fmtDuration(sec) {
    if (sec == null) return i18n.t("pages:fleet.history.ongoing");
    if (sec < 90) return `${Math.round(sec)}s`;
    if (sec < 5400) return `${Math.round(sec / 60)} min`;
    return `${(sec / 3600).toFixed(1)} h`;
}

/** A compact timeline of the cluster's state over the window: one bar per bucket. */
function HistoryStrip({ cluster, samples }) {
    const { t } = useTranslation();
    if (!samples || samples.length === 0) return null;
    const w = 240;
    const bar = w / samples.length;
    const desc = t("pages:fleet.history.stripAria", { bad: samples.filter((x) => x.state !== "healthy").length, total: samples.length });
    return (
        <svg viewBox={`0 0 ${w} 14`} className="h-3.5 w-full" role="img" aria-label={desc} data-testid={H.strip(cluster)} preserveAspectRatio="none">
            {samples.map((x, i) => (
                <rect key={i} x={i * bar} y={0} width={Math.max(bar - 0.4, 0.6)} height={14} rx={1} style={{ fill: STATE_BAR[x.state] || STATE_BAR.unknown }} />
            ))}
        </svg>
    );
}

function HistoryBlock({ h }) {
    const { t } = useTranslation();
    if (!h) return null;
    const incidents = (h.incidents || []).slice(-3).reverse();
    return (
        <div className="space-y-1.5 border-t border-border pt-3">
            <div className="flex items-baseline justify-between text-xs text-ink-3">
                <span>
                    <Trans
                        i18nKey="pages:fleet.history.reachable"
                        values={{ pct: h.uptimePercent }}
                        components={{ b: <b className="mono text-ink-2" data-testid={H.uptime(h.cluster)} /> }}
                    />
                </span>
                <span>{t("pages:fleet.history.failedChecks", { failed: h.failedChecks, total: h.checks })}</span>
            </div>
            <HistoryStrip cluster={h.cluster} samples={h.samples} />
            {incidents.length === 0 ? (
                <p className="text-[11.5px] text-ink-3" data-testid={H.none(h.cluster)}>{t("pages:fleet.history.noIncidents")}</p>
            ) : (
                <ul className="space-y-0.5 text-[11.5px]" data-testid={H.incidents(h.cluster)}>
                    {incidents.map((inc, i) => (
                        <li key={`${inc.from}-${i}`} data-testid={H.incident(h.cluster, i)} className={inc.state === "unreachable" ? "text-tone-red" : "text-tone-amber"}>
                            {t(inc.state === "unreachable" ? "pages:fleet.history.unreachableFor" : "pages:fleet.history.degradedFor", { duration: fmtDuration(inc.durationSeconds) })}
                            <span className="text-ink-3"> · {relTime(inc.from)}{inc.reason ? ` · ${inc.reason}` : ""}</span>
                        </li>
                    ))}
                </ul>
            )}
        </div>
    );
}

function ClusterCard({ c, onSwitch, history }) {
    const { t } = useTranslation();
    const st = STATE[c.state] || STATE.unknown;
    const stLabel = t(st.labelKey);
    const reachable = c.state === "healthy" || c.state === "degraded";
    return (
        <GlassCard data-testid={FLEET.card(c.name)} className="flex flex-col gap-3 p-4">
            <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                    <div className="flex items-center gap-2">
                        <Server size={15} className="shrink-0 text-ink-3" aria-hidden="true" />
                        <h2 className="truncate text-[15px] font-semibold">{c.label || c.name}</h2>
                        {c.current && <span className="pill soft">{t("pages:fleet.active")}</span>}
                    </div>
                    <div className="mono mt-0.5 truncate text-[11.5px] text-ink-3" title={c.server || c.name}>
                        {c.server || c.name}
                    </div>
                </div>
                <span data-testid={FLEET.state(c.name)} aria-label={t("pages:fleet.stateAria", { name: c.label || c.name, state: stLabel })}>
                    <span className="pill" style={{ "--c": st.tone }}>
                        <i aria-hidden="true" />
                        {stLabel}
                    </span>
                </span>
            </div>

            {(c.error || c.reason) && (
                <p data-testid={FLEET.detail(c.name)} className={`text-[12.5px] ${c.error ? "text-tone-red" : "text-tone-amber"}`}>
                    {c.error || c.reason}
                </p>
            )}

            {reachable ? (
                <>
                    <div className="grid grid-cols-4 gap-3">
                        <Metric label={t("pages:fleet.metrics.nodes")} value={c.readyNodes} of={c.nodes} />
                        <Metric label={t("pages:fleet.metrics.pods")} value={c.runningPods} of={c.pods} />
                        <Metric label={t("pages:fleet.metrics.apps")} value={c.workloads} />
                        <Metric label={t("pages:fleet.metrics.warnings")} value={c.warnings} />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                        <Meter label={t("common:fields.cpu")} value={c.cpuPercent} />
                        <Meter label={t("common:fields.memory")} value={c.memoryPercent} />
                    </div>
                </>
            ) : null}

            <HistoryBlock h={history} />

            <div className="mt-auto flex items-center justify-between gap-2 border-t border-border pt-3 text-xs text-ink-3">
                <span>
                    {c.checkedAt ? t("pages:fleet.checkedLine", { ms: c.latencyMs, time: relTime(c.checkedAt) }) : t("pages:fleet.notChecked")}
                    {c.since && c.state !== "healthy" ? t("pages:fleet.since", { time: relTime(c.since) }) : ""}
                </span>
                {!c.current && (
                    <button type="button" className="btn btn-line btn-sm" data-testid={FLEET.switch(c.name)} onClick={() => onSwitch(c.name)}>
                        {t("pages:fleet.switchTo")}
                    </button>
                )}
            </div>
        </GlassCard>
    );
}

const KIND_PILL = { Workload: "var(--blue)", Service: "var(--green)", Namespace: "var(--amber)" };

function hitHref(h) {
    if (h.kind === "Workload") return resourceHref(h.namespace, `${h.workloadKind}/${h.name}`);
    if (h.kind === "Service") return resourceHref(h.namespace, `Service/${h.name}`);
    return resourceHref("", `Namespace/${h.name}`);
}

/** Search workloads, services and namespaces across every registered cluster. */
function FleetSearch({ currentName }) {
    const { t } = useTranslation();
    const [q, setQ] = useState("");
    const [kind, setKind] = useState("");
    const [res, setRes] = useState(null);
    const [error, setError] = useState("");
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        const text = q.trim();
        if (text.length < 2) {
            setRes(null);
            setError("");
            return undefined;
        }
        let alive = true;
        const timer = setTimeout(async () => {
            setBusy(true);
            try {
                const { data } = await api.get("/fleet/search", { params: { q: text, kind: kind || undefined, limit: 50 } });
                if (alive) {
                    setRes(data);
                    setError("");
                }
            } catch (e) {
                if (alive) setError(e?.response?.data?.error || e?.message || t("errors:fleet.searchFailed"));
            } finally {
                if (alive) setBusy(false);
            }
        }, 300);
        return () => {
            alive = false;
            clearTimeout(timer);
        };
    }, [q, kind, t]);

    // Open a hit: make its cluster the active one for this tab, then go to the resource.
    const open = async (h) => {
        const href = hitHref(h);
        if (!href) return;
        try {
            if (h.cluster !== currentName) await api.post(`/clusters/${encodeURIComponent(h.cluster)}/activate`);
        } catch (e) {
            setError(e?.response?.data?.error || e?.message || t("errors:fleet.switchFailed"));
            return;
        }
        window.location.assign(href);
    };

    const skipped = res?.clusters?.skipped || [];
    const errors = res?.clusters?.errors || [];
    return (
        <GlassCard className="space-y-3 p-4">
            <div className="flex flex-wrap items-center gap-2">
                <div className="relative min-w-[14rem] flex-1">
                    <Search size={14} className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-ink-3" aria-hidden="true" />
                    <Input
                        aria-label={t("pages:fleet.search.aria")}
                        data-testid={FLEET.search}
                        value={q}
                        onChange={(e) => setQ(e.target.value)}
                        placeholder={t("pages:fleet.search.placeholder")}
                        className="bg-muted/30 pl-9"
                    />
                </div>
                <select aria-label={t("common:fields.kind")} data-testid={FLEET.searchKind} className="select" value={kind} onChange={(e) => setKind(e.target.value)}>
                    <option value="">{t("pages:fleet.search.everything")}</option>
                    <option value="Workload">{t("pages:fleet.search.workloads")}</option>
                    <option value="Service">{t("pages:fleet.search.services")}</option>
                    <option value="Namespace">{t("pages:fleet.search.namespaces")}</option>
                </select>
            </div>
            {error ? <div className="notice danger" role="alert">{error}</div> : null}
            {res && (
                <>
                    {(skipped.length > 0 || errors.length > 0) && (
                        <div className="notice" role="status" data-testid={FLEET.searchSkipped}>
                            {t("pages:fleet.search.notSearched", { list: [...skipped.map((s) => `${s.cluster} (${s.reason})`), ...errors.map((e) => `${e.cluster} (${e.error})`)].join("; ") })}
                        </div>
                    )}
                    {res.results.length === 0 ? (
                        <div className="empty" data-testid={FLEET.searchNone}>{t("pages:fleet.search.noMatches", { count: res.clusters.searched })}</div>
                    ) : (
                        <div className="max-h-[50vh] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:fleet.search.results")} data-testid={FLEET.searchResults}>
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>{t("pages:fleet.search.columns.cluster")}</th>
                                        <th>{t("pages:fleet.search.columns.match")}</th>
                                        <th>{t("common:fields.status")}</th>
                                        <th><span className="sr-only">{t("common:actions.open")}</span></th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {res.results.map((h) => (
                                        <tr key={`${h.cluster}/${h.kind}/${h.namespace}/${h.name}`} data-testid={FLEET.searchRow(h.cluster, h.kind, h.namespace, h.name)}>
                                            <td className="name">
                                                {h.clusterLabel || h.cluster}
                                                <small>{h.clusterState}</small>
                                            </td>
                                            <td>
                                                <span className="pill" style={{ "--c": KIND_PILL[h.kind] || "var(--blue)" }}>
                                                    <i aria-hidden="true" />
                                                    {h.kind === "Workload" ? h.workloadKind : h.kind}
                                                </span>{" "}
                                                <b>{h.name}</b>
                                                {h.namespace ? <small className="ml-1 text-ink-3">{h.namespace}</small> : null}
                                                {h.detail ? <div className="text-[11.5px] text-ink-3">{h.detail}</div> : null}
                                            </td>
                                            <td className="text-[12.5px]">{h.status}</td>
                                            <td>
                                                <button type="button" className="btn btn-line btn-sm" data-testid={FLEET.searchOpen(h.cluster, h.kind, h.namespace, h.name)} onClick={() => open(h)}>
                                                    {t("common:actions.open")}
                                                </button>
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                    {res.truncated ? <p className="text-xs text-ink-3">{t("pages:fleet.search.truncated", { shown: res.results.length, total: res.total })}</p> : null}
                </>
            )}
            {busy && !res ? <Skeleton className="h-10 rounded-xl bg-muted" /> : null}
        </GlassCard>
    );
}

/** Every registered cluster at a glance: reachability, capacity and warnings, probed in the background. */
export default function Fleet() {
    const { t } = useTranslation();
    const { activate } = useCluster();
    const [fleet, setFleet] = useState(null);
    const [error, setError] = useState("");
    const [refreshing, setRefreshing] = useState(false);
    const [histWindow, setHistWindow] = useState("24h");
    const [history, setHistory] = useState({});

    useEffect(() => {
        let alive = true;
        const tick = () =>
            api
                .get("/fleet/history", { params: { window: histWindow } })
                .then((r) => alive && setHistory(Object.fromEntries((r.data?.clusters || []).map((c) => [c.cluster, c]))))
                .catch(() => alive && setHistory({}));
        tick();
        const timer = setInterval(tick, POLL_MS);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, [histWindow]);

    const load = useCallback(async (refresh) => {
        setRefreshing(true);
        try {
            const { data } = refresh ? await api.post("/fleet/refresh") : await api.get("/fleet");
            setFleet(data);
            setError("");
        } catch (e) {
            setError(e?.response?.data?.error || e?.message || t("errors:fleet.loadFailed"));
        } finally {
            setRefreshing(false);
        }
    }, [t]);

    useEffect(() => {
        load(false);
        const timer = setInterval(() => load(false), POLL_MS);
        return () => clearInterval(timer);
    }, [load]);

    const s = fleet?.summary;
    const attention = s ? s.degraded + s.unreachable : 0;
    const stat = (key, label, value, suffix) => (
        <div className="stat media" data-testid={FLEET.summary(key)}>
            <b>
                {value}
                {suffix ? <small className="ml-1 text-ink-3">{suffix}</small> : null}
            </b>
            <span>{label}</span>
        </div>
    );

    return (
        <div data-testid={FLEET.root} className="page">
            <PageHeader
                title={t("pages:fleet.title")}
                description={t("pages:fleet.description")}
                action={
                    <button type="button" data-testid={FLEET.refresh} className="btn btn-line" onClick={() => load(true)} disabled={refreshing}>
                        <RefreshCw size={15} className={refreshing ? "animate-spin" : ""} /> {refreshing ? t("pages:fleet.checking") : t("pages:fleet.checkNow")}
                    </button>
                }
            />

            {error && (
                <div className="notice danger" role="alert">
                    {error}
                </div>
            )}

            {!fleet && !error && <Skeleton className="h-64 rounded-2xl bg-muted" />}

            {fleet && s && (
                <>
                    <div className="stats">
                        {stat("clusters", t("pages:fleet.summary.clusters"), s.clusters)}
                        {stat("healthy", t("pages:fleet.summary.healthy"), s.healthy)}
                        {stat("attention", t("pages:fleet.summary.attention"), attention)}
                        {stat("nodes", t("pages:fleet.summary.nodes"), s.readyNodes, `/ ${s.nodes}`)}
                        {stat("pods", t("pages:fleet.summary.pods"), s.runningPods, `/ ${s.pods}`)}
                        {stat("warnings", t("pages:fleet.summary.warnings"), s.warnings)}
                    </div>

                    <div className="flex items-center justify-end gap-2 text-xs text-ink-3">
                        <label htmlFor="fleet-history-window">{t("pages:fleet.history.label")}</label>
                        <select id="fleet-history-window" data-testid={H.window} className="select" value={histWindow} onChange={(e) => setHistWindow(e.target.value)}>
                            <option value="1h">{t("pages:fleet.history.last1h")}</option>
                            <option value="6h">{t("pages:fleet.history.last6h")}</option>
                            <option value="24h">{t("pages:fleet.history.last24h")}</option>
                        </select>
                    </div>

                    <FleetSearch currentName={fleet.clusters.find((c) => c.current)?.name} />

                    {fleet.clusters.length === 0 ? (
                        <GlassCard className="p-6">
                            <div className="empty" data-testid={FLEET.empty}>
                                {t("pages:fleet.empty")}
                            </div>
                        </GlassCard>
                    ) : (
                        <div className="grid gap-3.5 md:grid-cols-2 xl:grid-cols-3">
                            {fleet.clusters.map((c) => (
                                <ClusterCard key={c.name} c={c} onSwitch={activate} history={history[c.name]} />
                            ))}
                        </div>
                    )}
                    <p className="text-xs text-ink-3" data-testid={FLEET.checked}>
                        {t("pages:fleet.totals")}
                        {fleet.checkedAt ? t("pages:fleet.lastCheck", { time: relTime(fleet.checkedAt) }) : ""}
                        {fleet.intervalSeconds > 0 ? t("pages:fleet.interval", { n: fleet.intervalSeconds }) : t("pages:fleet.backgroundOff")}
                    </p>
                </>
            )}
        </div>
    );
}
