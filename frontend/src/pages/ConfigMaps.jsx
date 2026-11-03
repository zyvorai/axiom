import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
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
import { Badge } from "@/components/ui/badge";
import { FileCog, Search, Trash2, Plus, X } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { useConfirm } from "@/components/ConfirmDialog";

function CreateConfigMapDialog({ open, onOpenChange, onCreated }) {
    const { t } = useTranslation();
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
            toast.success(t("toasts:configMaps.created", { name }));
            onOpenChange(false);
            setName("");
            setPairs([{ key: "", value: "" }]);
            onCreated();
        } catch (e) {
            toast.error(t("toasts:configMaps.createFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>{t("dialogs:configMaps.create.title")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label htmlFor="config-maps-namespace" className="text-xs text-muted-foreground">{t("common:fields.namespace")}</Label>
                            <Input id="config-maps-namespace" value={namespace} onChange={(e) => setNamespace(e.target.value)} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="config-maps-name" className="text-xs text-muted-foreground">{t("common:fields.name")}</Label>
                            <Input id="config-maps-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="app-config" className="bg-muted/30" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <div id="cm-data-label" className="text-xs font-medium leading-none text-muted-foreground">{t("dialogs:configMaps.create.data")}</div>
                        <div className="space-y-2" role="group" aria-labelledby="cm-data-label">
                            {pairs.map((p, i) => (
                                <div key={i} className="flex items-center gap-1.5">
                                    <Input value={p.key} onChange={(e) => setPair(i, { key: e.target.value })} placeholder="KEY" aria-label={t("dialogs:configMaps.create.keyAria", { n: i + 1 })} className="bg-muted/30" />
                                    <Input value={p.value} onChange={(e) => setPair(i, { value: e.target.value })} placeholder="value" aria-label={t("dialogs:configMaps.create.valueAria", { n: i + 1 })} className="bg-muted/30" />
                                    <button aria-label={t("dialogs:configMaps.create.removeRow")} title={t("dialogs:configMaps.create.removeRow")} onClick={() => removePair(i)} className="shrink-0 text-muted-foreground hover:text-tone-red">
                                        <X className="h-4 w-4" />
                                    </button>
                                </div>
                            ))}
                        </div>
                        <Button size="sm" variant="ghost" onClick={addPair} className="mt-1 text-xs text-muted-foreground">
                            <Plus className="mr-1 h-3.5 w-3.5" /> {t("dialogs:configMaps.create.addKey")}
                        </Button>
                    </div>
                </div>
                <DialogFooter>
                    <Button variant="brand" onClick={submit} disabled={busy || !name} className="rounded-xl">
                        {busy ? t("dialogs:configMaps.create.creating") : t("common:actions.create")}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

export default function ConfigMaps() {
    const { t } = useTranslation();
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
            title: t("dialogs:configMaps.delete.title", { name: c.name }),
            description: t("dialogs:configMaps.delete.description"),
            confirmLabel: t("common:actions.delete"),
        });
        if (!ok) return;
        try {
            await api.delete(`/configmaps/${c.namespace}/${encodeURIComponent(c.name)}`);
            toast.success(t("toasts:configMaps.deleted", { name: c.name }));
            load();
        } catch (e) {
            toast.error(t("toasts:configMaps.deleteFailed"), { description: e?.response?.data?.error || e.message });
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
                title={t("pages:configMaps.title")}
                description={t("pages:configMaps.description")}
                action={
                    <button type="button" className="btn btn-fill" onClick={() => setCreateOpen(true)}>
                        <Plus size={15} /> {t("pages:configMaps.create")}
                    </button>
                }
                tools={
                    <div className="input">
                        <Search size={15} className="text-ink-3" />
                        <input
                            placeholder={t("pages:configMaps.searchPlaceholder")}
                            value={q}
                            onChange={(e) => setQ(e.target.value)} className="bare-input"
                        />
                    </div>
                }
            />

            <GlassCard className="overflow-hidden p-0">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin" tabIndex={0} role="region" aria-label={t("pages:configMaps.tableAria")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("common:fields.namespace")}</th>
                                <th>{t("pages:configMaps.columns.keys")}</th>
                                <th>{t("common:fields.age")}</th>
                                <th><span className="sr-only">{t("common:fields.actions")}</span></th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((c) => (
                                <tr key={`${c.namespace}/${c.name}`}>
                                    <td className="name">
                                        <span className="flex gap-2 items-center">
                                            <FileCog size={14} className="text-ink-3" />
                                            <Link
                                                to={`/configmaps/${encodeURIComponent(c.namespace)}/${encodeURIComponent(c.name)}`}
                                                className="hover:underline"
                                                title={c.name}
                                            >
                                                {c.name}
                                            </Link>
                                        </span>
                                    </td>
                                    <td className="mono">{c.namespace}</td>
                                    <td>
                                        <div className="flex flex-wrap gap-1">
                                            {(c.keys || []).map((k) => (
                                                <Badge key={k} variant="outline" className="mono text-[11px] bg-muted/30">
                                                    {k}
                                                </Badge>
                                            ))}
                                        </div>
                                    </td>
                                    <td className="mono">{c.age}</td>
                                    <td>
                                        <button type="button" className="btn btn-line btn-sm" aria-label={t("pages:configMaps.deleteAria", { name: c.name })} title={t("pages:configMaps.deleteAria", { name: c.name })} onClick={() => remove(c)}>
                                            <Trash2 size={13} />
                                        </button>
                                    </td>
                                </tr>
                            ))}
                            {rows.length === 0 && (
                                <tr>
                                    <td colSpan={5}>
                                        <div className="empty">{t("pages:configMaps.empty")}</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>

            <CreateConfigMapDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={load} />
        </div>
    );
}
