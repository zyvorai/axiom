import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import ApplyManifestDialog from "@/components/ApplyManifestDialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { Network, Boxes, ArrowRightLeft, Radar, ShieldHalf, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { NET } from "@/constants/testIds";

const NETPOL_TEMPLATE = `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: example-policy
  namespace: default
spec:
  podSelector: {}
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector: {}
`;

function NetworkPolicyPanel() {
    const [policies, setPolicies] = useState(null);
    const [createOpen, setCreateOpen] = useState(false);

    const load = () => api.get("/networkpolicies").then((r) => setPolicies(r.data || []));

    useEffect(() => {
        let alive = true;
        api.get("/networkpolicies").then((r) => alive && setPolicies(r.data || []));
        return () => {
            alive = false;
        };
    }, []);

    const remove = async (p) => {
        try {
            await api.delete(`/networkpolicies/${p.namespace}/${encodeURIComponent(p.name)}`);
            toast.success(`Deleted ${p.name}`);
            load();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };

    if (!policies) return <Skeleton className="h-40 rounded-2xl bg-white/[0.04]" />;

    return (
        <GlassCard className="p-0 overflow-hidden">
            <div className="flex items-center justify-between border-b border-white/5 px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                    <ShieldHalf className="h-4 w-4 text-i17-cosmic" /> NetworkPolicies
                </div>
                <Button size="sm" variant="outline" onClick={() => setCreateOpen(true)} className="h-7 rounded-lg border-white/10 bg-white/[0.03] text-xs">
                    <Plus className="mr-1 h-3 w-3" /> Create via YAML
                </Button>
            </div>
            <div className="max-h-[360px] overflow-auto scrollbar-thin">
                {policies.length === 0 && <div className="px-4 py-8 text-center text-sm text-muted-foreground">No NetworkPolicies configured.</div>}
                {policies.map((p, i) => (
                    <div key={i} className="flex items-center justify-between border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]">
                        <div className="min-w-0 flex-1">
                            <div className="mono truncate text-sm">{p.name}</div>
                            <div className="text-[11px] text-muted-foreground">
                                {p.namespace} · {(p.policyTypes || []).join(", ") || "—"}
                            </div>
                        </div>
                        <div className="mono max-w-[280px] truncate text-xs text-muted-foreground">{p.podSelector || "all pods"}</div>
                        <Button size="sm" variant="ghost" onClick={() => remove(p)} className="ml-3 h-7 px-2 text-muted-foreground hover:text-rose-400">
                            <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                    </div>
                ))}
            </div>
            <ApplyManifestDialog
                open={createOpen}
                onOpenChange={setCreateOpen}
                title="Create NetworkPolicy"
                initialManifest={NETPOL_TEMPLATE}
                onApplied={() => {
                    setCreateOpen(false);
                    load();
                }}
            />
        </GlassCard>
    );
}

function flowItems(flows) {
    if (!flows) return null;
    if (Array.isArray(flows)) return flows;
    if (Array.isArray(flows.items)) return flows.items;
    if (Array.isArray(flows.flows)) return flows.flows;
    return null;
}

function PacketWolfPanel() {
    const [status, setStatus] = useState(null);
    const [flows, setFlows] = useState(null);
    const [flowsError, setFlowsError] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/packetwolf/status").then((r) => alive && setStatus(r.data));
        load();
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    useEffect(() => {
        if (!status?.reachable) return;
        let alive = true;
        const load = () =>
            api
                .get("/packetwolf/proxy/api/v1/flows", { params: { limit: 25 } })
                .then((r) => alive && (setFlows(r.data), setFlowsError(null)))
                .catch((e) => alive && setFlowsError(e?.response?.data?.error || e.message));
        load();
        const t = setInterval(load, 8000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, [status?.reachable]);

    if (!status) return <Skeleton className="h-24 rounded-2xl bg-white/[0.04]" />;

    const tone = status.reachable ? "ok" : status.configured ? "warn" : "info";
    const label = status.reachable ? "Connected" : status.configured ? "Unreachable" : "Not configured";
    const items = flowItems(flows);

    return (
        <GlassCard className="p-0 overflow-hidden">
            <div className="flex items-center justify-between border-b border-white/5 px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                    <Radar className="h-4 w-4 text-i17-cosmic" /> PacketWolf
                </div>
                <div className="flex items-center gap-2">
                    <span className={`status-dot ${tone}`} />
                    <span className="text-xs text-muted-foreground">{label}</span>
                </div>
            </div>
            <div className="px-4 py-3 text-xs text-muted-foreground">
                {status.message || status.baseUrl || "eBPF flow visibility via the PacketWolf integration."}
            </div>
            {status.reachable && (
                <div className="max-h-[360px] overflow-auto scrollbar-thin border-t border-white/5">
                    {flowsError && <div className="px-4 py-6 text-center text-sm text-muted-foreground">{flowsError}</div>}
                    {!flowsError && items && items.length > 0 && (
                        <Table>
                            <TableHeader className="sticky top-0 z-10 backdrop-blur-xl bg-background/60">
                                <TableRow className="border-white/5 hover:bg-transparent">
                                    <TableHead>Source</TableHead>
                                    <TableHead>Destination</TableHead>
                                    <TableHead>Proto</TableHead>
                                    <TableHead>Verdict</TableHead>
                                </TableRow>
                            </TableHeader>
                            <TableBody>
                                {items.slice(0, 25).map((f, i) => (
                                    <TableRow key={i} className="border-white/5 hover:bg-white/[0.03]">
                                        <TableCell className="mono text-xs">{String(f.source ?? f.src ?? f.from ?? "—")}</TableCell>
                                        <TableCell className="mono text-xs">{String(f.destination ?? f.dst ?? f.to ?? "—")}</TableCell>
                                        <TableCell className="text-xs">{String(f.protocol ?? f.proto ?? "—")}</TableCell>
                                        <TableCell className="text-xs">{String(f.verdict ?? f.action ?? f.disposition ?? "—")}</TableCell>
                                    </TableRow>
                                ))}
                            </TableBody>
                        </Table>
                    )}
                    {!flowsError && items && items.length === 0 && (
                        <div className="px-4 py-6 text-center text-sm text-muted-foreground">No flows in the current window.</div>
                    )}
                    {!flowsError && flows && !items && (
                        <pre className="mono whitespace-pre-wrap px-4 py-3 text-[11px] text-muted-foreground">
                            {JSON.stringify(flows, null, 2)}
                        </pre>
                    )}
                </div>
            )}
        </GlassCard>
    );
}

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
                    { label: "Graph nodes", value: d.nodes.length, icon: Network, tone: "text-i17-sky" },
                    { label: "Connections", value: d.edges.length, icon: ArrowRightLeft, tone: "text-i17-sage" },
                    {
                        label: "Services",
                        value: d.nodes.filter((n) => n.kind === "service").length,
                        icon: Boxes,
                        tone: "text-i17-cosmic",
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
                                <Network className="h-4 w-4 text-i17-sage" />
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

            <PacketWolfPanel />
            <NetworkPolicyPanel />
        </div>
    );
}
