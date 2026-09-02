import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import FeatureTile from "@/components/FeatureTile";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Cpu, MemoryStick, Boxes, Server, Ban, CheckCircle2, LogOut } from "lucide-react";
import { NODES } from "@/constants/testIds";
import { toast } from "sonner";

export default function Nodes() {
    const [nodes, setNodes] = useState(null);
    const [busy, setBusy] = useState("");

    const load = () => api.get("/nodes").then((r) => setNodes(r.data));

    useEffect(() => {
        let alive = true;
        const tick = () => api.get("/nodes").then((r) => alive && setNodes(r.data));
        tick();
        const t = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    const act = async (name, action, path) => {
        setBusy(`${name}:${action}`);
        try {
            await api.post(`/nodes/${encodeURIComponent(name)}/${path}`);
            toast.success(`${name}: ${action} requested`);
            await load();
        } catch (e) {
            toast.error(`${action} failed`, { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    if (!nodes) {
        return (
            <div data-testid={NODES.root} className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {[...Array(6)].map((_, i) => (
                    <Skeleton key={i} className="h-48 rounded-2xl bg-muted" />
                ))}
            </div>
        );
    }

    return (
        <div data-testid={NODES.root} className="space-y-6">
            <PageHeader
                eyebrow="NODES"
                title="Nodes"
                description="Compute topology, allocatable capacity and kubelet status."
                action={<div className="text-xs text-muted-foreground">{nodes.length} nodes total</div>}
            />

            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {nodes.map((n) => (
                    <GlassCard key={n.name} data-testid={NODES.row(n.name)} className="p-5">
                        <div className="flex items-start justify-between">
                            <div className="min-w-0">
                                <Link to={`/nodes/${encodeURIComponent(n.name)}`} className="mono truncate text-sm hover:underline">
                                    {n.name}
                                </Link>
                                <div className="mt-1 text-xs text-muted-foreground">
                                    {n.role} · {n.internalIp}
                                </div>
                            </div>
                            <StatusBadge status={n.status} />
                        </div>

                        <div className="mt-4 grid grid-cols-2 gap-3">
                            <FeatureTile nested size="compact" mono icon={Cpu} label="CPU" value={n.allocatableCpu} className="p-2.5" />
                            <FeatureTile nested size="compact" mono icon={MemoryStick} label="Memory" value={n.allocatableMemory} className="p-2.5" />
                            <FeatureTile nested size="compact" mono icon={Boxes} label="Pods" value={`${n.pods}/${n.allocatablePods}`} className="p-2.5" />
                            <FeatureTile nested size="compact" mono icon={Server} label="Version" value={<span className="truncate">{n.version}</span>} className="p-2.5" />
                        </div>

                        <div className="mt-4 grid grid-cols-2 gap-3 border-t pt-3 text-xs text-muted-foreground">
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

                        <div className="mt-4 flex gap-2 border-t pt-3">
                            {n.status === "SchedulingDisabled" ? (
                                <Button
                                    size="sm"
                                    variant="outline"
                                    disabled={busy === `${n.name}:uncordon`}
                                    onClick={() => act(n.name, "uncordon", "uncordon")}
                                    className="flex-1 rounded-xl bg-muted/30"
                                >
                                    <CheckCircle2 className="mr-1.5 h-3.5 w-3.5" /> Uncordon
                                </Button>
                            ) : (
                                <Button
                                    size="sm"
                                    variant="outline"
                                    disabled={busy === `${n.name}:cordon`}
                                    onClick={() => act(n.name, "cordon", "cordon")}
                                    className="flex-1 rounded-xl bg-muted/30"
                                >
                                    <Ban className="mr-1.5 h-3.5 w-3.5" /> Cordon
                                </Button>
                            )}
                            <Button
                                size="sm"
                                variant="outline"
                                disabled={busy === `${n.name}:drain`}
                                onClick={() => act(n.name, "drain", "drain")}
                                className="flex-1 rounded-xl border-rose-500/30 bg-rose-500/10 text-rose-300 hover:bg-rose-500/20"
                            >
                                <LogOut className="mr-1.5 h-3.5 w-3.5" /> Drain
                            </Button>
                        </div>
                    </GlassCard>
                ))}
            </div>
        </div>
    );
}
