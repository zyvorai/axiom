import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { Network, Boxes, ArrowRightLeft } from "lucide-react";
import { NET } from "@/constants/testIds";

export default function Networking() {
    const [d, setD] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/network").then((r) => alive && setD(r.data));
        load();
        const t = setInterval(load, 8000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    if (!d) {
        return (
            <div data-testid={NET.root} className="space-y-4">
                <Skeleton className="h-40 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-96 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    return (
        <div data-testid={NET.root} className="space-y-4">
            <div className="flex items-end justify-between">
                <div>
                    <h1 className="text-3xl font-semibold tracking-tight">Networking</h1>
                    <p className="text-sm text-muted-foreground">
                        Service graph across pods, services and endpoints.
                    </p>
                </div>
                <div className="mono text-xs text-muted-foreground">namespace: {d.namespace}</div>
            </div>

            <div className="grid gap-4 md:grid-cols-3">
                {[
                    { label: "Graph nodes", value: d.nodes.length, icon: Network, tone: "text-[#6ab6f0]" },
                    { label: "Connections", value: d.edges.length, icon: ArrowRightLeft, tone: "text-[#93c88f]" },
                    {
                        label: "Services",
                        value: d.nodes.filter((n) => n.kind === "service").length,
                        icon: Boxes,
                        tone: "text-[#ff7550]",
                    },
                ].map((c) => (
                    <GlassCard key={c.label} className="p-4">
                        <div className="flex items-center justify-between">
                            <div className="text-xs uppercase tracking-widest text-muted-foreground">{c.label}</div>
                            <c.icon className={`h-4 w-4 ${c.tone}`} />
                        </div>
                        <div className="mt-2 text-2xl font-semibold tabular-nums">{c.value}</div>
                    </GlassCard>
                ))}
            </div>

            <div className="grid gap-4 xl:grid-cols-5">
                <GlassCard className="p-0 overflow-hidden xl:col-span-2">
                    <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">Graph nodes</div>
                    <div className="max-h-[520px] overflow-auto scrollbar-thin">
                        {d.nodes.map((n) => (
                            <div
                                key={n.id}
                                className="flex items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]"
                            >
                                <Network className="h-4 w-4 text-[#93c88f]" />
                                <div className="min-w-0 flex-1">
                                    <div className="mono truncate text-sm">{n.label}</div>
                                    <div className="text-[11px] text-muted-foreground">
                                        {n.kind} · {n.namespace}
                                    </div>
                                </div>
                                <div className="text-right">
                                    <div className="mono text-xs">{n.ip}</div>
                                    <div className="text-[11px] text-muted-foreground">{(n.ports || []).join(", ")}</div>
                                </div>
                            </div>
                        ))}
                    </div>
                </GlassCard>

                <GlassCard className="p-0 overflow-hidden xl:col-span-3">
                    <div className="flex items-center justify-between border-b border-white/5 px-4 py-3">
                        <div className="text-sm font-medium">Connections</div>
                        <span className="flex items-center gap-2 text-xs text-muted-foreground">
                            <span className="status-dot ok animate-pulse" /> live
                        </span>
                    </div>
                    <div className="max-h-[520px] overflow-auto scrollbar-thin">
                        {d.edges.map((e, i) => (
                            <div
                                key={i}
                                className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-2.5 text-xs last:border-none hover:bg-white/[0.03]"
                            >
                                <div className="col-span-5 mono truncate text-foreground">{e.from}</div>
                                <div className="col-span-1 text-center text-muted-foreground">→</div>
                                <div className="col-span-5 mono truncate text-foreground">{e.to}</div>
                                <div className="col-span-1 text-right text-muted-foreground">{e.label}</div>
                            </div>
                        ))}
                    </div>
                </GlassCard>
            </div>
        </div>
    );
}
