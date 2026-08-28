import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Trash2 } from "lucide-react";
import { toast } from "sonner";

export default function Pods() {
    const [data, setData] = useState(null);
    const [q, setQ] = useState("");
    const [ns, setNs] = useState("all");

    const load = () => api.get("/pods").then((r) => setData(r.data));

    useEffect(() => {
        let alive = true;
        const tick = () => api.get("/pods").then((r) => alive && setData(r.data));
        tick();
        const t = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    const { namespaces, rows } = useMemo(() => {
        if (!data) return { namespaces: [], rows: [] };
        const namespaces = [...new Set(data.map((p) => p.namespace))].sort();
        const rows = data.filter(
            (p) =>
                (ns === "all" || p.namespace === ns) &&
                (q === "" || p.name.toLowerCase().includes(q.toLowerCase()))
        );
        return { namespaces, rows };
    }, [data, q, ns]);

    const evict = async (p) => {
        try {
            await api.post(`/pods/${p.namespace}/${encodeURIComponent(p.name)}/evict`);
            toast.success(`Evicted ${p.name}`);
            load();
        } catch (e) {
            toast.error("Evict failed", { description: e?.response?.data?.error || e.message });
        }
    };

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
                    <h1 className="text-3xl font-semibold tracking-tight">Pods</h1>
                    <p className="text-sm text-muted-foreground">Every running pod across all namespaces.</p>
                </div>
            </div>

            <GlassCard className="p-3">
                <div className="flex flex-wrap items-center gap-2">
                    <div className="relative flex-1 min-w-[220px]">
                        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                        <Input
                            placeholder="Search pod name"
                            value={q}
                            onChange={(e) => setQ(e.target.value)}
                            className="border-white/10 bg-white/[0.03] pl-9"
                        />
                    </div>
                    <Select value={ns} onValueChange={setNs}>
                        <SelectTrigger className="w-[180px] border-white/10 bg-white/[0.03]">
                            <SelectValue placeholder="Namespace" />
                        </SelectTrigger>
                        <SelectContent>
                            <SelectItem value="all">All namespaces</SelectItem>
                            {namespaces.map((n) => (
                                <SelectItem key={n} value={n}>
                                    {n}
                                </SelectItem>
                            ))}
                        </SelectContent>
                    </Select>
                </div>
            </GlassCard>

            <GlassCard className="p-0 overflow-hidden">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin">
                    <Table>
                        <TableHeader className="sticky top-0 z-10 backdrop-blur-xl bg-background/60">
                            <TableRow className="border-white/5 hover:bg-transparent">
                                <TableHead>Name</TableHead>
                                <TableHead>Namespace</TableHead>
                                <TableHead>Node</TableHead>
                                <TableHead>Ready</TableHead>
                                <TableHead>Restarts</TableHead>
                                <TableHead>Status</TableHead>
                                <TableHead>IP</TableHead>
                                <TableHead>Age</TableHead>
                                <TableHead />
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((p) => (
                                <TableRow key={`${p.namespace}/${p.name}`} className="border-white/5 hover:bg-white/[0.03]">
                                    <TableCell className="font-medium">{p.name}</TableCell>
                                    <TableCell className="mono text-xs">{p.namespace}</TableCell>
                                    <TableCell className="mono text-xs">{p.node}</TableCell>
                                    <TableCell className="mono">{p.ready}</TableCell>
                                    <TableCell className={`mono ${p.restarts > 0 ? "text-amber-400" : ""}`}>{p.restarts}</TableCell>
                                    <TableCell>
                                        <StatusBadge status={p.status} />
                                    </TableCell>
                                    <TableCell className="mono text-xs">{p.ip}</TableCell>
                                    <TableCell className="mono text-xs">{p.age}</TableCell>
                                    <TableCell>
                                        <Button size="sm" variant="ghost" onClick={() => evict(p)} className="text-muted-foreground hover:text-rose-400">
                                            <Trash2 className="h-3.5 w-3.5" />
                                        </Button>
                                    </TableCell>
                                </TableRow>
                            ))}
                            {rows.length === 0 && (
                                <TableRow>
                                    <TableCell colSpan={9} className="py-10 text-center text-sm text-muted-foreground">
                                        No pods match the current filters.
                                    </TableCell>
                                </TableRow>
                            )}
                        </TableBody>
                    </Table>
                </div>
            </GlassCard>
        </div>
    );
}
