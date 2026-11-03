import { useCallback, useEffect, useState } from "react";
import { RefreshCw, Search, Server } from "lucide-react";
import { resourceHref } from "@/lib/resourceHref";
import { Input } from "@/components/ui/input";
import { api } from "@/lib/api";
import { useCluster } from "@/lib/cluster";
import { relTime } from "@/lib/troubleshoot";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { FLEET, FLEET_HISTORY as H } from "@/constants/testIds";

const POLL_MS = 30000;

// How a probe state is shown (tone tokens from the shared pill style).
const STATE = {
    healthy: { tone: "var(--green)", label: "Healthy" },
    degraded: { tone: "var(--orange)", label: "Degraded" },
    unreachable: { tone: "var(--red)", label: "Unreachable" },
    unknown: { tone: "var(--amber)", label: "Not checked yet" },
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
    if (sec == null) return "ongoing";
    if (sec < 90) return `${Math.round(sec)}s`;
    if (sec < 5400) return `${Math.round(sec / 60)} min`;
    return `${(sec / 3600).toFixed(1)} h`;
}

/** A compact timeline of the cluster's state over the window: one bar per bucket. */
function HistoryStrip({ cluster, samples }) {
    if (!samples || samples.length === 0) return null;
    const w = 240;
    const bar = w / samples.length;
    const desc = `State over the selected window: ${samples.filter((x) => x.state !== "healthy").length} of ${samples.length} intervals were not healthy`;
    return (
        <svg viewBox={`0 0 ${w} 14`} className="h-3.5 w-full" role="img" aria-label={desc} data-testid={H.strip(cluster)} preserveAspectRatio="none">
            {samples.map((x, i) => (
                <rect key={i} x={i * bar} y={0} width={Math.max(bar - 0.4, 0.6)} height={14} rx={1} style={{ fill: STATE_BAR[x.state] || STATE_BAR.unknown }} />
            ))}
        </svg>
    );
}

function HistoryBlock({ h }) {
    if (!h) return null;
    const incidents = (h.incidents || []).slice(-3).reverse();
    return (
        <div className="space-y-1.5 border-t border-border pt-3">
            <div className="flex items-baseline justify-between text-xs text-ink-3">
                <span>
                    Reachable <b className="mono text-ink-2" data-testid={H.uptime(h.cluster)}>{h.uptimePercent}%</b>
                </span>
                <span>{h.failedChecks} of {h.checks} checks failed</span>
            </div>
            <HistoryStrip cluster={h.cluster} samples={h.samples} />
            {incidents.length === 0 ? (
                <p className="text-[11.5px] text-ink-3" data-testid={H.none(h.cluster)}>No incidents in this window.</p>
            ) : (
                <ul className="space-y-0.5 text-[11.5px]" data-testid={H.incidents(h.cluster)}>
                    {incidents.map((inc, i) => (
                        <li key={`${inc.from}-${i}`} data-testid={H.incident(h.cluster, i)} className={inc.state === "unreachable" ? "text-tone-red" : "text-tone-amber"}>
                            {inc.state === "unreachable" ? "Unreachable" : "Degraded"} for {fmtDuration(inc.durationSeconds)}
                            <span className="text-ink-3"> · {relTime(inc.from)}{inc.reason ? ` · ${inc.reason}` : ""}</span>
                        </li>
                    ))}
                </ul>
            )}
        </div>
    );
}

