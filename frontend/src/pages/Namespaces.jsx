import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { Boxes, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useConfirm } from "@/components/ConfirmDialog";

export default function Namespaces() {
    const confirm = useConfirm();
    const [items, setItems] = useState(null);
    const [open, setOpen] = useState(false);
    const [name, setName] = useState("");
    const [busy, setBusy] = useState(false);

    const load = () => api.get("/namespaces").then((r) => setItems(r.data));

    useEffect(() => {
        let alive = true;
        api.get("/namespaces").then((r) => alive && setItems(r.data));
        return () => {
            alive = false;
        };
    }, []);

    const create = async () => {
        if (!name) return;
        setBusy(true);
        try {
            await api.post("/namespaces", { name });
            toast.success(`Namespace ${name} created`);
            setOpen(false);
            setName("");
            load();
        } catch (e) {
            toast.error("Create failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    const remove = async (n) => {
        const ok = await confirm({
            title: `Delete namespace ${n.name}?`,
            description: "This deletes every resource in this namespace. This cannot be undone.",
            confirmLabel: "Delete",
        });
        if (!ok) return;
        try {
            await api.delete(`/namespaces/${encodeURIComponent(n.name)}`);
            toast.success(`Deleted ${n.name}`);
            load();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };

    if (!items) {
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
                    <h1 className="text-3xl font-semibold tracking-tight">Namespaces</h1>
                    <p className="text-sm text-muted-foreground">Every namespace in the cluster, with pod and service counts.</p>
                </div>
                <Button size="sm" variant="brand" onClick={() => setOpen(true)} className="rounded-xl">
                    <Plus className="mr-1.5 h-3.5 w-3.5" /> Create namespace
                </Button>
            </div>

            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {items.map((n) => (
                    <GlassCard key={n.name} className="p-5">
                        <div className="flex items-start justify-between">
                            <div className="flex items-center gap-2 min-w-0">
                                <Boxes className="h-4 w-4 shrink-0 text-i17-sky" />
                                <div className="mono truncate text-sm">{n.name}</div>
                            </div>
                            <StatusBadge status={n.status} />
                        </div>
                        <div className="mt-4 grid grid-cols-2 gap-3 text-xs text-muted-foreground">
                            <div>
                                <div className="text-[10px] uppercase tracking-widest">Pods</div>
                                <div className="mono mt-0.5 text-foreground">{n.pods}</div>
                            </div>
                            <div>
                                <div className="text-[10px] uppercase tracking-widest">Services</div>
                                <div className="mono mt-0.5 text-foreground">{n.services}</div>
                            </div>
                        </div>
                        <div className="mt-4 flex items-center justify-between border-t border-white/5 pt-3">
                            <span className="text-xs text-muted-foreground">{n.age}</span>
                            <Button size="sm" variant="ghost" onClick={() => remove(n)} className="text-muted-foreground hover:text-rose-400">
                                <Trash2 className="h-3.5 w-3.5" />
                            </Button>
                        </div>
                    </GlassCard>
                ))}
            </div>

            <Dialog open={open} onOpenChange={setOpen}>
                <DialogContent className="glass max-w-sm border-white/10">
                    <DialogHeader>
                        <DialogTitle>Create namespace</DialogTitle>
                    </DialogHeader>
                    <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="tenant-b" className="border-white/10 bg-white/[0.03]" />
                    <DialogFooter>
                        <Button variant="brand" onClick={create} disabled={busy || !name} className="rounded-xl">
                            {busy ? "Creating…" : "Create"}
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </div>
    );
}
