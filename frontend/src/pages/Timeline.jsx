import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { RefreshCw, Search, X } from "lucide-react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { SeverityBadge, SourceBadge } from "@/components/SeverityBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { TIMELINE } from "@/constants/testIds";
import { objectHref, relTime } from "@/lib/troubleshoot";

const SOURCES = ["event", "audit", "alert", "rollout"];
const SINCE = [
    { value: "", label: "All time" },
    { value: "1h", label: "Last hour" },
    { value: "6h", label: "Last 6 hours" },
    { value: "24h", label: "Last 24 hours" },
    { value: "7d", label: "Last 7 days" },
];

/** One merged feed of Kubernetes events, Axiom audit entries, alerts and rollouts. */
export default function Timeline() {
    const [params, setParams] = useSearchParams();
    const namespace = params.get("namespace") || "";
    const object = params.get("object") || "";
    const [since, setSince] = useState("");
    const [sources, setSources] = useState(() => new Set(SOURCES));
    const [q, setQ] = useState("");
    const [entries, setEntries] = useState(null);
    const [error, setError] = useState("");
    const [namespaces, setNamespaces] = useState([]);

    const setParam = (key, value) => {
        const next = new URLSearchParams(params);
        if (value) next.set(key, value);
        else next.delete(key);
        setParams(next, { replace: true });
    };

    useEffect(() => {
        let alive = true;
        api
            .get("/namespaces")
            .then((r) => alive && setNamespaces((r.data || []).map((n) => n.name)))
            .catch(() => {});
        return () => {
            alive = false;
        };
    }, []);

    const load = useCallback(() => {
        const query = { limit: 500 };
        if (namespace) query.namespace = namespace;
        if (object) query.object = object;
        if (since) query.since = since;
        return api
            .get("/timeline", { params: query })
            .then((r) => {
                setEntries(r.data || []);
                setError("");
            })
            .catch((e) => setError(e?.response?.data?.error || e.message));
    }, [namespace, object, since]);

    useEffect(() => {
        load();
        const t = setInterval(load, 10000);
        return () => clearInterval(t);
    }, [load]);

    const rows = useMemo(() => {
        const needle = q.trim().toLowerCase();
        return (entries || []).filter(
            (e) =>
                sources.has(e.source) &&
                (!needle || `${e.summary} ${e.object} ${e.namespace} ${e.actor || ""}`.toLowerCase().includes(needle))
        );
    }, [entries, sources, q]);

    const toggleSource = (s) =>
        setSources((prev) => {
            const next = new Set(prev);
            if (next.has(s)) next.delete(s);
            else next.add(s);
            return next;
        });

    return (
        <div data-testid={TIMELINE.root} className="page">
            <PageHeader
                title="Timeline"
                description="Everything that changed, in one feed: Kubernetes events, Axiom audit entries, alerts and rollout revisions."
                action={
                    <button type="button" data-testid={TIMELINE.refresh} className="btn btn-line" onClick={load}>
                        <RefreshCw size={15} /> Refresh
                    </button>
                }
                tools={
                    <>
                        <div className="input">
                            <Search size={15} className="text-ink-3" aria-hidden="true" />
                            <input
                                data-testid={TIMELINE.search}
                                aria-label="Search the timeline"
                                placeholder="Search summary, object or user"
                                value={q}
                                onChange={(e) => setQ(e.target.value)}
                                className="bare-input"
                            />
                        </div>
                        <select
                            data-testid={TIMELINE.namespace}
                            aria-label="Namespace"
                            className="select"
                            value={namespace}
                            onChange={(e) => setParam("namespace", e.target.value)}
                        >
                            <option value="">All namespaces</option>
                            {namespaces.map((n) => (
                                <option key={n} value={n}>
                                    {n}
                                </option>
                            ))}
                        </select>
                        <select
                            data-testid={TIMELINE.since}
                            aria-label="Time range"
                            className="select"
                            value={since}
                            onChange={(e) => setSince(e.target.value)}
                        >
                            {SINCE.map((s) => (
                                <option key={s.value} value={s.value}>
                                    {s.label}
                                </option>
                            ))}
                        </select>
                        <div className="seg" role="group" aria-label="Sources">
                            {SOURCES.map((s) => (
                                <button
                                    key={s}
                                    type="button"
                                    data-testid={TIMELINE.source(s)}
                                    aria-pressed={sources.has(s)}
                                    onClick={() => toggleSource(s)}
                                >
                                    {s}
                                </button>
                            ))}
                        </div>
                        {object && (
                            <span className="pill soft" data-testid={TIMELINE.objectChip}>
                                {object}
                                <button type="button" aria-label="Clear object filter" onClick={() => setParam("object", "")} className="ml-1">
                                    <X size={12} />
                                </button>
                            </span>
                        )}
                    </>
                }
            />

            {error && (
                <div className="notice danger" role="alert">
                    {error}
                </div>
            )}

            {!entries && !error ? (
                <Skeleton className="h-[60vh] rounded-2xl bg-muted" />
            ) : (
                <GlassCard className="overflow-hidden p-0">
                    <div className="max-h-[70vh] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label="Timeline entries">
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>When</th>
                                    <th>Source</th>
                                    <th>Severity</th>
                                    <th>Object</th>
                                    <th>What happened</th>
                                    <th>By</th>
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map((e, i) => {
                                    const href = e.object ? objectHref(e.namespace, e.object) : null;
                                    return (
                                        <tr key={`${e.time}-${i}`} data-testid={TIMELINE.row(i)}>
                                            <td className="mono num" title={new Date(e.time).toLocaleString()}>
                                                {relTime(e.time)}
                                            </td>
                                            <td>
                                                <SourceBadge source={e.source} />
                                            </td>
                                            <td>
                                                <SeverityBadge severity={e.severity} />
                                            </td>
                                            <td className="mono">
                                                {e.object ? (
                                                    href ? (
                                                        <Link to={href} className="text-tone-link hover:underline">
                                                            {e.namespace ? `${e.namespace}/` : ""}
                                                            {e.object}
                                                        </Link>
                                                    ) : (
                                                        `${e.namespace ? `${e.namespace}/` : ""}${e.object}`
                                                    )
                                                ) : (
                                                    <span className="text-ink-3">cluster</span>
                                                )}
                                            </td>
                                            <td>{e.summary}</td>
                                            <td className="mono">{e.actor || ""}</td>
                                        </tr>
                                    );
                                })}
                                {rows.length === 0 && (
                                    <tr>
                                        <td colSpan={6}>
                                            <div className="empty" data-testid={TIMELINE.empty}>
                                                Nothing matches these filters.
                                            </div>
                                        </td>
                                    </tr>
                                )}
                            </tbody>
                        </table>
                    </div>
                </GlassCard>
            )}
        </div>
    );
}