function ClusterCard({ c, onSwitch, history }) {
    const st = STATE[c.state] || STATE.unknown;
    const reachable = c.state === "healthy" || c.state === "degraded";
    return (
        <GlassCard data-testid={FLEET.card(c.name)} className="flex flex-col gap-3 p-4">
            <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                    <div className="flex items-center gap-2">
                        <Server size={15} className="shrink-0 text-ink-3" aria-hidden="true" />
                        <h2 className="truncate text-[15px] font-semibold">{c.label || c.name}</h2>
                        {c.current && <span className="pill soft">active</span>}
                    </div>
                    <div className="mono mt-0.5 truncate text-[11.5px] text-ink-3" title={c.server || c.name}>
                        {c.server || c.name}
                    </div>
                </div>
                <span data-testid={FLEET.state(c.name)} aria-label={`${c.label || c.name}: ${st.label}`}>
                    <span className="pill" style={{ "--c": st.tone }}>
                        <i aria-hidden="true" />
                        {st.label}
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
                        <Metric label="Nodes" value={c.readyNodes} of={c.nodes} />
                        <Metric label="Pods" value={c.runningPods} of={c.pods} />
                        <Metric label="Apps" value={c.workloads} />
                        <Metric label="Warnings" value={c.warnings} />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                        <Meter label="CPU" value={c.cpuPercent} />
                        <Meter label="Memory" value={c.memoryPercent} />
                    </div>
                </>
            ) : null}

            <HistoryBlock h={history} />

            <div className="mt-auto flex items-center justify-between gap-2 border-t border-border pt-3 text-xs text-ink-3">
                <span>
                    {c.checkedAt ? `${c.latencyMs} ms · checked ${relTime(c.checkedAt)}` : "Not checked yet"}
                    {c.since && c.state !== "healthy" ? ` · since ${relTime(c.since)}` : ""}
                </span>
                {!c.current && (
                    <button type="button" className="btn btn-line btn-sm" data-testid={FLEET.switch(c.name)} onClick={() => onSwitch(c.name)}>
                        Switch to
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
        const t = setTimeout(async () => {
            setBusy(true);
            try {
                const { data } = await api.get("/fleet/search", { params: { q: text, kind: kind || undefined, limit: 50 } });
                if (alive) {
                    setRes(data);
                    setError("");
                }
            } catch (e) {
                if (alive) setError(e?.response?.data?.error || e?.message || "Search failed");
            } finally {
                if (alive) setBusy(false);
            }
        }, 300);
        return () => {
            alive = false;
            clearTimeout(t);
        };
    }, [q, kind]);

    // Open a hit: make its cluster the active one for this tab, then go to the resource.
    const open = async (h) => {
        const href = hitHref(h);
        if (!href) return;
        try {
            if (h.cluster !== currentName) await api.post(`/clusters/${encodeURIComponent(h.cluster)}/activate`);
        } catch (e) {
            setError(e?.response?.data?.error || e?.message || "Could not switch cluster");
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
                        aria-label="Search every cluster"
                        data-testid={FLEET.search}
                        value={q}
                        onChange={(e) => setQ(e.target.value)}
                        placeholder="Where is… search workloads, services and namespaces in every cluster"
                        className="bg-muted/30 pl-9"
                    />
                </div>
                <select aria-label="Kind" data-testid={FLEET.searchKind} className="select" value={kind} onChange={(e) => setKind(e.target.value)}>
                    <option value="">Everything</option>
                    <option value="Workload">Workloads</option>
                    <option value="Service">Services</option>
                    <option value="Namespace">Namespaces</option>
                </select>
            </div>
            {error ? <div className="notice danger" role="alert">{error}</div> : null}
            {res && (
                <>
                    {(skipped.length > 0 || errors.length > 0) && (
                        <div className="notice" role="status" data-testid={FLEET.searchSkipped}>
                            Not searched: {[...skipped.map((s) => `${s.cluster} (${s.reason})`), ...errors.map((e) => `${e.cluster} (${e.error})`)].join("; ")}
                        </div>
                    )}
                    {res.results.length === 0 ? (
                        <div className="empty" data-testid={FLEET.searchNone}>No matches in {res.clusters.searched} cluster{res.clusters.searched === 1 ? "" : "s"}.</div>
                    ) : (
                        <div className="max-h-[50vh] overflow-auto" tabIndex={0} role="region" aria-label="Search results" data-testid={FLEET.searchResults}>
                            <table className="tbl">
                                <thead>
                                    <tr>
                                        <th>Cluster</th>
                                        <th>Match</th>
                                        <th>Status</th>
                                        <th><span className="sr-only">Open</span></th>
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
                                                    Open
                                                </button>
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                    {res.truncated ? <p className="text-xs text-ink-3">Showing the first {res.results.length} of {res.total} matches — narrow the search.</p> : null}
                </>
            )}
            {busy && !res ? <Skeleton className="h-10 rounded-xl bg-muted" /> : null}
        </GlassCard>
    );
}

/** Every registered cluster at a glance: reachability, capacity and warnings, probed in the background. */
export default function Fleet() {
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
        const t = setInterval(tick, POLL_MS);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, [histWindow]);

    const load = useCallback(async (refresh) => {
        setRefreshing(true);
        try {
            const { data } = refresh ? await api.post("/fleet/refresh") : await api.get("/fleet");
            setFleet(data);
            setError("");
        } catch (e) {
            setError(e?.response?.data?.error || e?.message || "Could not load the fleet");
        } finally {
            setRefreshing(false);
        }
    }, []);

    useEffect(() => {
        load(false);
        const t = setInterval(() => load(false), POLL_MS);
        return () => clearInterval(t);
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
                title="Fleet"
                description="Every registered cluster, probed in the background: whether it answers, how many nodes are ready, and what needs attention. Register more under Settings → Clusters."
                action={
                    <button type="button" data-testid={FLEET.refresh} className="btn btn-line" onClick={() => load(true)} disabled={refreshing}>
                        <RefreshCw size={15} className={refreshing ? "animate-spin" : ""} /> {refreshing ? "Checking…" : "Check now"}
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
                        {stat("clusters", "Clusters", s.clusters)}
                        {stat("healthy", "Healthy", s.healthy)}
                        {stat("attention", "Need attention", attention)}
                        {stat("nodes", "Nodes ready", s.readyNodes, `/ ${s.nodes}`)}
                        {stat("pods", "Pods running", s.runningPods, `/ ${s.pods}`)}
                        {stat("warnings", "Warnings", s.warnings)}
                    </div>

                    <div className="flex items-center justify-end gap-2 text-xs text-ink-3">
                        <label htmlFor="fleet-history-window">History</label>
                        <select id="fleet-history-window" data-testid={H.window} className="select" value={histWindow} onChange={(e) => setHistWindow(e.target.value)}>
                            <option value="1h">Last hour</option>
                            <option value="6h">Last 6 hours</option>
                            <option value="24h">Last 24 hours</option>
                        </select>
                    </div>

                    <FleetSearch currentName={fleet.clusters.find((c) => c.current)?.name} />

                    {fleet.clusters.length === 0 ? (
                        <GlassCard className="p-6">
                            <div className="empty" data-testid={FLEET.empty}>
                                No clusters are registered.
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
                        Totals count the clusters that answered{fleet.checkedAt ? ` · last check ${relTime(fleet.checkedAt)}` : ""}
                        {fleet.intervalSeconds > 0 ? ` · checked every ${fleet.intervalSeconds}s` : " · background checks are off"}
                    </p>
                </>
            )}
        </div>
    );
}
