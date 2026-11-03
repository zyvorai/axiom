import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { SeverityBadge, SourceBadge } from "@/components/SeverityBadge";
import { TIMELINE } from "@/constants/testIds";
import { relTime, timelineHref } from "@/lib/troubleshoot";

/**
 * "What changed" for one workload: its events, audit entries, alerts and rollout
 * revisions (and those of the ReplicaSets/pods it owns), newest first. Fully
 * self-contained so it can be dropped into any detail page as a tab body.
 */
export default function WorkloadTimeline({ namespace, kind, name }) {
    const [entries, setEntries] = useState(null);
    const [error, setError] = useState("");
    const object = `${kind}/${name}`;

    useEffect(() => {
        let alive = true;
        const load = () =>
            api
                .get("/timeline", { params: { namespace, object, limit: 100 } })
                .then((r) => {
                    if (!alive) return;
                    setEntries(r.data || []);
                    setError("");
                })
                .catch((e) => alive && setError(e?.response?.data?.error || e.message));
        load();
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, [namespace, object]);

    return (
        <GlassCard className="overflow-hidden p-0" data-testid={TIMELINE.workloadRoot}>
            <div className="flex items-center justify-between border-b border-border px-4 py-3">
                <div className="text-sm font-medium">What changed</div>
                <Link to={timelineHref({ namespace, object })} className="text-xs text-tone-link hover:underline">
                    Open in Timeline
                </Link>
            </div>
            {error && (
                <div className="notice danger m-3" role="alert">
                    {error}
                </div>
            )}
            {!entries && !error && <div className="px-4 py-8 text-center text-sm text-muted-foreground">Loading…</div>}
            {entries && entries.length === 0 && (
                <div className="px-4 py-8 text-center text-sm text-muted-foreground">No recorded changes for this workload.</div>
            )}
            {(entries || []).map((e, i) => (
                <div
                    key={`${e.time}-${i}`}
                    data-testid={TIMELINE.workloadRow(i)}
                    className="grid grid-cols-12 items-center gap-3 border-b border-border px-4 py-3 last:border-none"
                >
                    <div className="col-span-2 mono text-xs text-muted-foreground" title={new Date(e.time).toLocaleString()}>
                        {relTime(e.time)}
                    </div>
                    <div className="col-span-2">
                        <SourceBadge source={e.source} />
                    </div>
                    <div className="col-span-2">
                        <SeverityBadge severity={e.severity} />
                    </div>
                    <div className="col-span-6 text-sm text-foreground">
                        {e.summary}
                        {e.actor && <span className="text-xs text-muted-foreground"> · by {e.actor}</span>}
                    </div>
                </div>
            ))}
        </GlassCard>
    );
}
