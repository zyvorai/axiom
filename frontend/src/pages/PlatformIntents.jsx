import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";

export default function PlatformIntents() {
    const [items, setItems] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/platform-intents").then((r) => alive && setItems(r.data));
        load();
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    if (!items) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title="Platform Intents"
                description="Workloads requested via natural-language intent (Workloads → Run workload → Intent)."
            />

            <GlassCard className="overflow-hidden">
                <div style={{ overflowX: "auto" }} className="max-h-[65vh] overflow-y-auto scrollbar-thin">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Namespace</th>
                                <th>Kind</th>
                                <th>Image</th>
                                <th>Resources</th>
                                <th>Status</th>
                                <th>Age</th>
                            </tr>
                        </thead>
                        <tbody>
                            {items.map((it, i) => (
                                <tr key={i}>
                                    <td className="name mono">
                                        {it.name}
                                        {it.message && <small>{it.message}</small>}
                                    </td>
                                    <td className="mono">{it.namespace}</td>
                                    <td>{it.workloadKind}</td>
                                    <td className="mono">{it.image}</td>
                                    <td className="num">
                                        {it.replicas ? `${it.replicas}x ` : ""}
                                        {it.cpu} {it.memory}
                                    </td>
                                    <td>
                                        <StatusBadge status={it.phase} />
                                    </td>
                                    <td className="num">{it.age}</td>
                                </tr>
                            ))}
                            {items.length === 0 && (
                                <tr>
                                    <td colSpan={7}>
                                        <div className="empty">No platform intents yet.</div>
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
