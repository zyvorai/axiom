import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
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
import { Badge } from "@/components/ui/badge";
import { KeySquare, Search, Trash2, Plus, X, RefreshCcw, Link2 } from "lucide-react";
import { toast } from "sonner";
import { useConfirm } from "@/components/ConfirmDialog";

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
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>Create secret</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Namespace</Label>
                            <Input value={namespace} onChange={(e) => setNamespace(e.target.value)} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Name</Label>
                            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="app-config" className="bg-muted/30" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Type</Label>
                        <Select value={type} onValueChange={setType}>
                            <SelectTrigger className="bg-muted/30">
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
                                    <Input value={p.key} onChange={(e) => setPair(i, { key: e.target.value })} placeholder="KEY" className="bg-muted/30" />
                                    <Input value={p.value} onChange={(e) => setPair(i, { value: e.target.value })} placeholder="value" className="bg-muted/30" />
                                    <button aria-label="Remove row" title="Remove row" onClick={() => removePair(i)} className="shrink-0 text-muted-foreground hover:text-tone-red">
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
        <GlassCard className="overflow-hidden p-0">
            <div className="flex items-center gap-2 border-b px-4 py-3 text-sm font-medium">
                <Link2 className="h-4 w-4 text-tone-link" /> External Secrets
            </div>
            <div className="max-h-[300px] overflow-auto scrollbar-thin">
                {!items && <Skeleton className="m-4 h-16 rounded-xl bg-muted" />}
                {items && (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Store</th>
                                <th>Refresh</th>
                                <th>Status</th>
                                <th>Age</th>
                            </tr>
                        </thead>
                        <tbody>
                            {items.map((s, i) => (
                                <tr key={i}>
                                    <td className="name">
                                        {s.name}
                                        <small>{s.namespace}</small>
                                    </td>
                                    <td className="mono">{s.store}</td>
                                    <td>{s.refresh}</td>
                                    <td>
                                        <StatusBadge status={s.ready ? "Ready" : s.status} />
                                    </td>
                                    <td className="mono">{s.age}</td>
                                </tr>
                            ))}
                            {items.length === 0 && (
                                <tr>
                                    <td colSpan={5}>
                                        <div className="empty">No ExternalSecret resources found.</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                )}
            </div>
        </GlassCard>
    );
}

export default function Secrets() {
    const confirm = useConfirm();
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
        const ok = await confirm({
            title: `Delete secret ${s.name}?`,
            description: "This also removes all its keys. This cannot be undone.",
            confirmLabel: "Delete",
        });
        if (!ok) return;
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
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="mt-4 h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title="Secrets"
                description="Opaque, TLS and registry secrets across all namespaces."
                action={
                    <>
                        <button type="button" className="btn btn-line" onClick={load}>
                            <RefreshCcw size={15} /> Refresh
                        </button>
                        <button type="button" className="btn btn-fill" onClick={() => setCreateOpen(true)}>
                            <Plus size={15} /> Create secret
                        </button>
                    </>
                }
                tools={
                    <div className="input">
                        <Search size={15} className="text-ink-3" />
                        <input
                            placeholder="Search secret name"
                            value={q}
                            onChange={(e) => setQ(e.target.value)} className="bare-input"
                        />
                    </div>
                }
            />

            <GlassCard className="overflow-hidden p-0">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Namespace</th>
                                <th>Type</th>
                                <th>Keys</th>
                                <th>Age</th>
                                <th />
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((s) => (
                                <tr key={`${s.namespace}/${s.name}`}>
                                    <td className="name">
                                        <span className="flex gap-2 items-center">
                                            <KeySquare size={14} className="text-ink-3" />
                                            <Link
                                                to={`/secrets/${encodeURIComponent(s.namespace)}/${encodeURIComponent(s.name)}`}
                                                className="hover:underline"
                                                title={s.name}
                                            >
                                                {s.name}
                                            </Link>
                                        </span>
                                    </td>
                                    <td className="mono">{s.namespace}</td>
                                    <td className="mono">{s.type}</td>
                                    <td>
                                        <div className="flex flex-wrap gap-1">
                                            {(s.keys || []).map((k) => (
                                                <Badge key={k} variant="outline" className="mono text-[10px] bg-muted/30">
                                                    {k}
                                                </Badge>
                                            ))}
                                        </div>
                                    </td>
                                    <td className="mono">{s.age}</td>
                                    <td>
                                        <button type="button" className="btn btn-line btn-sm" aria-label={`Delete secret ${s.name}`} title={`Delete secret ${s.name}`} onClick={() => remove(s)}>
                                            <Trash2 size={13} />
                                        </button>
                                    </td>
                                </tr>
                            ))}
                            {rows.length === 0 && (
                                <tr>
                                    <td colSpan={6}>
                                        <div className="empty">No secrets match the current filters.</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>

            <ExternalSecretsSection />

            <CreateSecretDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
        </div>
    );
}
