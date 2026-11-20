import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { CalendarClock, PlayCircle, Boxes, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { useConfirm } from "@/components/ConfirmDialog";
import SearchBox from "@/components/SearchBox";
import NamespaceFilter from "@/components/NamespaceFilter";
import SavedViews from "@/components/SavedViews";
import { useUrlState } from "@/lib/urlState";
import { useNamespaceFilter } from "@/lib/namespace";

function Section({ title, icon: Icon, rows, columns, empty }) {
    const { t } = useTranslation();
    return (
        <GlassCard className="overflow-hidden">
            <div className="flex items-center justify-between border-b px-4 py-3">
                <div className="flex items-center gap-2 text-sm font-medium">
                    <Icon className="h-4 w-4 text-tone-link" /> {title}
                </div>
            </div>
            <div className="max-h-[420px] overflow-auto scrollbar-thin" tabIndex={0} role="region" aria-label={title}>
                <table className="tbl">
                    <thead>
                        <tr>
                            {columns.map((c) => (
                                <th key={c.key} style={c.align === "right" ? { textAlign: "right" } : undefined}>
                                    {c.header || <span className="sr-only">{t("common:fields.actions")}</span>}
                                </th>
                            ))}
                        </tr>
                    </thead>
                    <tbody>
                        {rows.map((r, i) => (
                            <tr key={i}>
                                {columns.map((c) => (
                                    <td key={c.key} className={c.className} style={c.align === "right" ? { textAlign: "right" } : undefined}>
                                        {c.render ? c.render(r) : r[c.key]}
                                    </td>
                                ))}
                            </tr>
                        ))}
                        {rows.length === 0 && (
                            <tr>
                                <td colSpan={columns.length}>
                                    <div className="empty">{empty}</div>
                                </td>
                            </tr>
                        )}
                    </tbody>
                </table>
            </div>
        </GlassCard>
    );
}

export default function Jobs() {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const [cronJobs, setCronJobs] = useState(null);
    const [jobs, setJobs] = useState(null);
    const [q, setQ] = useUrlState("q", "");
    const [ns, setNs, knownNamespaces, globalNs] = useNamespaceFilter();

    const load = () => {
        Promise.all([
            api.get("/cronjobs").catch(() => ({ data: [] })),
            api.get("/jobs").catch(() => ({ data: [] })),
        ]).then(([c, j]) => {
            setCronJobs(c.data);
            setJobs(j.data);
        });
    };

    useEffect(() => {
        load();
        const timer = setInterval(load, 10000);
        return () => clearInterval(timer);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const trigger = async (c) => {
        try {
            await api.post(`/cronjobs/${c.namespace}/${encodeURIComponent(c.name)}/trigger`);
            toast.success(t("toasts:jobs.triggered", { name: c.name }));
            load();
        } catch (e) {
            toast.error(t("toasts:jobs.triggerFailed"), { description: e?.response?.data?.error || e.message });
        }
    };

    const setSuspend = async (c, suspend) => {
        try {
            await api.post(`/cronjobs/${c.namespace}/${encodeURIComponent(c.name)}/suspend`, { suspend });
            toast.success(suspend ? t("toasts:jobs.suspended", { name: c.name }) : t("toasts:jobs.resumed", { name: c.name }));
            load();
        } catch (e) {
            toast.error(t("toasts:jobs.updateFailed"), { description: e?.response?.data?.error || e.message });
        }
    };

    const deleteCronJob = async (c) => {
        const ok = await confirm({
            title: t("dialogs:jobs.deleteCronJob.title", { name: c.name }),
            description: t("dialogs:jobs.deleteCronJob.description"),
            confirmLabel: t("common:actions.delete"),
        });
        if (!ok) return;
        try {
            await api.delete(`/cronjobs/${c.namespace}/${encodeURIComponent(c.name)}`);
            toast.success(t("toasts:jobs.deleted", { name: c.name }));
            load();
        } catch (e) {
            toast.error(t("toasts:jobs.deleteFailed"), { description: e?.response?.data?.error || e.message });
        }
    };

    const deleteJob = async (j) => {
        const ok = await confirm({
            title: t("dialogs:jobs.deleteJob.title", { name: j.name }),
            description: t("dialogs:jobs.deleteJob.description"),
            confirmLabel: t("common:actions.delete"),
        });
        if (!ok) return;
        try {
            await api.delete(`/jobs/${j.namespace}/${encodeURIComponent(j.name)}`);
            toast.success(t("toasts:jobs.deleted", { name: j.name }));
            load();
        } catch (e) {
            toast.error(t("toasts:jobs.deleteFailed"), { description: e?.response?.data?.error || e.message });
        }
    };

    const matches = (r) => (ns === "all" || r.namespace === ns) && (q === "" || r.name.toLowerCase().includes(q.toLowerCase()));
    const shownCronJobs = useMemo(() => (cronJobs || []).filter(matches), [cronJobs, q, ns]); // eslint-disable-line react-hooks/exhaustive-deps
    const shownJobs = useMemo(() => (jobs || []).filter(matches), [jobs, q, ns]); // eslint-disable-line react-hooks/exhaustive-deps
    const namespaces = useMemo(
        () => [...new Set([...knownNamespaces, ...(cronJobs || []).map((r) => r.namespace), ...(jobs || []).map((r) => r.namespace)])].sort(),
        [knownNamespaces, cronJobs, jobs]
    );

    if (!cronJobs) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="mt-4 h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title={t("pages:jobs.title")}
                description={t("pages:jobs.description")}
                tools={
                    <>
                        <SearchBox placeholder={t("pages:jobs.searchPlaceholder")} value={q} onChange={setQ} />
                        <NamespaceFilter value={ns} onChange={setNs} namespaces={namespaces} />
                        <SavedViews
                            pageKey="jobs"
                            filters={{ q, ns }}
                            onApply={(f) => {
                                setQ(f.q ?? "");
                                setNs(f.ns ?? globalNs);
                            }}
                        />
                    </>
                }
            />

            <Section
                title={t("pages:jobs.cronJobs.title")}
                icon={CalendarClock}
                rows={shownCronJobs}
                empty={cronJobs.length ? t("pages:jobs.cronJobs.noMatches") : t("pages:jobs.cronJobs.empty")}
                columns={[
                    {
                        key: "name",
                        header: t("common:fields.name"),
                        className: "name",
                        render: (r) => (
                            <>
                                <Link to={`/cronjobs/${encodeURIComponent(r.namespace)}/${encodeURIComponent(r.name)}`} className="hover:underline">
                                    {r.name}
                                </Link>
                                <small>{r.namespace}</small>
                            </>
                        ),
                    },
                    { key: "schedule", header: t("pages:jobs.columns.schedule"), className: "mono", render: (r) => r.schedule },
                    { key: "status", header: t("common:fields.status"), render: (r) => <StatusBadge status={r.suspend ? "Paused" : "Running"} /> },
                    { key: "lastSchedule", header: t("pages:jobs.columns.lastRun"), className: "mono", render: (r) => r.lastSchedule || "—" },
                    { key: "age", header: t("common:fields.age"), className: "mono", render: (r) => r.age },
                    {
                        key: "actions",
                        header: "",
                        align: "right",
                        render: (r) => (
                            <div className="flex items-center justify-end gap-2">
                                <button type="button" className="btn btn-line btn-sm" title={t("pages:jobs.runNow")} onClick={() => trigger(r)}>
                                    <PlayCircle size={13} />
                                </button>
                                <Switch aria-label={t("pages:jobs.scheduleEnabledFor", { name: r.name })} checked={!r.suspend} onCheckedChange={(v) => setSuspend(r, !v)} />
                                <button type="button" className="btn btn-line btn-sm" title={t("common:actions.delete")} onClick={() => deleteCronJob(r)}>
                                    <Trash2 size={13} />
                                </button>
                            </div>
                        ),
                    },
                ]}
            />

            <div className="h-6" />

            <Section
                title={t("pages:jobs.jobs.title")}
                icon={Boxes}
                rows={shownJobs}
                empty={(jobs || []).length ? t("pages:jobs.jobs.noMatches") : t("pages:jobs.jobs.empty")}
                columns={[
                    {
                        key: "name",
                        header: t("common:fields.name"),
                        className: "name",
                        render: (r) => (
                            <>
                                <Link to={`/jobs/${encodeURIComponent(r.namespace)}/${encodeURIComponent(r.name)}`} className="hover:underline">
                                    {r.name}
                                </Link>
                                <small>{r.ownerCronJob ? t("pages:jobs.namespaceFromCronJob", { namespace: r.namespace, cronJob: r.ownerCronJob }) : r.namespace}</small>
                            </>
                        ),
                    },
                    { key: "status", header: t("common:fields.status"), render: (r) => <StatusBadge status={r.status} /> },
                    { key: "completions", header: t("pages:jobs.columns.completions"), className: "mono", render: (r) => r.completions },
                    { key: "duration", header: t("pages:jobs.columns.duration"), className: "mono", render: (r) => r.duration || "—" },
                    { key: "age", header: t("common:fields.age"), className: "mono", render: (r) => r.age },
                    {
                        key: "actions",
                        header: "",
                        align: "right",
                        render: (r) => (
                            <div className="flex justify-end">
                                <button type="button" className="btn btn-line btn-sm" title={t("common:actions.delete")} onClick={() => deleteJob(r)}>
                                    <Trash2 size={13} />
                                </button>
                            </div>
                        ),
                    },
                ]}
            />
        </div>
    );
}
