import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { KeySquare, Search, Trash2 } from "lucide-react";
import { toast } from "sonner";

export default function Secrets() {
    const [data, setData] = useState(null);
    const [q, setQ] = useState("");

    const load = () => api.get("/secrets").then((r) => setData(r.data));

    useEffect(() => {
        let alive = true;
        api.get("/secrets").then((r) => alive && setData(r.data));
        return () => {
            alive = false;
        };
    }, []);

    const rows = useMemo(
        () => (data || []).filter((s) => q === "" || s.name.toLowerCase().includes(q.toLowerCase())),
        [data, q]
    );

    const remove = async (s) => {
        try {
            await api.delete(`/secrets/${s.namespace}/${encodeURIComponent(s.name)}`);
            toast.success(`Deleted ${s.name}`);
            load();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
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
                    <h1 className="text-3xl font-semibold tracking-tight">Secrets</h1>
                    <p className="text-sm text-muted-foreground">Opaque, TLS and registry secrets across all namespaces.</p>
                </div>
            </div>

            <GlassCard className="p-3">
                <div className="relative">
                    <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <Input placeholder="Search secret name" value={q} onChange={(e) => setQ(e.target.value)} className="border-white/10 bg-white/[0.03] pl-9" />
                </div>
            </GlassCard>

            <GlassCard className="p-0 overflow-hidden">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin">
                    <Table>
                        <TableHeader className="sticky top-0 z-10 backdrop-blur-xl bg-background/60">
                            <TableRow className="border-white/5 hover:bg-transparent">
                                <TableHead>Name</TableHead>
                                <TableHead>Namespace</TableHead>
                                <TableHead>Type</TableHead>
                                <TableHead>Keys</TableHead>
                                <TableHead>Age</TableHead>
                                <TableHead />
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((s) => (
                                <TableRow key={`${s.namespace}/${s.name}`} className="border-white/5 hover:bg-white/[0.03]">
                                    <TableCell className="flex items-center gap-2 font-medium">
                                        <KeySquare className="h-3.5 w-3.5 text-[#b49bd8]" /> {s.name}
                                    </TableCell>
                                    <TableCell className="mono text-xs">{s.namespace}</TableCell>
                                    <TableCell className="mono text-xs">{s.type}</TableCell>
                                    <TableCell>
                                        <div className="flex flex-wrap gap-1">
                                            {(s.keys || []).map((k) => (
                                                <Badge key={k} variant="outline" className="mono text-[10px] border-white/10 bg-white/[0.03]">
                                                    {k}
                                                </Badge>
                                            ))}
                                        </div>
                                    </TableCell>
                                    <TableCell className="mono text-xs">{s.age}</TableCell>
                                    <TableCell>
                                        <Button size="sm" variant="ghost" onClick={() => remove(s)} className="text-muted-foreground hover:text-rose-400">
                                            <Trash2 className="h-3.5 w-3.5" />
                                        </Button>
                                    </TableCell>
                                </TableRow>
                            ))}
                            {rows.length === 0 && (
                                <TableRow>
                                    <TableCell colSpan={6} className="py-10 text-center text-sm text-muted-foreground">
                                        No secrets match the current filters.
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
