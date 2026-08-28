import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Sparkles } from "lucide-react";

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
            <div className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-64 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    return (
        <div className="space-y-4">
            <div>
                <h1 className="text-3xl font-semibold tracking-tight">Platform Intents</h1>
                <p className="text-sm text-muted-foreground">
                    Workloads requested via natural-language intent (Workloads → Run workload → Intent).
                </p>
            </div>

            <GlassCard className="p-0 overflow-hidden">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin">
                    {items.length === 0 && (
                        <div className="flex flex-col items-center gap-2 px-4 py-16 text-center text-sm text-muted-foreground">
                            <Sparkles className="h-5 w-5" />
                            No platform intents yet.
                        </div>
                    )}
                    {items.map((it, i) => (
                        <div
                            key={i}
                            className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]"
                        >
                            <div className="col-span-3 min-w-0">
                                <div className="mono truncate text-sm">{it.name}</div>
                                <div className="text-[11px] text-muted-foreground">{it.namespace}</div>
                            </div>
                            <div className="col-span-2 text-xs text-muted-foreground">{it.workloadKind}</div>
                            <div className="col-span-2 mono text-xs text-muted-foreground truncate">{it.image}</div>
                            <div className="col-span-2 text-xs text-muted-foreground">
                                {it.replicas ? `${it.replicas}x` : ""} {it.cpu} {it.memory}
                            </div>
                            <div className="col-span-2">
                                <StatusBadge status={it.phase} />
                            </div>
                            <div className="col-span-1 text-right text-xs text-muted-foreground">{it.age}</div>
                            {it.message && <div className="col-span-12 mt-1 text-[11px] text-muted-foreground">{it.message}</div>}
                        </div>
                    ))}
                </div>
            </GlassCard>
        </div>
    );
}
