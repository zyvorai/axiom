import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
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
import { Boxes, Plus, Trash2, ChevronRight } from "lucide-react";
import { toast } from "sonner";
import { useConfirm } from "@/components/ConfirmDialog";
import { Link } from "react-router-dom";

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
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <div className="grid g3 mt-4">
                    {[...Array(6)].map((_, i) => (
                        <Skeleton key={i} className="h-48 rounded-2xl bg-muted" />
                    ))}
                </div>
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title="Namespaces"
                description="Every namespace in the cluster, with pod and service counts."
                action={
                    <button type="button" className="btn btn-fill" onClick={() => setOpen(true)}>
                        <Plus size={15} /> Create namespace
                    </button>
                }
            />

            <div className="grid g3">
                {items.map((n) => (
                    <div key={n.name} className="tile">
                        <Link
                            to={`/namespaces/${encodeURIComponent(n.name)}`}
                            data-testid={`namespace-card-${n.name}`}
                            className="block text-inherit"
                        >
                            <div className="tile-h">
                                <Boxes size={16} className="text-tone-link shrink-0" />
                                <b className="mono truncate">{n.name}</b>
                                <StatusBadge status={n.status} />
                            </div>
                            <div className="tile-specs c2">
                                <div>
                                    Pods
                                    <b>{n.pods}</b>
                                </div>
                                <div>
                                    Services
                                    <b>{n.services}</b>
                                </div>
                            </div>
                        </Link>
                        <div className="tile-acts justify-between items-center">
                            <Link to={`/namespaces/${encodeURIComponent(n.name)}`} className="btn btn-ghost btn-sm pl-0">
                                Open <ChevronRight size={13} />
                            </Link>
                            <div className="flex items-center gap-2">
                                <span className="text-[12.5px] text-ink-3">{n.age}</span>
                                <button
                                    type="button"
                                    className="btn btn-line btn-sm"
                                    onClick={(e) => {
                                        e.preventDefault();
                                        e.stopPropagation();
                                        remove(n);
                                    }}
                                    aria-label={`Delete namespace ${n.name}`}
                                >
                                    <Trash2 size={13} />
                                </button>
                            </div>
                        </div>
                    </div>
                ))}
            </div>

            <Dialog open={open} onOpenChange={setOpen}>
                <DialogContent className="max-w-sm">
                    <DialogHeader>
                        <DialogTitle>Create namespace</DialogTitle>
                    </DialogHeader>
                    <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="tenant-b" className="bg-muted/30" />
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
