import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDistanceToNowStrict } from "date-fns";
import { EVENTS } from "@/constants/testIds";

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
            <div data-testid={EVENTS.root} className="space-y-4">
                <Skeleton className="h-8 w-64 rounded-lg bg-muted" />
                <Skeleton className="h-[70vh] rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div data-testid={EVENTS.root} className="space-y-6">
            <PageHeader
                eyebrow="EVENTS"
                title="Events"
                description="Kubernetes event stream, most recent first."
            />

            <GlassCard className="p-0 overflow-hidden">
                <div className="max-h-[70vh] overflow-auto scrollbar-thin">
                    {events.map((e, i) => (
                        <div
                            key={i}
                            data-testid={EVENTS.row(i)}
                            className="grid grid-cols-12 items-start gap-3 border-b px-4 py-3 last:border-none hover:bg-muted"
                        >
                            <div className="col-span-2">
                                <StatusBadge status={e.type} />
                            </div>
                            <div className="col-span-3 mono text-xs text-sky-300">{e.reason}</div>
                            <div className="col-span-5 text-sm text-foreground/90">{e.message}</div>
                            <div className="col-span-1 mono text-xs text-muted-foreground">×{e.count}</div>
                            <div className="col-span-1 mono text-right text-xs text-muted-foreground">{relTime(e.time)}</div>
                            <div className="col-span-12 mt-0.5 mono text-[11px] text-muted-foreground">
                                {e.namespace}/{e.object}
                            </div>
                        </div>
                    ))}
                    {events.length === 0 && (
                        <div className="py-10 text-center text-sm text-muted-foreground">No recent events.</div>
                    )}
                </div>
            </GlassCard>
        </div>
    );
}
