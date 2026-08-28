import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
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
import { Search, Terminal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Trash2 } from "lucide-react";
import { toast } from "sonner";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";

function ExecDialog({ pod, onOpenChange }) {
    const containers = pod.containers && pod.containers.length ? pod.containers : [""];
    const [container, setContainer] = useState(containers[0]);
    const [command, setCommand] = useState("ls -la");
    const [output, setOutput] = useState("");
    const [busy, setBusy] = useState(false);

    const run = async () => {
        setBusy(true);
        setOutput("");
        try {
            const { data } = await api.post(`/pods/${pod.namespace}/${encodeURIComponent(pod.name)}/exec`, {
                container,
                command,
            });
            setOutput(data.output || "");
        } catch (e) {
            setOutput(e?.response?.data?.error || e.message);
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-lg border-white/10">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">Exec · {pod.name}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    {containers.length > 1 && (
                        <Select value={container} onValueChange={setContainer}>
                            <SelectTrigger className="border-white/10 bg-white/[0.03]">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                {containers.map((c) => (
                                    <SelectItem key={c} value={c}>
                                        {c}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    )}
                    <div className="flex gap-2">
                        <Input
                            value={command}
                            onChange={(e) => setCommand(e.target.value)}
                            onKeyDown={(e) => e.key === "Enter" && run()}
                            className="mono border-white/10 bg-white/[0.03]"
                            placeholder="ls -la"
                        />
                        <Button onClick={run} disabled={busy} className="shrink-0 rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white">
                            {busy ? "Running…" : "Run"}
                        </Button>
                    </div>
                    <pre className="mono h-64 overflow-auto whitespace-pre-wrap rounded-lg border border-white/10 bg-black/40 p-3 text-[12px] text-foreground/90">
                        {output || "Output will appear here."}
                    </pre>
                </div>
            </DialogContent>
        </Dialog>
    );
}

export default function Pods() {
    const [data, setData] = useState(null);
    const [q, setQ] = useState("");
    const [ns, setNs] = useState("all");
    const [execTarget, setExecTarget] = useState(null);

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
                                    <TableCell className="font-medium">
                                        <Link to={`/pods/${p.namespace}/${encodeURIComponent(p.name)}`} className="hover:underline">
                                            {p.name}
                                        </Link>
                                    </TableCell>
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
                                        <div className="flex items-center gap-1">
                                            <Button size="sm" variant="ghost" onClick={() => setExecTarget(p)} className="h-7 w-7 p-0 text-muted-foreground hover:text-foreground">
                                                <Terminal className="h-3.5 w-3.5" />
                                            </Button>
                                            <Button size="sm" variant="ghost" onClick={() => evict(p)} className="h-7 w-7 p-0 text-muted-foreground hover:text-rose-400">
                                                <Trash2 className="h-3.5 w-3.5" />
                                            </Button>
                                        </div>
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

            {execTarget && <ExecDialog pod={execTarget} onOpenChange={(o) => !o && setExecTarget(null)} />}
        </div>
    );
}
