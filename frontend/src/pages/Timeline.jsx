import { useCallback, useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { RefreshCw, Search, X } from "lucide-react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import RecentChanges from "@/components/RecentChanges";
import PageHeader from "@/components/PageHeader";
import { SeverityBadge, SourceBadge } from "@/components/SeverityBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { TIMELINE } from "@/constants/testIds";
import { objectHref, relTime } from "@/lib/troubleshoot";
import { useTranslation } from "react-i18next";
import { formatDate } from "@/lib/i18n";

const SOURCES = ["event", "audit", "alert", "rollout"];
const sinceOptions = (t) => [
    { value: "", label: t("pages:timeline.since.all") },
    { value: "1h", label: t("pages:timeline.since.hour") },
    { value: "6h", label: t("pages:timeline.since.sixHours") },
    { value: "24h", label: t("pages:timeline.since.day") },
    { value: "7d", label: t("pages:timeline.since.week") },
];

function sourceLabel(t, s) {
    if (s === "event") return t("pages:timeline.sources.event");
    if (s === "audit") return t("pages:timeline.sources.audit");
    if (s === "alert") return t("pages:timeline.sources.alert");
    if (s === "rollout") return t("pages:timeline.sources.rollout");
    return s;
}

/** One merged feed of Kubernetes events, Axiom audit entries, alerts and rollouts. */
export default function Timeline() {
    const { t } = useTranslation();
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
        const timer = setInterval(load, 10000);
        return () => clearInterval(timer);
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
                title={t("pages:timeline.title")}
                description={t("pages:timeline.description")}
                action={
                    <button type="button" data-testid={TIMELINE.refresh} className="btn btn-line" onClick={load}>
                        <RefreshCw size={15} /> {t("common:actions.refresh")}
                    </button>
                }
                tools={
                    <>
                        <div className="input">
                            <Search size={15} className="text-ink-3" aria-hidden="true" />
                            <input
                                data-testid={TIMELINE.search}
                                aria-label={t("pages:timeline.searchLabel")}
                                placeholder={t("pages:timeline.searchPlaceholder")}
                                value={q}
                                onChange={(e) => setQ(e.target.value)}
                                className="bare-input"
                            />
                        </div>
                        <select
                            data-testid={TIMELINE.namespace}
                            aria-label={t("common:fields.namespace")}
                            className="select"
                            value={namespace}
                            onChange={(e) => setParam("namespace", e.target.value)}
                        >
                            <option value="">{t("pages:timeline.allNamespaces")}</option>
                            {namespaces.map((n) => (
                                <option key={n} value={n}>
                                    {n}
                                </option>
                            ))}
                        </select>
                        <select
                            data-testid={TIMELINE.since}
                            aria-label={t("pages:timeline.timeRange")}
                            className="select"
                            value={since}
                            onChange={(e) => setSince(e.target.value)}
                        >
                            {sinceOptions(t).map((s) => (
                                <option key={s.value} value={s.value}>
                                    {s.label}
                                </option>
                            ))}
                        </select>
                        <div className="seg" role="group" aria-label={t("pages:timeline.sourcesLabel")}>
                            {SOURCES.map((s) => (
                                <button
                                    key={s}
                                    type="button"
                                    data-testid={TIMELINE.source(s)}
                                    aria-pressed={sources.has(s)}
                                    onClick={() => toggleSource(s)}
                                >
                                    {sourceLabel(t, s)}
                                </button>
                            ))}
                        </div>
                        {object && (
                            <span className="pill soft" data-testid={TIMELINE.objectChip}>
                                {object}
                                <button type="button" aria-label={t("pages:timeline.clearObject")} onClick={() => setParam("object", "")} className="ml-1">
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

            <RecentChanges namespace={namespace} workload={object.includes("/") ? object.split("/")[1] : ""} />

            {!entries && !error ? (
                <Skeleton className="h-[60vh] rounded-2xl bg-muted" />
            ) : (
                <GlassCard className="overflow-hidden p-0">
                    <div className="max-h-[70vh] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:timeline.entries")}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("pages:timeline.columns.when")}</th>
                                    <th>{t("common:fields.source")}</th>
                                    <th>{t("pages:timeline.columns.severity")}</th>
                                    <th>{t("common:fields.object")}</th>
                                    <th>{t("pages:timeline.columns.what")}</th>
                                    <th>{t("pages:timeline.columns.by")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map((e, i) => {
                                    const href = e.object ? objectHref(e.namespace, e.object) : null;
                                    return (
                                        <tr key={`${e.time}-${i}`} data-testid={TIMELINE.row(i)}>
                                            <td className="mono num" title={formatDate(e.time)}>
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
                                                    <span className="text-ink-3">{t("pages:timeline.clusterScope")}</span>
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
                                                {t("pages:timeline.empty")}
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
