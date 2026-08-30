import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Button } from "@/components/ui/button";
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
import { FileCog, Search, Trash2, Plus, X } from "lucide-react";
import { toast } from "sonner";
import { useConfirm } from "@/components/ConfirmDialog";

function CreateConfigMapDialog({ open, onOpenChange, onCreated }) {
    const [namespace, setNamespace] = useState("default");
    const [name, setName] = useState("");
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
            await api.post("/configmaps", { namespace, name, data });
            toast.success(`ConfigMap ${name} created`);
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
                    <DialogTitle>Create ConfigMap</DialogTitle>
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
                    <Button variant="brand" onClick={submit} disabled={busy || !name} className="rounded-xl">
                        {busy ? "Creating…" : "Create"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

export default function ConfigMaps() {
    const confirm = useConfirm();
    const [data, setData] = useState(null);
    const [q, setQ] = useState("");
    const [createOpen, setCreateOpen] = useState(false);

    const load = () => api.get("/configmaps").then((r) => setData(r.data));

    useEffect(() => {
        let alive = true;
        api.get("/configmaps").then((r) => alive && setData(r.data));
        return () => {
            alive = false;
        };
    }, []);

    const rows = useMemo(
        () => (data || []).filter((c) => q === "" || c.name.toLowerCase().includes(q.toLowerCase())),
        [data, q]
    );

    const remove = async (c) => {
        const ok = await confirm({
            title: `Delete ConfigMap ${c.name}?`,
            description: "This cannot be undone.",
            confirmLabel: "Delete",
        });
        if (!ok) return;
        try {
            await api.delete(`/configmaps/${c.namespace}/${encodeURIComponent(c.name)}`);
            toast.success(`Deleted ${c.name}`);
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
        <div className="space-y-6">
            <PageHeader
                eyebrow="CONFIGMAPS"
                title="ConfigMaps"
                description="Application configuration across all namespaces."
                action={
                    <Button size="sm" variant="brand" onClick={() => setCreateOpen(true)} className="rounded-xl">
                        <Plus className="mr-1.5 h-3.5 w-3.5" /> Create ConfigMap
                    </Button>
                }
            />

            <GlassCard className="p-3">
                <div className="relative">
                    <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <Input placeholder="Search ConfigMap name" value={q} onChange={(e) => setQ(e.target.value)} className="border-white/10 bg-white/[0.03] pl-9" />
                </div>
            </GlassCard>

            <GlassCard className="p-0 overflow-hidden">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin">
                    <Table>
                        <TableHeader className="sticky top-0 z-10 backdrop-blur-xl bg-background/60">
                            <TableRow className="border-white/5 hover:bg-transparent">
                                <TableHead>Name</TableHead>
                                <TableHead>Namespace</TableHead>
                                <TableHead>Keys</TableHead>
                                <TableHead>Age</TableHead>
                                <TableHead />
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {rows.map((c) => (
                                <TableRow key={`${c.namespace}/${c.name}`} className="border-white/5 hover:bg-white/[0.03]">
                                    <TableCell className="flex max-w-xs items-center gap-2 font-medium">
                                        <FileCog className="h-3.5 w-3.5 shrink-0 text-i17-sky" />
                                        <span className="min-w-0 truncate" title={c.name}>{c.name}</span>
                                    </TableCell>
                                    <TableCell className="mono text-xs">{c.namespace}</TableCell>
                                    <TableCell>
                                        <div className="flex flex-wrap gap-1">
                                            {(c.keys || []).map((k) => (
                                                <Badge key={k} variant="outline" className="mono text-[10px] border-white/10 bg-white/[0.03]">
                                                    {k}
                                                </Badge>
                                            ))}
                                        </div>
                                    </TableCell>
                                    <TableCell className="mono text-xs">{c.age}</TableCell>
                                    <TableCell>
                                        <Button size="sm" variant="ghost" onClick={() => remove(c)} className="text-muted-foreground hover:text-rose-400">
                                            <Trash2 className="h-3.5 w-3.5" />
                                        </Button>
                                    </TableCell>
                                </TableRow>
                            ))}
                            {rows.length === 0 && (
                                <TableRow>
                                    <TableCell colSpan={5} className="py-10 text-center text-sm text-muted-foreground">
                                        No ConfigMaps match the current filters.
                                    </TableCell>
                                </TableRow>
                            )}
                        </TableBody>
                    </Table>
                </div>
            </GlassCard>

            <CreateConfigMapDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
        </div>
    );
}
