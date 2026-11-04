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
import { useTranslation } from "react-i18next";
import { useConfirm } from "@/components/ConfirmDialog";

const SECRET_TYPES = ["Opaque", "kubernetes.io/tls", "kubernetes.io/dockerconfigjson", "kubernetes.io/basic-auth"];

function CreateSecretDialog({ open, onOpenChange, onCreated }) {
    const { t } = useTranslation();
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
            toast.success(t("toasts:secrets.created", { name }));
            onOpenChange(false);
            setName("");
            setPairs([{ key: "", value: "" }]);
            onCreated();
        } catch (e) {
            toast.error(t("toasts:secrets.createFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-md">
                <DialogHeader>
                    <DialogTitle>{t("dialogs:secrets.create.title")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label htmlFor="secrets-namespace" className="text-xs text-muted-foreground">{t("common:fields.namespace")}</Label>
                            <Input id="secrets-namespace" value={namespace} onChange={(e) => setNamespace(e.target.value)} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="secrets-name" className="text-xs text-muted-foreground">{t("common:fields.name")}</Label>
                            <Input id="secrets-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="app-config" className="bg-muted/30" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="secrets-type" className="text-xs text-muted-foreground">{t("common:fields.type")}</Label>
                        <Select value={type} onValueChange={setType}>
                            <SelectTrigger id="secrets-type" className="bg-muted/30">
                                <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                                {SECRET_TYPES.map((st) => (
                                    <SelectItem key={st} value={st}>
                                        {st}
                                    </SelectItem>
                                ))}
                            </SelectContent>
                        </Select>
                    </div>
                    <div className="space-y-1.5">
                        <div id="secret-data-label" className="text-xs font-medium leading-none text-muted-foreground">{t("dialogs:secrets.create.data")}</div>
                        <div className="space-y-2" role="group" aria-labelledby="secret-data-label">
                            {pairs.map((p, i) => (
                                <div key={i} className="flex items-center gap-1.5">
                                    <Input value={p.key} onChange={(e) => setPair(i, { key: e.target.value })} placeholder={t("dialogs:secrets.create.keyPlaceholder")} aria-label={t("dialogs:secrets.create.keyAria", { n: i + 1 })} className="bg-muted/30" />
                                    <Input value={p.value} onChange={(e) => setPair(i, { value: e.target.value })} placeholder={t("dialogs:secrets.create.valuePlaceholder")} aria-label={t("dialogs:secrets.create.valueAria", { n: i + 1 })} className="bg-muted/30" />
                                    <button aria-label={t("dialogs:secrets.create.removeRow")} title={t("dialogs:secrets.create.removeRow")} onClick={() => removePair(i)} className="shrink-0 text-muted-foreground hover:text-tone-red">
                                        <X className="h-4 w-4" />
                                    </button>
                                </div>
                            ))}
                        </div>
                        <Button size="sm" variant="ghost" onClick={addPair} className="mt-1 text-xs text-muted-foreground">
                            <Plus className="mr-1 h-3.5 w-3.5" /> {t("dialogs:secrets.create.addKey")}
                        </Button>
                    </div>
                </div>
                <DialogFooter>
                    <Button variant="brand" onClick={submit} disabled={busy || !name} className="rounded-xl">
                        {busy ? t("dialogs:secrets.create.creating") : t("common:actions.create")}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function ExternalSecretsSection() {
    const { t } = useTranslation();
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
                <Link2 className="h-4 w-4 text-tone-link" /> {t("pages:secrets.external.title")}
            </div>
            <div className="max-h-[300px] overflow-auto scrollbar-thin" tabIndex={0} role="region" aria-label={t("pages:secrets.external.aria")}>
                {!items && <Skeleton className="m-4 h-16 rounded-xl bg-muted" />}
                {items && (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("pages:secrets.external.columns.store")}</th>
                                <th>{t("pages:secrets.external.columns.refresh")}</th>
                                <th>{t("common:fields.status")}</th>
                                <th>{t("common:fields.age")}</th>
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
                                        <div className="empty">{t("pages:secrets.external.empty")}</div>
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
    const { t } = useTranslation();
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
            title: t("dialogs:secrets.delete.title", { name: s.name }),
            description: t("dialogs:secrets.delete.description"),
            confirmLabel: t("common:actions.delete"),
        });
        if (!ok) return;
        try {
            await api.delete(`/secrets/${s.namespace}/${encodeURIComponent(s.name)}`);
            toast.success(t("toasts:secrets.deleted", { name: s.name }));
            load();
        } catch (e) {
            toast.error(t("toasts:secrets.deleteFailed"), { description: e?.response?.data?.error || e.message });
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
                title={t("pages:secrets.title")}
                description={t("pages:secrets.description")}
                action={
                    <>
                        <button type="button" className="btn btn-line" onClick={load}>
                            <RefreshCcw size={15} /> {t("common:actions.refresh")}
                        </button>
                        <button type="button" className="btn btn-fill" onClick={() => setCreateOpen(true)}>
                            <Plus size={15} /> {t("pages:secrets.create")}
                        </button>
                    </>
                }
                tools={
                    <div className="input">
                        <Search size={15} className="text-ink-3" />
                        <input
                            placeholder={t("pages:secrets.searchPlaceholder")}
                            value={q}
                            onChange={(e) => setQ(e.target.value)} className="bare-input"
                        />
                    </div>
                }
            />

            <GlassCard className="overflow-hidden p-0">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin" tabIndex={0} role="region" aria-label={t("pages:secrets.tableAria")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("common:fields.namespace")}</th>
                                <th>{t("common:fields.type")}</th>
                                <th>{t("pages:secrets.columns.keys")}</th>
                                <th>{t("common:fields.age")}</th>
                                <th><span className="sr-only">{t("common:fields.actions")}</span></th>
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
                                                <Badge key={k} variant="outline" className="mono text-[11px] bg-muted/30">
                                                    {k}
                                                </Badge>
                                            ))}
                                        </div>
                                    </td>
                                    <td className="mono">{s.age}</td>
                                    <td>
                                        <button type="button" className="btn btn-line btn-sm" aria-label={t("pages:secrets.deleteAria", { name: s.name })} title={t("pages:secrets.deleteAria", { name: s.name })} onClick={() => remove(s)}>
                                            <Trash2 size={13} />
                                        </button>
                                    </td>
                                </tr>
                            ))}
                            {rows.length === 0 && (
                                <tr>
                                    <td colSpan={6}>
                                        <div className="empty">{t("pages:secrets.empty")}</div>
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
