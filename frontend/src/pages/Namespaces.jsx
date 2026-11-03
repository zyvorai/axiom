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
import { useTranslation } from "react-i18next";

export default function Namespaces() {
    const { t } = useTranslation();
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
            toast.success(t("toasts:namespaces.created", { name }));
            setOpen(false);
            setName("");
            load();
        } catch (e) {
            toast.error(t("toasts:namespaces.createFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    const remove = async (n) => {
        const ok = await confirm({
            title: t("dialogs:namespaces.delete.title", { name: n.name }),
            description: t("dialogs:namespaces.delete.description"),
            confirmLabel: t("common:actions.delete"),
        });
        if (!ok) return;
        try {
            await api.delete(`/namespaces/${encodeURIComponent(n.name)}`);
            toast.success(t("toasts:namespaces.deleted", { name: n.name }));
            load();
        } catch (e) {
            toast.error(t("toasts:namespaces.deleteFailed"), { description: e?.response?.data?.error || e.message });
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
                title={t("pages:namespaces.title")}
                description={t("pages:namespaces.description")}
                action={
                    <button type="button" className="btn btn-fill" onClick={() => setOpen(true)}>
                        <Plus size={15} /> {t("pages:namespaces.create")}
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
                                    {t("pages:namespaces.specs.pods")}
                                    <b>{n.pods}</b>
                                </div>
                                <div>
                                    {t("pages:namespaces.specs.services")}
                                    <b>{n.services}</b>
                                </div>
                            </div>
                        </Link>
                        <div className="tile-acts justify-between items-center">
                            <Link to={`/namespaces/${encodeURIComponent(n.name)}`} className="btn btn-ghost btn-sm pl-0">
                                {t("common:actions.open")} <ChevronRight size={13} />
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
                                    aria-label={t("pages:namespaces.deleteAria", { name: n.name })}
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
                        <DialogTitle>{t("dialogs:namespaces.create.title")}</DialogTitle>
                    </DialogHeader>
                    <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="tenant-b" className="bg-muted/30" />
                    <DialogFooter>
                        <Button variant="brand" onClick={create} disabled={busy || !name} className="rounded-xl">
                            {busy ? t("dialogs:namespaces.create.creating") : t("common:actions.create")}
                        </Button>
                    </DialogFooter>
                </DialogContent>
            </Dialog>
        </div>
    );
}
