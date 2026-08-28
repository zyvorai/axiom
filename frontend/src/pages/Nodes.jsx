import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Cpu, MemoryStick, Boxes, Server } from "lucide-react";
import { NODES } from "@/constants/testIds";

export default function Nodes() {
    const [nodes, setNodes] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/nodes").then((r) => alive && setNodes(r.data));
        load();
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    if (!nodes) {
        return (
            <div data-testid={NODES.root} className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {[...Array(6)].map((_, i) => (
                    <Skeleton key={i} className="h-48 rounded-2xl bg-white/[0.04]" />
                ))}
            </div>
        );
    }

    return (
        <div data-testid={NODES.root} className="space-y-4">
            <div className="flex items-end justify-between">
                <div>
                    <h1 className="text-3xl font-semibold tracking-tight">Nodes</h1>
                    <p className="text-sm text-muted-foreground">
                        Compute topology, allocatable capacity and kubelet status.
                    </p>
                </div>
                <div className="text-xs text-muted-foreground">{nodes.length} nodes total</div>
            </div>

            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {nodes.map((n) => (
                    <GlassCard key={n.name} data-testid={NODES.row(n.name)} className="p-5">
                        <div className="flex items-start justify-between">
                            <div className="min-w-0">
                                <div className="mono truncate text-sm">{n.name}</div>
                                <div className="mt-1 text-xs text-muted-foreground">
                                    {n.role} · {n.internalIp}
                                </div>
                            </div>
                            <StatusBadge status={n.status} />
                        </div>

                        <div className="mt-4 grid grid-cols-2 gap-3 text-xs">
                            <div className="rounded-lg border border-white/5 bg-white/[0.02] p-2.5">
                                <div className="flex items-center gap-1 text-[10px] uppercase tracking-widest text-muted-foreground">
                                    <Cpu className="h-3 w-3" /> CPU
                                </div>
                                <div className="mono mt-1 text-sm">{n.allocatableCpu}</div>
                            </div>
                            <div className="rounded-lg border border-white/5 bg-white/[0.02] p-2.5">
                                <div className="flex items-center gap-1 text-[10px] uppercase tracking-widest text-muted-foreground">
                                    <MemoryStick className="h-3 w-3" /> Memory
                                </div>
                                <div className="mono mt-1 text-sm">{n.allocatableMemory}</div>
                            </div>
                            <div className="rounded-lg border border-white/5 bg-white/[0.02] p-2.5">
                                <div className="flex items-center gap-1 text-[10px] uppercase tracking-widest text-muted-foreground">
                                    <Boxes className="h-3 w-3" /> Pods
                                </div>
                                <div className="mono mt-1 text-sm">
                                    {n.pods}/{n.allocatablePods}
                                </div>
                            </div>
                            <div className="rounded-lg border border-white/5 bg-white/[0.02] p-2.5">
                                <div className="flex items-center gap-1 text-[10px] uppercase tracking-widest text-muted-foreground">
                                    <Server className="h-3 w-3" /> Version
                                </div>
                                <div className="mono mt-1 truncate text-sm">{n.version}</div>
                            </div>
                        </div>

                        <div className="mt-4 grid grid-cols-2 gap-3 border-t border-white/5 pt-3 text-xs text-muted-foreground">
                            <div>
                                <div className="text-[10px] uppercase tracking-widest">OS</div>
                                <div className="mt-0.5 text-foreground">{n.osImage}</div>
                            </div>
                            <div>
                                <div className="text-[10px] uppercase tracking-widest">Kernel</div>
                                <div className="mono mt-0.5 text-foreground">{n.kernelVersion}</div>
                            </div>
                            <div className="col-span-2">
                                <div className="text-[10px] uppercase tracking-widest">Runtime</div>
                                <div className="mono mt-0.5 truncate text-foreground">{n.containerRuntime}</div>
                            </div>
                        </div>
                    </GlassCard>
                ))}
            </div>
        </div>
    );
}
