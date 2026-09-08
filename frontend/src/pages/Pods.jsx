import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
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
import { Search, Terminal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Trash2 } from "lucide-react";
import { toast } from "sonner";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { useConfirm } from "@/components/ConfirmDialog";
import MacTerm from "@/components/MacTerm";

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
            <DialogContent className="glass max-w-lg">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">Exec · {pod.name}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    {containers.length > 1 && (
                        <Select value={container} onValueChange={setContainer}>
                            <SelectTrigger className="bg-muted/30">
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
                            className="mono bg-muted/30"
                            placeholder="ls -la"
                        />
                        <Button variant="brand" onClick={run} disabled={busy} className="shrink-0 rounded-xl">
                            {busy ? "Running…" : "Run"}
                        </Button>
                    </div>
                    <MacTerm
                        title={`exec — ${pod.name}`}
                        bodyClassName="h-64 overflow-auto whitespace-pre-wrap px-4 py-3 text-[12px] leading-[1.55] text-[#f1f1f0]"
                    >
                        {output || <span className="text-white/40">Output will appear here.</span>}
                    </MacTerm>
                </div>
            </DialogContent>
        </Dialog>
    );
}

export default function Pods() {
    const confirm = useConfirm();
    const [data, setData] = useState(null);
    const [q, setQ] = useState("");
    const [ns, setNs] = useState("all");
    const [execTarget, setExecTarget] = useState(null);
    const [selected, setSelected] = useState(new Set());
    const [bulkBusy, setBulkBusy] = useState(false);

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
        const ok = await confirm({
            title: `Evict pod ${p.name}?`,
            description: "This cannot be undone.",
            confirmLabel: "Evict",
        });
        if (!ok) return;
        try {
            await api.post(`/pods/${p.namespace}/${encodeURIComponent(p.name)}/evict`);
            toast.success(`Evicted ${p.name}`);
            load();
        } catch (e) {
            toast.error("Evict failed", { description: e?.response?.data?.error || e.message });
        }
    };

    const key = (p) => `${p.namespace}/${p.name}`;
    const toggle = (p, checked) => {
        setSelected((s) => {
            const next = new Set(s);
            if (checked) next.add(key(p));
            else next.delete(key(p));
            return next;
        });
    };
    const toggleAll = (checked) => setSelected(checked ? new Set(rows.map(key)) : new Set());

    const bulkEvict = async () => {
        const targets = rows.filter((p) => selected.has(key(p)));
        // rows reflects the last 10s poll, which can be stale relative to
        // `selected` — a selected pod may have already been evicted/deleted
        // elsewhere in that window. Surface the gap instead of silently
        // acting on fewer pods than the user actually selected.
        const skipped = selected.size - targets.length;
        const ok = await confirm({
            title: `Evict ${targets.length} pod${targets.length === 1 ? "" : "s"}?`,
            description: "This cannot be undone.",
            confirmLabel: "Evict",
        });
        if (!ok) return;
        setBulkBusy(true);
        try {
            const results = await Promise.allSettled(
                targets.map((p) => api.post(`/pods/${p.namespace}/${encodeURIComponent(p.name)}/evict`))
            );
            const failed = results.filter((r) => r.status === "rejected").length;
            const skippedNote = skipped > 0 ? ` (${skipped} selected pod${skipped === 1 ? "" : "s"} no longer listed, skipped)` : "";
            if (failed) toast.error(`${failed} of ${targets.length} evictions failed${skippedNote}`);
            else toast.success(`Evicted ${targets.length} pod${targets.length === 1 ? "" : "s"}${skippedNote}`);
            setSelected(new Set());
            load();
        } finally {
            setBulkBusy(false);
        }
    };

    if (!data) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="mt-4 h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title="Pods"
                description="Every running pod across all namespaces."
                action={
                    selected.size > 0 && (
                        <div className="flex items-center gap-3 border-y border-border px-3 py-1.5">
                            <span className="text-xs text-muted-foreground">{selected.size} selected</span>
                            <Button size="sm" variant="outline" disabled={bulkBusy} onClick={bulkEvict} className="h-7 rounded-lg border-rose-500/30 bg-rose-500/10 text-xs text-rose-300 hover:bg-rose-500/20">
                                <Trash2 className="mr-1 h-3 w-3" /> {bulkBusy ? "Evicting…" : "Evict"}
                            </Button>
                        </div>
                    )
                }
                tools={
                    <>
                        <div className="input">
                            <Search size={15} style={{ color: "var(--ink-3)" }} />
                            <input
                                placeholder="Search pod name"
                                value={q}
                                onChange={(e) => setQ(e.target.value)}
                                style={{ background: "transparent", border: "none", outline: "none", color: "inherit", flex: 1, font: "inherit" }}
                            />
                        </div>
                        <Select value={ns} onValueChange={setNs}>
                            <SelectTrigger className="w-[180px] bg-muted/30">
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
                    </>
                }
            />

            <GlassCard className="overflow-hidden">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th className="w-10">
                                    <Checkbox
                                        checked={rows.length > 0 && selected.size === rows.length}
                                        onCheckedChange={toggleAll}
                                    />
                                </th>
                                <th>Name</th>
                                <th>Namespace</th>
                                <th>Node</th>
                                <th>Ready</th>
                                <th>Restarts</th>
                                <th>Status</th>
                                <th>IP</th>
                                <th>Age</th>
                                <th />
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((p) => (
                                <tr key={`${p.namespace}/${p.name}`}>
                                    <td>
                                        <Checkbox checked={selected.has(key(p))} onCheckedChange={(v) => toggle(p, v)} />
                                    </td>
                                    <td className="name">
                                        <Link to={`/pods/${p.namespace}/${encodeURIComponent(p.name)}`} className="hover:underline">
                                            {p.name}
                                        </Link>
                                    </td>
                                    <td className="mono">{p.namespace}</td>
                                    <td className="mono">{p.node}</td>
                                    <td className="mono num">{p.ready}</td>
                                    <td className={`mono num ${p.restarts > 0 ? "text-amber-400" : ""}`}>{p.restarts}</td>
                                    <td>
                                        <StatusBadge status={p.status} />
                                    </td>
                                    <td className="mono">{p.ip}</td>
                                    <td className="mono">{p.age}</td>
                                    <td>
                                        <div style={{ display: "flex", alignItems: "center", justifyContent: "flex-end", gap: 6 }}>
                                            <button type="button" className="btn btn-line btn-sm" title="Exec" onClick={() => setExecTarget(p)}>
                                                <Terminal size={13} />
                                            </button>
                                            <button type="button" className="btn btn-line btn-sm" title="Evict" onClick={() => evict(p)}>
                                                <Trash2 size={13} />
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                            {rows.length === 0 && (
                                <tr>
                                    <td colSpan={10}>
                                        <div className="empty">No pods match the current filters.</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>

            {execTarget && <ExecDialog pod={execTarget} onOpenChange={(o) => !o && setExecTarget(null)} />}
        </div>
    );
}
