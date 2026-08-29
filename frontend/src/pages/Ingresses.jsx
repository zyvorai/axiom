import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import AutoTable from "@/components/AutoTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { Search, Lock, LockOpen, Waypoints, Router } from "lucide-react";

export default function Ingresses() {
    const [data, setData] = useState(null);
    const [gateways, setGateways] = useState(null);
    const [q, setQ] = useState("");

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/ingresses").then((r) => alive && setData(r.data));
        load();
        api.get("/gateways").then((r) => alive && setGateways(r.data || [])).catch(() => alive && setGateways([]));
        const t = setInterval(load, 15000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    const rows = useMemo(
        () => (data || []).filter((i) => q === "" || i.name.toLowerCase().includes(q.toLowerCase()) || (i.hosts || []).some((h) => h.toLowerCase().includes(q.toLowerCase()))),
        [data, q]
    );

    if (!data) {
        return (
            <div className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-96 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    return (
        <div className="space-y-4">
            <div className="flex items-end justify-between">
                <div>
                    <h1 className="text-3xl font-semibold tracking-tight">Ingresses</h1>
                    <p className="text-sm text-muted-foreground">External routes into cluster services, equivalent to OpenShift Routes.</p>
                </div>
            </div>

            <GlassCard className="p-3">
                <div className="relative">
                    <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <Input placeholder="Search by name or host" value={q} onChange={(e) => setQ(e.target.value)} className="border-white/10 bg-white/[0.03] pl-9" />
                </div>
            </GlassCard>

            <GlassCard className="p-0 overflow-hidden">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin">
                    <Table>
                        <TableHeader className="sticky top-0 z-10 backdrop-blur-xl bg-background/60">
                            <TableRow className="border-white/5 hover:bg-transparent">
                                <TableHead>Name</TableHead>
                                <TableHead>Namespace</TableHead>
                                <TableHead>Class</TableHead>
                                <TableHead>Hosts</TableHead>
                                <TableHead>Address</TableHead>
                                <TableHead>TLS</TableHead>
                                <TableHead>Age</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((i) => (
                                <TableRow key={`${i.namespace}/${i.name}`} className="border-white/5 hover:bg-white/[0.03]">
                                    <TableCell className="flex items-center gap-2 font-medium">
                                        <Waypoints className="h-3.5 w-3.5 text-i17-sky" /> {i.name}
                                    </TableCell>
                                    <TableCell className="mono text-xs">{i.namespace}</TableCell>
                                    <TableCell className="text-xs">{i.class || "—"}</TableCell>
                                    <TableCell>
                                        <div className="flex flex-wrap gap-1">
                                            {(i.hosts || []).map((h) => (
                                                <Badge key={h} variant="outline" className="mono text-[10px] border-white/10 bg-white/[0.03]">
                                                    {h}
                                                </Badge>
                                            ))}
                                        </div>
                                    </TableCell>
                                    <TableCell className="mono text-xs text-muted-foreground">{i.address || "—"}</TableCell>
                                    <TableCell>
                                        {i.tls ? (
                                            <Lock className="h-3.5 w-3.5 text-emerald-400" />
                                        ) : (
                                            <LockOpen className="h-3.5 w-3.5 text-muted-foreground" />
                                        )}
                                    </TableCell>
                                    <TableCell className="mono text-xs">{i.age}</TableCell>
                                </TableRow>
                            ))}
                            {rows.length === 0 && (
                                <TableRow>
                                    <TableCell colSpan={7} className="py-10 text-center text-sm text-muted-foreground">
                                        No ingresses match the current filters.
                                    </TableCell>
                                </TableRow>
                            )}
                        </TableBody>
                    </Table>
                </div>
            </GlassCard>

            <AutoTable
                title="Gateway API"
                icon={Router}
                items={gateways}
                emptyHint={gateways === null ? "Loading…" : "No Gateway API objects found (GatewayClass/Gateway/HTTPRoute)."}
            />
        </div>
    );
}
