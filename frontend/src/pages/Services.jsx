import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
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
import { Search } from "lucide-react";

export default function Services() {
    const navigate = useNavigate();
    const [data, setData] = useState(null);
    const [q, setQ] = useState("");

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/services").then((r) => alive && setData(r.data));
        load();
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    const rows = useMemo(
        () => (data || []).filter((s) => q === "" || s.name.toLowerCase().includes(q.toLowerCase())),
        [data, q]
    );

    if (!data) {
        return (
            <div className="space-y-4">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="space-y-6">
            <PageHeader
                eyebrow="SERVICES"
                title="Services"
                description="ClusterIP, NodePort and LoadBalancer services across all namespaces."
            />

            <GlassCard className="p-3">
                <div className="relative">
                    <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <Input placeholder="Search service name" value={q} onChange={(e) => setQ(e.target.value)} className="bg-muted/30 pl-9" />
                </div>
            </GlassCard>

            <GlassCard className="overflow-hidden">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin">
                    <Table>
                        <TableHeader className="sticky top-0 z-10 backdrop-blur-xl bg-background/60">
                            <TableRow className="hover:bg-transparent">
                                <TableHead>Name</TableHead>
                                <TableHead>Namespace</TableHead>
                                <TableHead>Type</TableHead>
                                <TableHead>Cluster IP</TableHead>
                                <TableHead>Ports</TableHead>
                                <TableHead>Endpoints</TableHead>
                                <TableHead>Age</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((s) => (
                                <TableRow
                                    key={`${s.namespace}/${s.name}`}
                                    className="cursor-pointer hover:bg-muted"
                                    onClick={() => navigate(`/services/${encodeURIComponent(s.namespace)}/${encodeURIComponent(s.name)}`)}
                                >
                                    <TableCell className="font-medium">
                                        <Link
                                            to={`/services/${encodeURIComponent(s.namespace)}/${encodeURIComponent(s.name)}`}
                                            className="text-primary hover:underline"
                                            onClick={(e) => e.stopPropagation()}
                                        >
                                            {s.name}
                                        </Link>
                                    </TableCell>
                                    <TableCell className="mono text-xs">{s.namespace}</TableCell>
                                    <TableCell className="text-xs">{s.type}</TableCell>
                                    <TableCell className="mono text-xs">{s.clusterIP}</TableCell>
                                    <TableCell>
                                        <div className="flex flex-wrap gap-1">
                                            {(s.ports || []).map((p, i) => (
                                                <Badge key={i} variant="outline" className="mono text-[10px] bg-muted/30">
                                                    {p.port}
                                                    {p.targetPort ? `→${p.targetPort}` : ""}/{p.protocol || "TCP"}
                                                </Badge>
                                            ))}
                                        </div>
                                    </TableCell>
                                    <TableCell className="mono text-xs text-muted-foreground">{(s.endpoints || []).length}</TableCell>
                                    <TableCell className="mono text-xs">{s.age}</TableCell>
                                </TableRow>
                            ))}
                            {rows.length === 0 && (
                                <TableRow>
                                    <TableCell colSpan={7} className="py-10 text-center text-sm text-muted-foreground">
                                        No services match the current filters.
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
