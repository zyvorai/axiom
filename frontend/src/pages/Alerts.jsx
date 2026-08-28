import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { formatDistanceToNowStrict } from "date-fns";
import { AlertTriangle, ListChecks } from "lucide-react";

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true });
    } catch {
        return iso;
    }
}

export default function Alerts() {
    const [d, setD] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/alerts").then((r) => alive && setD(r.data));
        load();
        const t = setInterval(load, 8000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    if (!d) {
        return (
            <div className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-64 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    const warnings = d.warnings || [];
    const rules = d.rules || [];

    return (
        <div className="space-y-4">
            <div>
                <h1 className="text-3xl font-semibold tracking-tight">Alerts</h1>
                <p className="text-sm text-muted-foreground">Active warnings and Prometheus alerting rule groups.</p>
            </div>

            <GlassCard className="p-0 overflow-hidden">
                <div className="flex items-center gap-2 border-b border-white/5 px-4 py-3 text-sm font-medium">
                    <AlertTriangle className="h-4 w-4 text-amber-500" /> Active warnings
                </div>
                <div className="max-h-[400px] overflow-auto scrollbar-thin">
                    {warnings.length === 0 && <div className="px-4 py-8 text-center text-sm text-muted-foreground">No active warnings.</div>}
                    {warnings.map((w, i) => (
                        <div key={i} className="flex items-start gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]">
                            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
                            <div className="min-w-0 flex-1">
                                <div className="text-sm">
                                    <span className="mono text-xs text-sky-300">{w.reason}</span> — {w.message}
                                </div>
                                <div className="mt-0.5 text-[11px] text-muted-foreground">
                                    {w.namespace}/{w.object} · ×{w.count}
                                </div>
                            </div>
                            <div className="text-[11px] text-muted-foreground">{relTime(w.time)}</div>
                        </div>
                    ))}
                </div>
            </GlassCard>

            <GlassCard className="p-0 overflow-hidden">
                <div className="flex items-center gap-2 border-b border-white/5 px-4 py-3 text-sm font-medium">
                    <ListChecks className="h-4 w-4 text-[#93c88f]" /> Alerting rule groups
                </div>
                <div className="max-h-[300px] overflow-auto scrollbar-thin">
                    {rules.length === 0 && <div className="px-4 py-8 text-center text-sm text-muted-foreground">No alerting rules configured.</div>}
                    {rules.map((r, i) => (
                        <div key={i} className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]">
                            <div className="col-span-6 min-w-0">
                                <div className="mono truncate text-sm">{r.name}</div>
                                <div className="text-[11px] text-muted-foreground">{r.namespace}</div>
                            </div>
                            <div className="col-span-4 text-xs text-muted-foreground">{r.groups} groups</div>
                            <div className="col-span-2 text-right text-xs text-muted-foreground">{r.age}</div>
                        </div>
                    ))}
                </div>
            </GlassCard>
        </div>
    );
}
