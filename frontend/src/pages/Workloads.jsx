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
import { WORKLOADS } from "@/constants/testIds";

export default function Workloads() {
    const [data, setData] = useState(null);
    const [q, setQ] = useState("");
    const [ns, setNs] = useState("all");
    const [kind, setKind] = useState("all");

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/workloads").then((r) => alive && setData(r.data));
        load();
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    const { namespaces, kinds, rows } = useMemo(() => {
        if (!data) return { namespaces: [], kinds: [], rows: [] };
        const namespaces = [...new Set(data.map((w) => w.namespace))].sort();
        const kinds = [...new Set(data.map((w) => w.kind))].sort();
        const rows = data.filter(
            (w) =>
                (ns === "all" || w.namespace === ns) &&
                (kind === "all" || w.kind === kind) &&
                (q === "" ||
                    w.name.toLowerCase().includes(q.toLowerCase()) ||
                    w.image.toLowerCase().includes(q.toLowerCase()))
        );
        return { namespaces, kinds, rows };
    }, [data, q, ns, kind]);

    if (!data) {
        return (
            <div data-testid={WORKLOADS.root} className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-96 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    return (
        <div data-testid={WORKLOADS.root} className="space-y-4">
            <div className="flex items-end justify-between">
                <div>
                    <h1 className="text-3xl font-semibold tracking-tight">Workloads</h1>
                    <p className="text-sm text-muted-foreground">
                        Deployments, StatefulSets, DaemonSets, Pods and CronJobs.
                    </p>
                </div>
            </div>

            <GlassCard className="p-3">
                <div className="flex flex-wrap items-center gap-2">
                    <div className="relative flex-1 min-w-[220px]">
                        <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                        <Input
                            data-testid={WORKLOADS.search}
                            placeholder="Search name or image"
                            value={q}
                            onChange={(e) => setQ(e.target.value)}
                            className="border-white/10 bg-white/[0.03] pl-9"
                        />
                    </div>
                    <Select value={ns} onValueChange={setNs}>
                        <SelectTrigger data-testid={WORKLOADS.filterNs} className="w-[180px] border-white/10 bg-white/[0.03]">
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
                    <Select value={kind} onValueChange={setKind}>
                        <SelectTrigger data-testid={WORKLOADS.filterKind} className="w-[160px] border-white/10 bg-white/[0.03]">
                            <SelectValue placeholder="Kind" />
                        </SelectTrigger>
                        <SelectContent>
                            <SelectItem value="all">All kinds</SelectItem>
                            {kinds.map((k) => (
                                <SelectItem key={k} value={k}>
                                    {k}
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
                                <TableHead>Kind</TableHead>
                                <TableHead>Desired</TableHead>
                                <TableHead>Ready</TableHead>
                                <TableHead>Available</TableHead>
                                <TableHead>Status</TableHead>
                                <TableHead>Age</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((w) => (
                                <TableRow
                                    key={`${w.namespace}/${w.name}`}
                                    data-testid={WORKLOADS.row(w.name)}
                                    className="border-white/5 hover:bg-white/[0.03]"
                                >
                                    <TableCell>
                                        <div className="font-medium">{w.name}</div>
                                        <div className="mono truncate text-[11px] text-muted-foreground">{w.image}</div>
                                    </TableCell>
                                    <TableCell className="mono text-xs">{w.namespace}</TableCell>
                                    <TableCell className="text-xs">{w.kind}</TableCell>
                                    <TableCell className="mono tabular-nums">{w.desired}</TableCell>
                                    <TableCell className="mono tabular-nums">{w.ready}</TableCell>
                                    <TableCell className="mono tabular-nums">{w.available}</TableCell>
                                    <TableCell>
                                        <StatusBadge status={w.status} />
                                    </TableCell>
                                    <TableCell className="mono text-xs">{w.age}</TableCell>
                                </TableRow>
                            ))}
                            {rows.length === 0 && (
                                <TableRow>
                                    <TableCell colSpan={8} className="py-10 text-center text-sm text-muted-foreground">
                                        No workloads match the current filters.
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
