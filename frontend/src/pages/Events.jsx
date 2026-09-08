import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDistanceToNowStrict } from "date-fns";
import { EVENTS } from "@/constants/testIds";
import { resourceHref } from "@/lib/resourceHref";

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true });
    } catch {
        return iso;
    }
}

export default function Events() {
    const [events, setEvents] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/events").then((r) => alive && setEvents(r.data));
        load();
        const t = setInterval(load, 8000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

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
                title="Events"
                description="Kubernetes event stream, most recent first."
            />

            <GlassCard className="overflow-hidden">
                <div style={{ overflowX: "auto" }} className="max-h-[70vh] overflow-y-auto scrollbar-thin">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Type</th>
                                <th>Reason</th>
                                <th>Message</th>
                                <th>Object</th>
                                <th>Count</th>
                                <th>Age</th>
                            </tr>
                        </thead>
                        <tbody>
                            {events.map((e, i) => {
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
                                                <Link to={href} className="text-primary hover:underline">
                                                    {e.namespace}/{e.object}
                                                </Link>
                                            ) : (
                                                `${e.namespace}/${e.object}`
                                            )}
                                        </td>
                                        <td className="num">×{e.count}</td>
                                        <td className="mono num">{relTime(e.time)}</td>
                                    </tr>
                                );
                            })}
                            {events.length === 0 && (
                                <tr>
                                    <td colSpan={6}>
                                        <div className="empty">No recent events.</div>
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
