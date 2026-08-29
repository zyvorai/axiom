import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
import {
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
} from "@/components/ui/select";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import {
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableHeader,
    TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { KeySquare, Search, Trash2, Plus, X, RefreshCcw, Link2 } from "lucide-react";
import { toast } from "sonner";

const SECRET_TYPES = ["Opaque", "kubernetes.io/tls", "kubernetes.io/dockerconfigjson", "kubernetes.io/basic-auth"];

function CreateSecretDialog({ open, onOpenChange, onCreated }) {
    const [namespace, setNamespace] = useState("default");
    const [name, setName] = useState("");
    const [type, setType] = useState("Opaque");
    const [pairs, setPairs] = useState([{ key: "", value: "" }]);
    const [busy, setBusy] = useState(false);

    const setPair = (i, patch) => setPairs((p) => p.map((row, idx) => (idx === i ? { ...row, ...patch } : row)));
    const addPair = () => setPairs((p) => [...p, { key: "", value: "" }]);
    const removePair = (i) => setPairs((p) => p.filter((_, idx) => idx !== i));

    const submit = async () => {
        if (!name) return;
        const data = {};
        for (const { key, value } of pairs) if (key) data[key] = value;
        setBusy(true);
        try {
            await api.post("/secrets", { namespace, name, type, data });
            toast.success(`Secret ${name} created`);
            onOpenChange(false);
            setName("");
            setPairs([{ key: "", value: "" }]);
            onCreated();
        } catch (e) {
            toast.error("Create failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="glass max-w-md border-white/10">
                <DialogHeader>
                    <DialogTitle>Create secret</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Namespace</Label>
                            <Input value={namespace} onChange={(e) => setNamespace(e.target.value)} className="border-white/10 bg-white/[0.03]" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Name</Label>
                            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="app-config" className="border-white/10 bg-white/[0.03]" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Type</Label>
                        <Select value={type} onValueChange={setType}>
                            <SelectTrigger className="border-white/10 bg-white/[0.03]">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                {SECRET_TYPES.map((t) => (
                                    <SelectItem key={t} value={t}>
                                        {t}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Data</Label>
                        <div className="space-y-2">
                            {pairs.map((p, i) => (
                                <div key={i} className="flex items-center gap-1.5">
                                    <Input value={p.key} onChange={(e) => setPair(i, { key: e.target.value })} placeholder="KEY" className="border-white/10 bg-white/[0.03]" />
                                    <Input value={p.value} onChange={(e) => setPair(i, { value: e.target.value })} placeholder="value" className="border-white/10 bg-white/[0.03]" />
                                    <button onClick={() => removePair(i)} className="shrink-0 text-muted-foreground hover:text-rose-400">
                                        <X className="h-4 w-4" />
                                    </button>
                                </div>
                            ))}
                        </div>
                        <Button size="sm" variant="ghost" onClick={addPair} className="mt-1 text-xs text-muted-foreground">
                            <Plus className="mr-1 h-3.5 w-3.5" /> Add key
                        </Button>
                    </div>
                </div>
                <DialogFooter>
                    <Button onClick={submit} disabled={busy || !name} className="rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white">
                        {busy ? "Creating…" : "Create"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function ExternalSecretsSection() {
    const [items, setItems] = useState(null);

    useEffect(() => {
        let alive = true;
        api.get("/external-secrets").then((r) => alive && setItems(r.data)).catch(() => alive && setItems([]));
        return () => {
            alive = false;
        };
    }, []);

    return (
        <GlassCard className="p-0 overflow-hidden">
            <div className="flex items-center gap-2 border-b border-white/5 px-4 py-3 text-sm font-medium">
                <Link2 className="h-4 w-4 text-[#6ab6f0]" /> External Secrets
            </div>
            <div className="max-h-[300px] overflow-auto scrollbar-thin">
                {!items && <Skeleton className="m-4 h-16 rounded-xl bg-white/[0.04]" />}
                {items && items.length === 0 && (
                    <div className="px-4 py-8 text-center text-sm text-muted-foreground">No ExternalSecret resources found.</div>
                )}
                {items &&
                    items.map((s, i) => (
                        <div key={i} className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]">
                            <div className="col-span-5 min-w-0">
                                <div className="mono truncate text-sm">{s.name}</div>
                                <div className="text-[11px] text-muted-foreground">{s.namespace}</div>
                            </div>
                            <div className="col-span-3 mono truncate text-xs text-muted-foreground">{s.store}</div>
                            <div className="col-span-2 text-xs text-muted-foreground">{s.refresh}</div>
                            <div className="col-span-1">
                                <StatusBadge status={s.ready ? "Ready" : s.status} />
                            </div>
                            <div className="col-span-1 text-right text-xs text-muted-foreground">{s.age}</div>
                        </div>
                    ))}
            </div>
        </GlassCard>
    );
}

export default function Secrets() {
    const [data, setData] = useState(null);
    const [q, setQ] = useState("");
    const [createOpen, setCreateOpen] = useState(false);

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
                <div className="flex gap-2">
                    <Button size="sm" variant="outline" onClick={load} className="rounded-xl border-white/10 bg-white/[0.03]">
                        <RefreshCcw className="mr-1.5 h-3.5 w-3.5" /> Refresh
                    </Button>
                    <Button size="sm" onClick={() => setCreateOpen(true)} className="rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white">
                        <Plus className="mr-1.5 h-3.5 w-3.5" /> Create secret
                    </Button>
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
                                    <TableCell className="flex max-w-xs items-center gap-2 font-medium">
                                        <KeySquare className="h-3.5 w-3.5 shrink-0 text-[#b49bd8]" />
                                        <span className="min-w-0 truncate" title={s.name}>{s.name}</span>
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

            <ExternalSecretsSection />

            <CreateSecretDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
        </div>
    );
}
