import { useCallback, useEffect, useState } from "react";
import { RefreshCw, Server } from "lucide-react";
import { api } from "@/lib/api";
import { useCluster } from "@/lib/cluster";
import { relTime } from "@/lib/troubleshoot";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { FLEET } from "@/constants/testIds";

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

function ClusterCard({ c, onSwitch }) {
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

/** Every registered cluster at a glance: reachability, capacity and warnings, probed in the background. */
export default function Fleet() {
    const { activate } = useCluster();
    const [fleet, setFleet] = useState(null);
    const [error, setError] = useState("");
    const [refreshing, setRefreshing] = useState(false);

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

                    {fleet.clusters.length === 0 ? (
                        <GlassCard className="p-6">
                            <div className="empty" data-testid={FLEET.empty}>
                                No clusters are registered.
                            </div>
                        </GlassCard>
                    ) : (
                        <div className="grid gap-3.5 md:grid-cols-2 xl:grid-cols-3">
                            {fleet.clusters.map((c) => (
                                <ClusterCard key={c.name} c={c} onSwitch={activate} />
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
