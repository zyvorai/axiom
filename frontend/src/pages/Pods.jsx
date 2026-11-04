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
import { Terminal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { useConfirm } from "@/components/ConfirmDialog";
import MacTerm from "@/components/MacTerm";
import SearchBox from "@/components/SearchBox";
import NamespaceFilter from "@/components/NamespaceFilter";
import SavedViews from "@/components/SavedViews";
import { useUrlState } from "@/lib/urlState";
import { useNamespaceFilter } from "@/lib/namespace";

function ExecDialog({ pod, onOpenChange }) {
    const { t } = useTranslation();
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
            <DialogContent className="max-w-lg">
                <DialogHeader>
                    <DialogTitle className="mono text-sm">{t("dialogs:pods.exec.title", { name: pod.name })}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    {containers.length > 1 && (
                        <Select value={container} onValueChange={setContainer}>
                            <SelectTrigger aria-label={t("dialogs:pods.exec.container")} className="bg-muted/30">
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
                            {busy ? t("dialogs:pods.exec.running") : t("dialogs:pods.exec.run")}
                        </Button>
                    </div>
                    <MacTerm
                        title={`exec — ${pod.name}`}
                        bodyClassName="h-64 overflow-auto whitespace-pre-wrap px-4 py-3 text-[12px] leading-[1.55] text-term-plain"
                    >
                        {output || <span className="text-term-dim">{t("dialogs:pods.exec.outputPlaceholder")}</span>}
                    </MacTerm>
                </div>
            </DialogContent>
        </Dialog>
    );
}

export default function Pods() {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const [data, setData] = useState(null);
    const [q, setQ] = useUrlState("q", "");
    const [ns, setNs, knownNamespaces, globalNs] = useNamespaceFilter();
    const [execTarget, setExecTarget] = useState(null);
    const [selected, setSelected] = useState(new Set());
    const [bulkBusy, setBulkBusy] = useState(false);

    const load = () => api.get("/pods").then((r) => setData(r.data));

    useEffect(() => {
        let alive = true;
        const tick = () => api.get("/pods").then((r) => alive && setData(r.data));
        tick();
        const timer = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    const { namespaces, rows } = useMemo(() => {
        if (!data) return { namespaces: [], rows: [] };
        const namespaces = [...new Set([...knownNamespaces, ...data.map((p) => p.namespace)])].sort();
        const rows = data.filter(
            (p) =>
                (ns === "all" || p.namespace === ns) &&
                (q === "" || p.name.toLowerCase().includes(q.toLowerCase()))
        );
        return { namespaces, rows };
    }, [data, q, ns, knownNamespaces]);

    const evict = async (p) => {
        const ok = await confirm({
            title: t("dialogs:pods.evict.title", { name: p.name }),
            description: t("dialogs:pods.evict.description"),
            confirmLabel: t("dialogs:pods.evict.confirm"),
        });
        if (!ok) return;
        try {
            await api.post(`/pods/${p.namespace}/${encodeURIComponent(p.name)}/evict`);
            toast.success(t("toasts:pods.evicted", { name: p.name }));
            load();
        } catch (e) {
            toast.error(t("toasts:pods.evictFailed"), { description: e?.response?.data?.error || e.message });
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
            title: t("dialogs:pods.bulkEvict.title", { count: targets.length }),
            description: t("dialogs:pods.evict.description"),
            confirmLabel: t("dialogs:pods.evict.confirm"),
        });
        if (!ok) return;
        setBulkBusy(true);
        try {
            const results = await Promise.allSettled(
                targets.map((p) => api.post(`/pods/${p.namespace}/${encodeURIComponent(p.name)}/evict`))
            );
            const failed = results.filter((r) => r.status === "rejected").length;
            const note = skipped > 0 ? t("toasts:pods.skippedNote", { count: skipped }) : "";
            if (failed) toast.error(t("toasts:pods.bulkEvictFailed", { failed, total: targets.length, note }));
            else toast.success(t("toasts:pods.bulkEvicted", { count: targets.length, note }));
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
                title={t("pages:pods.title")}
                description={t("pages:pods.description")}
                action={
                    selected.size > 0 && (
                        <div className="flex items-center gap-3 border-y border-border px-3 py-1.5">
                            <span className="text-xs text-muted-foreground">{t("common:count.selected", { count: selected.size })}</span>
                            <Button size="sm" variant="outline" disabled={bulkBusy} onClick={bulkEvict} className="h-7 rounded-lg border-soft-danger-border bg-soft-danger text-xs text-tone-red hover:bg-soft-danger">
                                <Trash2 className="mr-1 h-3 w-3" /> {bulkBusy ? t("pages:pods.evicting") : t("pages:pods.evict")}
                            </Button>
                        </div>
                    )
                }
                tools={
                    <>
                        <SearchBox placeholder={t("pages:pods.searchPlaceholder")} value={q} onChange={setQ} />
                        <NamespaceFilter value={ns} onChange={setNs} namespaces={namespaces} />
                        <SavedViews
                            pageKey="pods"
                            filters={{ q, ns }}
                            onApply={(f) => {
                                setQ(f.q ?? "");
                                setNs(f.ns ?? globalNs);
                            }}
                        />
                    </>
                }
            />

            <GlassCard className="overflow-hidden p-0">
                <div className="max-h-[65vh] overflow-auto scrollbar-thin" tabIndex={0} role="region" aria-label={t("pages:pods.title")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th className="w-10">
                                    <Checkbox
                                        aria-label={t("pages:pods.selectAll")}
                                        checked={rows.length > 0 && selected.size === rows.length}
                                        onCheckedChange={toggleAll}
                                    />
                                </th>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("common:fields.namespace")}</th>
                                <th>{t("common:fields.node")}</th>
                                <th>{t("common:fields.ready")}</th>
                                <th>{t("common:fields.restarts")}</th>
                                <th>{t("common:fields.status")}</th>
                                <th>{t("pages:pods.columns.ip")}</th>
                                <th>{t("common:fields.age")}</th>
                                <th><span className="sr-only">{t("common:fields.actions")}</span></th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((p) => (
                                <tr key={`${p.namespace}/${p.name}`}>
                                    <td>
                                        <Checkbox aria-label={t("pages:pods.selectOne", { name: p.name })} checked={selected.has(key(p))} onCheckedChange={(v) => toggle(p, v)} />
                                    </td>
                                    <td className="name">
                                        <Link to={`/pods/${p.namespace}/${encodeURIComponent(p.name)}`} className="hover:underline">
                                            {p.name}
                                        </Link>
                                    </td>
                                    <td className="mono">{p.namespace}</td>
                                    <td className="mono">{p.node}</td>
                                    <td className="mono num">{p.ready}</td>
                                    <td className={`mono num ${p.restarts > 0 ? "text-tone-amber" : ""}`}>{p.restarts}</td>
                                    <td>
                                        <StatusBadge status={p.status} />
                                    </td>
                                    <td className="mono">{p.ip}</td>
                                    <td className="mono">{p.age}</td>
                                    <td>
                                        <div className="flex items-center justify-end gap-1.5">
                                            <button type="button" className="btn btn-line btn-sm" title={t("pages:pods.exec")} onClick={() => setExecTarget(p)}>
                                                <Terminal size={13} />
                                            </button>
                                            <button type="button" className="btn btn-line btn-sm" title={t("pages:pods.evict")} onClick={() => evict(p)}>
                                                <Trash2 size={13} />
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                            {rows.length === 0 && (
                                <tr>
                                    <td colSpan={10}>
                                        <div className="empty">{t("pages:pods.noMatches")}</div>
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
