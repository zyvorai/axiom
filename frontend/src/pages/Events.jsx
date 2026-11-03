import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDistanceToNowStrict } from "date-fns";
import { EVENTS, TIMELINE, AI } from "@/constants/testIds";
import { ExplainWithAiButton } from "@/components/AiAssistantDrawer";
import { canExplainEvent } from "@/lib/ai";
import { resourceHref } from "@/lib/resourceHref";
import SearchBox from "@/components/SearchBox";
import NamespaceFilter from "@/components/NamespaceFilter";
import SavedViews from "@/components/SavedViews";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useUrlState } from "@/lib/urlState";
import { useNamespaceFilter } from "@/lib/namespace";
import { History } from "lucide-react";
import { timelineHref } from "@/lib/troubleshoot";
import { useTranslation } from "react-i18next";
import { dateFnsLocale } from "@/lib/i18n";

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true, locale: dateFnsLocale() });
    } catch {
        return iso;
    }
}

export default function Events() {
    const { t } = useTranslation();
    const [events, setEvents] = useState(null);
    const [q, setQ] = useUrlState("q", "");
    const [type, setType] = useUrlState("type", "all");
    const [ns, setNs, knownNamespaces, globalNs] = useNamespaceFilter();

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/events").then((r) => alive && setEvents(r.data));
        load();
        const timer = setInterval(load, 8000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    const rows = useMemo(() => {
        const needle = q.toLowerCase();
        return (events || []).filter(
            (e) =>
                (ns === "all" || e.namespace === ns) &&
                (type === "all" || e.type === type) &&
                (needle === "" ||
                    [e.reason, e.message, e.object].some((f) => String(f || "").toLowerCase().includes(needle)))
        );
    }, [events, q, ns, type]);
    const namespaces = useMemo(
        () => [...new Set([...knownNamespaces, ...(events || []).map((e) => e.namespace).filter(Boolean)])].sort(),
        [knownNamespaces, events]
    );

    if (!events) {
        return (
            <div data-testid={EVENTS.root} className="page">
                <Skeleton className="h-8 w-64 rounded-lg bg-muted" />
                <Skeleton className="h-[70vh] rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div data-testid={EVENTS.root} className="page">
            <PageHeader
                title={t("pages:events.title")}
                description={t("pages:events.description")}
                tools={
                    <>
                        <SearchBox placeholder={t("pages:events.search")} value={q} onChange={setQ} />
                        <NamespaceFilter value={ns} onChange={setNs} namespaces={namespaces} />
                        <Select value={type} onValueChange={setType}>
                            <SelectTrigger aria-label={t("pages:events.filterType")} className="w-[150px] bg-muted/30">
                                <SelectValue placeholder={t("common:fields.type")} />
                            </SelectTrigger>
                            <SelectContent>
                                <SelectItem value="all">{t("pages:events.allTypes")}</SelectItem>
                                <SelectItem value="Warning">{t("common:status.Warning")}</SelectItem>
                                <SelectItem value="Normal">{t("common:status.Normal")}</SelectItem>
                            </SelectContent>
                        </Select>
                        <SavedViews
                            pageKey="events"
                            filters={{ q, ns, type }}
                            onApply={(f) => {
                                setQ(f.q ?? "");
                                setNs(f.ns ?? globalNs);
                                setType(f.type ?? "all");
                            }}
                        />
                    </>
                }
                action={
                    <Link to="/timeline" data-testid={TIMELINE.eventsLink} className="btn btn-line">
                        <History size={15} /> {t("pages:events.openTimeline")}
                    </Link>
                }
            />

            <GlassCard className="overflow-hidden p-0">
                <div className="max-h-[70vh] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:events.title")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.type")}</th>
                                <th>{t("common:fields.reason")}</th>
                                <th>{t("common:fields.message")}</th>
                                <th>{t("common:fields.object")}</th>
                                <th>{t("common:fields.count")}</th>
                                <th>{t("common:fields.age")}</th>
                                <th><span className="sr-only">{t("common:fields.actions")}</span></th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((e, i) => {
                                const href = resourceHref(e.namespace, e.object);
                                return (
                                    <tr key={i} data-testid={EVENTS.row(i)}>
                                        <td>
                                            <StatusBadge status={e.type} />
                                        </td>
                                        <td className="mono">{e.reason}</td>
                                        <td>{e.message}</td>
                                        <td className="mono">
                                            {href ? (
                                                <Link to={href} className="text-tone-link hover:underline">
                                                    {e.namespace}/{e.object}
                                                </Link>
                                            ) : (
                                                `${e.namespace}/${e.object}`
                                            )}{" "}
                                            <Link
                                                to={timelineHref({ namespace: e.namespace, object: e.object })}
                                                aria-label={t("pages:events.whatChangedFor", { object: e.object })}
                                                title={t("pages:events.whatChanged")}
                                                className="text-ink-3 hover:text-ink"
                                            >
                                                <History size={12} className="inline" aria-hidden="true" />
                                            </Link>
                                        </td>
                                        <td className="num">×{e.count}</td>
                                        <td className="mono num">{relTime(e.time)}</td>
                                        <td>
                                            {canExplainEvent(e) && (
                                                <ExplainWithAiButton
                                                    testId={AI.explainEvent(i)}
                                                    target={{ namespace: e.namespace, kind: "event", name: e.object, reason: e.reason, label: `${e.reason}: ${e.namespace}/${e.object}` }}
                                                />
                                            )}
                                        </td>
                                    </tr>
                                );
                            })}
                            {rows.length === 0 && (
                                <tr>
                                    <td colSpan={7}>
                                        <div className="empty">{events.length ? t("pages:events.noMatches") : t("pages:events.empty")}</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>
        </div>
    );
}
