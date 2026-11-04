import { usePageTitle } from "@/lib/pageTitle";
import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams, useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { formatDistanceToNowStrict } from "date-fns";
import { useTranslation } from "react-i18next";
import { dateFnsLocale } from "@/lib/i18n";
import { Anchor, GitBranch, Boxes, Bot, Fingerprint, ScrollText, ShieldCheck, DollarSign, Trash2, RotateCcw, Check, X, Webhook, Plus, Search, Download, Pencil } from "lucide-react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import MacTerm from "@/components/MacTerm";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { toast } from "sonner";
import { useConfirm } from "@/components/ConfirmDialog";
import { RbacBySubject, RbacWhoCan } from "@/components/RbacExplorer";
import { RBAC, HELM_CATALOG, GITOPS_CREATE, OPENCOST, GITOPS_EDIT } from "@/constants/testIds";
import HelmCatalog from "@/components/HelmCatalog";
import { CostBudgets, CostRightsizing } from "@/components/CostPanels";
import GitOpsCreateDialog from "@/components/GitOpsCreateDialog";
import GitOpsDeleteDialog from "@/components/GitOpsDeleteDialog";
import GitOpsEditDialog from "@/components/GitOpsEditDialog";
import GitOpsCredentials, { CredentialsButton } from "@/components/GitOpsRepoCreds";
import { useAuth } from "@/lib/auth";

const TABS = [
    { key: "helm", labelKey: "pages:ops.tabs.helm", icon: Anchor },
    { key: "gitops", labelKey: "pages:ops.tabs.gitops", icon: GitBranch },
    { key: "agents", labelKey: "pages:ops.tabs.agents", icon: Bot },
    { key: "identity", labelKey: "pages:ops.tabs.identity", icon: Fingerprint },
    { key: "audit", labelKey: "pages:ops.tabs.audit", icon: ScrollText },
    { key: "access", labelKey: "pages:ops.tabs.access", icon: ShieldCheck },
    { key: "webhooks", labelKey: "pages:ops.tabs.webhooks", icon: Webhook },
    { key: "cost", labelKey: "pages:ops.tabs.cost", icon: DollarSign },
];

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true, locale: dateFnsLocale() });
    } catch {
        return iso;
    }
}

function useEndpoint(path, deps = []) {
    const [data, setData] = useState(null);
    useEffect(() => {
        let alive = true;
        setData(null);
        api.get(path).then((r) => alive && setData(r.data)).catch(() => alive && setData(null));
        return () => {
            alive = false;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, deps);
    return data;
}

// Row-list wrapper for tabs whose rows are richer than a flat table (Agents, Webhooks).
function Rows({ items, empty, render }) {
    const { t } = useTranslation();
    if (!items) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    if (items.length === 0) return <div className="empty">{empty}</div>;
    return <div className="max-h-[520px] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:ops.results")}>{items.map(render)}</div>;
}

// Renders a `.tbl` once `items` has rows; a loading skeleton or `.empty` block
// otherwise (kept outside any <table> so the DOM stays valid either way).
function TablePanel({ items, empty, table }) {
    if (!items) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    if (items.length === 0) return <div className="empty">{empty}</div>;
    return table(items);
}

function HelmRow({ r, onChanged }) {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const [revision, setRevision] = useState("");
    const [busy, setBusy] = useState("");

    const uninstall = async () => {
        const ok = await confirm({
            title: t("dialogs:ops.uninstall.title", { name: r.name }),
            description: t("dialogs:ops.cannotUndo"),
            confirmLabel: t("dialogs:ops.uninstall.confirm"),
        });
        if (!ok) return;
        setBusy("uninstall");
        try {
            await api.post(`/helm/releases/${r.namespace}/${encodeURIComponent(r.name)}/uninstall`);
            toast.success(t("toasts:ops.helm.uninstalled", { name: r.name }));
            onChanged();
        } catch (e) {
            toast.error(t("toasts:ops.helm.uninstallFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    const rollback = async () => {
        setBusy("rollback");
        try {
            await api.post(`/helm/releases/${r.namespace}/${encodeURIComponent(r.name)}/rollback`, {
                revision: revision ? Number(revision) : 0,
            });
            toast.success(t("toasts:ops.helm.rolledBack", { name: r.name }));
            onChanged();
        } catch (e) {
            toast.error(t("toasts:ops.helm.rollbackFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    return (
        <tr>
            <td className="name">
                {r.name}
                <small>{r.namespace}</small>
            </td>
            <td className="mono text-[12.5px] text-ink-3">{r.chart}</td>
            <td className="num">{t("pages:ops.helm.rev", { n: r.revision })}</td>
            <td>
                <StatusBadge status={r.status === "deployed" ? "Healthy" : r.status} />
            </td>
            <td className="num">{r.age}</td>
            <td>
                <div className="flex items-center justify-end gap-1.5">
                    <Input
                        value={revision}
                        onChange={(e) => setRevision(e.target.value)}
                        placeholder={t("pages:ops.helm.revPlaceholder")}
                        className="h-7 w-14 bg-muted/30 px-2 text-xs"
                    />
                    <button type="button" className="btn btn-line btn-sm" disabled={!!busy} onClick={rollback} title={t("pages:ops.helm.rollback")}>
                        <RotateCcw size={13} />
                    </button>
                    <button type="button" className="btn btn-danger btn-sm" disabled={!!busy} onClick={uninstall} title={t("pages:ops.helm.uninstall")}>
                        <Trash2 size={13} />
                    </button>
                </div>
            </td>
        </tr>
    );
}

function InstallChartDialog({ open, onOpenChange, onInstalled }) {
    const { t } = useTranslation();
    const [form, setForm] = useState({ namespace: "default", releaseName: "", repoUrl: "", chartName: "", chartVersion: "", values: "" });
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const submit = async () => {
        setBusy(true);
        try {
            await api.post("/helm/install", form);
            toast.success(t("toasts:ops.helm.installStarted", { name: form.releaseName }));
            onOpenChange(false);
            onInstalled();
        } catch (e) {
            toast.error(t("toasts:ops.helm.installFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-lg">
                <DialogHeader>
                    <DialogTitle>{t("dialogs:ops.installChart.title")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label htmlFor="ops-namespace" className="text-xs text-muted-foreground">{t("common:fields.namespace")}</Label>
                            <Input id="ops-namespace" value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="ops-release-name" className="text-xs text-muted-foreground">{t("dialogs:ops.installChart.releaseName")}</Label>
                            <Input id="ops-release-name" value={form.releaseName} onChange={(e) => set({ releaseName: e.target.value })} placeholder="my-release" className="bg-muted/30" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="ops-chart-repo-url" className="text-xs text-muted-foreground">{t("dialogs:ops.installChart.repoUrl")}</Label>
                        <Input id="ops-chart-repo-url" value={form.repoUrl} onChange={(e) => set({ repoUrl: e.target.value })} placeholder="https://charts.example.com" className="bg-muted/30" />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label htmlFor="ops-chart-name" className="text-xs text-muted-foreground">{t("dialogs:ops.installChart.chartName")}</Label>
                            <Input id="ops-chart-name" value={form.chartName} onChange={(e) => set({ chartName: e.target.value })} placeholder="ingress-nginx" className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="ops-version" className="text-xs text-muted-foreground">{t("dialogs:ops.installChart.version")}</Label>
                            <Input id="ops-version" value={form.chartVersion} onChange={(e) => set({ chartVersion: e.target.value })} placeholder="4.11.0" className="bg-muted/30" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="ops-values-yaml" className="text-xs text-muted-foreground">{t("dialogs:ops.installChart.values")}</Label>
                        <MacTerm title="values.yaml" bodyClassName="!p-0">
                            <Textarea id="ops-values-yaml" value={form.values} onChange={(e) => set({ values: e.target.value })} className="macos-term-body mono min-h-[100px] resize-none rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0" spellCheck={false} />
                        </MacTerm>
                    </div>
                    <p className="text-[11px] text-muted-foreground">
                        {t("dialogs:ops.installChart.note")}
                    </p>
                </div>
                <DialogFooter>
                    <button type="button" className="btn btn-fill" onClick={submit} disabled={busy || !form.releaseName || !form.repoUrl || !form.chartName}>
                        {busy ? t("dialogs:ops.installChart.starting") : t("dialogs:ops.installChart.install")}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function HelmTab() {
    const { t } = useTranslation();
    const [reload, setReload] = useState(0);
    const releases = useEndpoint("/helm/releases", [reload]);
    const [installOpen, setInstallOpen] = useState(false);
    const [view, setView] = useState("releases");
    return (
        <div className="stack">
            <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="seg" role="group" aria-label={t("pages:ops.helm.view")}>
                    <button type="button" aria-pressed={view === "releases"} data-testid={HELM_CATALOG.viewReleases} onClick={() => setView("releases")}>
                        {t("pages:ops.helm.releases")}
                    </button>
                    <button type="button" aria-pressed={view === "catalog"} data-testid={HELM_CATALOG.viewCatalog} onClick={() => setView("catalog")}>
                        <Boxes size={13} className="mr-1 inline" aria-hidden="true" /> {t("pages:ops.helm.catalog")}
                    </button>
                </div>
                {view === "releases" && (
                    <button type="button" className="btn btn-fill btn-sm" onClick={() => setInstallOpen(true)}>
                        <Plus size={14} /> {t("pages:ops.helm.installChart")}
                    </button>
                )}
            </div>
            {view === "catalog" ? (
                <HelmCatalog
                    onInstalled={() => {
                        setReload((n) => n + 1);
                        setView("releases");
                    }}
                />
            ) : (
            <div className="card p-0">
                <TablePanel
                    items={releases}
                    empty={t("pages:ops.helm.empty")}
                    table={(rows) => (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("pages:ops.helm.columns.release")}</th>
                                    <th>{t("pages:ops.helm.columns.chart")}</th>
                                    <th>{t("pages:ops.helm.columns.revision")}</th>
                                    <th>{t("common:fields.status")}</th>
                                    <th>{t("common:fields.age")}</th>
                                    <th><span className="sr-only">{t("common:fields.actions")}</span></th>
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map((r) => (
                                    <HelmRow key={`${r.namespace}/${r.name}`} r={r} onChanged={() => setReload((n) => n + 1)} />
                                ))}
                            </tbody>
                        </table>
                    )}
                />
            </div>
            )}
            <InstallChartDialog open={installOpen} onOpenChange={setInstallOpen} onInstalled={() => setReload((n) => n + 1)} />
        </div>
    );
}

function GitOpsTab() {
    const { t } = useTranslation();
    const [refresh, setRefresh] = useState(0);
    const apps = useEndpoint("/gitops", [refresh]);
    const [syncing, setSyncing] = useState("");
    const [createOpen, setCreateOpen] = useState(false);
    const [deleting, setDeleting] = useState(null);
    const [editing, setEditing] = useState(null);
    const [credsOpen, setCredsOpen] = useState(false);
    const { isViewer } = useAuth();

    const sync = async (a) => {
        const busyKey = `${a.namespace}/${a.name}`;
        setSyncing(busyKey);
        try {
            await api.post(`/gitops/${a.kind}/${a.namespace}/${encodeURIComponent(a.name)}/sync`);
            toast.success(t("toasts:ops.gitops.syncRequested", { name: a.name }));
            setRefresh((n) => n + 1);
        } catch (e) {
            toast.error(t("toasts:ops.gitops.syncFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setSyncing("");
        }
    };

    return (
        <div className="stack">
        <div className="flex justify-end gap-2">
            {!isViewer && <CredentialsButton onClick={() => setCredsOpen(true)} />}
            <button type="button" className="btn btn-fill btn-sm" data-testid={GITOPS_CREATE.open} onClick={() => setCreateOpen(true)}>
                <Plus size={14} /> {t("pages:ops.gitops.newApplication")}
            </button>
        </div>
        <div className="card p-0">
            <TablePanel
                items={apps}
                empty={t("pages:ops.gitops.empty")}
                table={(rows) => (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("pages:ops.gitops.columns.application")}</th>
                                <th>{t("common:fields.source")}</th>
                                <th>{t("common:fields.status")}</th>
                                <th>{t("common:fields.age")}</th>
                                <th><span className="sr-only">{t("common:fields.actions")}</span></th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((a) => (
                                <tr key={`${a.namespace}/${a.name}`}>
                                    <td className="name">
                                        {a.name}
                                        <small>{a.kind} · {a.namespace}</small>
                                    </td>
                                    <td className="mono text-[12.5px] text-ink-3">{a.source}</td>
                                    <td><StatusBadge status={a.status === "Synced" || a.status === "Ready" ? "Healthy" : a.status} /></td>
                                    <td className="num">{a.age}</td>
                                    <td>
                                        <div className="flex justify-end gap-1.5">
                                            <button
                                                type="button"
                                                className="btn btn-line btn-sm"
                                                disabled={syncing === `${a.namespace}/${a.name}`}
                                                onClick={() => sync(a)}
                                                title={t("pages:ops.gitops.sync")}
                                                aria-label={t("pages:ops.gitops.syncAria", { name: a.name })}
                                            >
                                                <RotateCcw size={13} />
                                            </button>
                                            {!isViewer && (
                                                <button
                                                    type="button"
                                                    className="btn btn-line btn-sm"
                                                    data-testid={GITOPS_EDIT.open(a.kind, a.namespace, a.name)}
                                                    title={t("pages:ops.gitops.editAria", { name: a.name })}
                                                    aria-label={t("pages:ops.gitops.editAria", { name: a.name })}
                                                    onClick={() => setEditing(a)}
                                                >
                                                    <Pencil size={13} />
                                                </button>
                                            )}
                                            {!isViewer && (
                                                <button
                                                    type="button"
                                                    className="btn btn-danger btn-sm"
                                                    data-testid={GITOPS_CREATE.deleteRow(a.kind, a.namespace, a.name)}
                                                    title={t("pages:ops.gitops.deleteAria", { name: a.name })}
                                                    aria-label={t("pages:ops.gitops.deleteAria", { name: a.name })}
                                                    onClick={() => setDeleting(a)}
                                                >
                                                    <Trash2 size={13} />
                                                </button>
                                            )}
                                        </div>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            />
        </div>
        <GitOpsCreateDialog open={createOpen} onOpenChange={setCreateOpen} onCreated={() => setRefresh((n) => n + 1)} />
        <GitOpsEditDialog app={editing} onClose={() => setEditing(null)} onSaved={() => setRefresh((n) => n + 1)} />
        <GitOpsCredentials open={credsOpen} onOpenChange={setCredsOpen} />
        <GitOpsDeleteDialog app={deleting} onClose={() => setDeleting(null)} onDeleted={() => setRefresh((n) => n + 1)} />
        </div>
    );
}

function AgentRunRow({ r, onChanged }) {
    const { t } = useTranslation();
    const [busy, setBusy] = useState("");
    const pending = r.approval === "Pending";

    const decide = async (action) => {
        setBusy(action);
        try {
            await api.post(`/agent-runs/${r.namespace}/${encodeURIComponent(r.name)}/${action}`);
            toast.success(action === "deny" ? t("toasts:ops.agents.denied", { name: r.name }) : t("toasts:ops.agents.approved", { name: r.name }));
            onChanged();
        } catch (e) {
            toast.error(action === "deny" ? t("toasts:ops.agents.denyFailed") : t("toasts:ops.agents.approveFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    return (
        <div className="list-row">
            <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                    <div className="mono text-[13.5px]">{r.name}</div>
                    <div className="mt-0.5 text-[12.5px] text-ink-3">{r.goal}</div>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                    <StatusBadge status={r.phase === "Proposed" ? "Pending" : r.phase} />
                    {pending && (
                        <>
                            <button
                                type="button"
                                className="btn btn-sm btn-success"
                                disabled={!!busy}
                                aria-label={t("pages:ops.agents.approve")}
                                title={t("pages:ops.agents.approve")}
                                onClick={() => decide("approve")}
                            >
                                <Check size={13} />
                            </button>
                            <button
                                type="button"
                                className="btn btn-danger btn-sm"
                                disabled={!!busy}
                                aria-label={t("pages:ops.agents.deny")}
                                title={t("pages:ops.agents.deny")}
                                onClick={() => decide("deny")}
                            >
                                <X size={13} />
                            </button>
                        </>
                    )}
                </div>
            </div>
            {r.plan?.summary && (
                <div className="mt-2 border-y border-line py-2 text-[12.5px] text-ink-3">
                    {r.plan.summary}
                </div>
            )}
            <div className="mt-2 flex items-center gap-2 text-[11.5px] text-ink-3">
                <span>{t("pages:ops.agents.autonomy", { value: r.autonomy })}</span>
                <span>·</span>
                <span>{t("pages:ops.agents.risk", { value: r.risk })}</span>
                <span>·</span>
                <span>{t("pages:ops.agents.approval", { value: r.approval })}</span>
            </div>
        </div>
    );
}

function AgentsTab() {
    const { t } = useTranslation();
    const [reload, setReload] = useState(0);
    const runs = useEndpoint("/agent-runs", [reload]);
    return (
        <div className="card p-0">
            <Rows
                items={runs}
                empty={t("pages:ops.agents.empty")}
                render={(r) => <AgentRunRow key={`${r.namespace}/${r.name}`} r={r} onChanged={() => setReload((n) => n + 1)} />}
            />
        </div>
    );
}

function IdentityTab() {
    const { t } = useTranslation();
    const identity = useEndpoint("/haven/identity");
    if (!identity) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    return (
        <div className="stack">
            <div className="card p-0">
                <div className="panel-h">{t("pages:ops.identity.planes")}</div>
                <TablePanel
                    items={identity.planes}
                    empty={t("pages:ops.identity.empty")}
                    table={(rows) => (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("pages:ops.identity.columns.plane")}</th>
                                    <th>{t("pages:ops.identity.columns.hostname")}</th>
                                    <th>{t("common:fields.status")}</th>
                                    <th>{t("common:fields.age")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map((p) => (
                                    <tr key={`${p.namespace}/${p.name}`}>
                                        <td className="name">
                                            {p.name}
                                            <small>{p.namespace} · {p.profile}</small>
                                        </td>
                                        <td className="mono text-[12.5px] text-ink-3">{p.hostname}</td>
                                        <td><StatusBadge status={p.phase} /></td>
                                        <td className="num">{p.age}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    )}
                />
            </div>
            <div className="card">
                <div className="mb-2 text-[14px] font-[500]">{t("pages:ops.identity.adminCredentials")}</div>
                <div className="text-[12.5px] text-ink-3">
                    {identity.admin?.exists ? t("pages:ops.identity.adminSecret", { name: identity.admin.secretName, namespace: identity.admin.namespace }) : t("common:states.notConfigured")}
                    {identity.admin?.hint && <span> — {identity.admin.hint}</span>}
                </div>
            </div>
        </div>
    );
}

function downloadBlob(filename, content, mime) {
    const blob = new Blob([content], { type: mime });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

function auditToCSV(entries) {
    const header = ["time", "action", "target", "detail", "user"];
    const escape = (v) => `"${String(v ?? "").replace(/"/g, '""')}"`;
    const rows = entries.map((e) => header.map((k) => escape(e[k])).join(","));
    return [header.join(","), ...rows].join("\n");
}

function AuditTab() {
    const { t } = useTranslation();
    const entries = useEndpoint("/audit");
    const [q, setQ] = useState("");

    const filtered = useMemo(() => {
        if (!entries) return entries;
        if (!q.trim()) return entries;
        const needle = q.toLowerCase();
        return entries.filter((e) =>
            [e.action, e.target, e.detail, e.user].some((f) => (f || "").toLowerCase().includes(needle))
        );
    }, [entries, q]);

    const exportCSV = () => {
        downloadBlob(`axiom-audit-${new Date().toISOString().slice(0, 10)}.csv`, auditToCSV(filtered || []), "text/csv");
    };
    const exportJSON = () => {
        downloadBlob(`axiom-audit-${new Date().toISOString().slice(0, 10)}.json`, JSON.stringify(filtered || [], null, 2), "application/json");
    };

    return (
        <div className="stack">
            <div className="flex items-center gap-2.5">
                <div className="relative flex-1 min-w-[220px]">
                    <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-ink-3" />
                    <Input
                        placeholder={t("pages:ops.audit.searchPlaceholder")}
                        value={q}
                        onChange={(e) => setQ(e.target.value)}
                        className="bg-muted/30 pl-[34px]"
                    />
                </div>
                <button type="button" className="btn btn-line btn-sm" onClick={exportCSV} disabled={!filtered || filtered.length === 0}>
                    <Download size={13} /> {t("pages:ops.audit.exportCsv")}
                </button>
                <button type="button" className="btn btn-line btn-sm" onClick={exportJSON} disabled={!filtered || filtered.length === 0}>
                    <Download size={13} /> {t("pages:ops.audit.exportJson")}
                </button>
            </div>
            <div className="card p-0">
                <TablePanel
                    items={filtered}
                    empty={q ? t("pages:ops.audit.noMatches") : t("pages:ops.audit.empty")}
                    table={(rows) => (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("pages:ops.audit.columns.action")}</th>
                                    <th>{t("common:fields.target")}</th>
                                    <th>{t("pages:ops.audit.columns.detail")}</th>
                                    <th>{t("pages:ops.audit.columns.user")}</th>
                                    <th>{t("common:fields.time")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map((e, i) => (
                                    <tr key={i}>
                                        <td className="mono text-[12.5px] text-tone-link">{e.action}</td>
                                        <td className="mono text-[12.5px]">{e.target}</td>
                                        <td className="text-[12.5px] text-ink-3">{e.detail}</td>
                                        <td className="text-[12.5px] text-ink-3">{e.user}</td>
                                        <td className="num">{relTime(e.time)}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    )}
                />
            </div>
        </div>
    );
}

function CreateRoleDialog({ open, onOpenChange, onCreated }) {
    const { t } = useTranslation();
    const [namespace, setNamespace] = useState("default");
    const [name, setName] = useState("");
    const [rulesText, setRulesText] = useState('[\n  {"apiGroups": [""], "resources": ["pods"], "verbs": ["get", "list"]}\n]');
    const [busy, setBusy] = useState(false);

    const submit = async () => {
        let rules;
        try {
            rules = JSON.parse(rulesText);
        } catch {
            toast.error(t("errors:ops.rulesJson"));
            return;
        }
        setBusy(true);
        try {
            await api.post("/rbac/roles", { namespace, name, rules });
            toast.success(t("toasts:ops.rbac.roleCreated", { name }));
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error(t("toasts:ops.rbac.createRoleFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-lg">
                <DialogHeader>
                    <DialogTitle>{t("dialogs:ops.createRole.title")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label htmlFor="ops-namespace-2" className="text-xs text-muted-foreground">{t("common:fields.namespace")}</Label>
                            <Input id="ops-namespace-2" value={namespace} onChange={(e) => setNamespace(e.target.value)} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="ops-name" className="text-xs text-muted-foreground">{t("common:fields.name")}</Label>
                            <Input id="ops-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="app-editor" className="bg-muted/30" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="ops-rules" className="text-xs text-muted-foreground">{t("dialogs:ops.createRole.rules")}</Label>
                        <MacTerm title="rules.json" bodyClassName="!p-0">
                            <Textarea id="ops-rules" value={rulesText} onChange={(e) => setRulesText(e.target.value)} className="macos-term-body mono min-h-[140px] resize-none rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0" spellCheck={false} />
                        </MacTerm>
                    </div>
                </div>
                <DialogFooter>
                    <button type="button" className="btn btn-fill" onClick={submit} disabled={busy || !name}>
                        {busy ? t("dialogs:ops.creating") : t("common:actions.create")}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function CreateRoleBindingDialog({ open, onOpenChange, onCreated }) {
    const { t } = useTranslation();
    const [namespace, setNamespace] = useState("default");
    const [name, setName] = useState("");
    const [roleName, setRoleName] = useState("");
    const [subjectsText, setSubjectsText] = useState('[\n  {"kind": "User", "name": "jane"}\n]');
    const [busy, setBusy] = useState(false);

    const submit = async () => {
        let subjects;
        try {
            subjects = JSON.parse(subjectsText);
        } catch {
            toast.error(t("errors:ops.subjectsJson"));
            return;
        }
        setBusy(true);
        try {
            await api.post("/rbac/rolebindings", { namespace, name, roleName, subjects });
            toast.success(t("toasts:ops.rbac.bindingCreated", { name }));
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error(t("toasts:ops.rbac.createBindingFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-lg">
                <DialogHeader>
                    <DialogTitle>{t("dialogs:ops.createBinding.title")}</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-3 gap-3">
                        <div className="space-y-1.5">
                            <Label htmlFor="ops-namespace-3" className="text-xs text-muted-foreground">{t("common:fields.namespace")}</Label>
                            <Input id="ops-namespace-3" value={namespace} onChange={(e) => setNamespace(e.target.value)} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="ops-name-2" className="text-xs text-muted-foreground">{t("common:fields.name")}</Label>
                            <Input id="ops-name-2" value={name} onChange={(e) => setName(e.target.value)} placeholder="app-editor-binding" className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="ops-role-name" className="text-xs text-muted-foreground">{t("dialogs:ops.createBinding.roleName")}</Label>
                            <Input id="ops-role-name" value={roleName} onChange={(e) => setRoleName(e.target.value)} placeholder="app-editor" className="bg-muted/30" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label htmlFor="ops-subjects" className="text-xs text-muted-foreground">{t("dialogs:ops.createBinding.subjects")}</Label>
                        <MacTerm title="subjects.json" bodyClassName="!p-0">
                            <Textarea id="ops-subjects" value={subjectsText} onChange={(e) => setSubjectsText(e.target.value)} className="macos-term-body mono min-h-[100px] resize-none rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0" spellCheck={false} />
                        </MacTerm>
                    </div>
                </div>
                <DialogFooter>
                    <button type="button" className="btn btn-fill" onClick={submit} disabled={busy || !name || !roleName}>
                        {busy ? t("dialogs:ops.creating") : t("common:actions.create")}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function RbacGroupCard({ title, items, deletable, onDelete, onCreate, createLabel }) {
    const { t } = useTranslation();
    return (
        <div className="card p-0">
            <div className="panel-h">
                <div className="text-[14px] font-[500]">{title}</div>
                {onCreate && (
                    <button type="button" className="btn btn-line btn-sm" onClick={onCreate}>
                        <Plus size={13} /> {createLabel}
                    </button>
                )}
            </div>
            <TablePanel
                items={items}
                empty={t("pages:ops.noneFound")}
                table={(rows) => (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{items && items[0] && items[0].rules != null ? t("pages:ops.rbac.columns.rules") : t("pages:ops.rbac.columns.subjects")}</th>
                                {deletable && <th><span className="sr-only">{t("common:fields.actions")}</span></th>}
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((r, i) => (
                                <tr key={i}>
                                    <td className="name">
                                        {r.name}
                                        {r.namespace && <small>{r.namespace}</small>}
                                        {r.roleRef && <small className="mono">{r.roleRef}</small>}
                                    </td>
                                    <td className="num">{r.rules != null ? t("pages:ops.rbac.rulesCount", { n: r.rules }) : t("pages:ops.rbac.subjectsCount", { n: r.subjects })}</td>
                                    {deletable && (
                                        <td>
                                            <div className="flex justify-end">
                                                <button type="button" className="btn btn-danger btn-sm" aria-label={t("common:actions.delete")} title={t("common:actions.delete")} onClick={() => onDelete(r)}>
                                                    <Trash2 size={13} />
                                                </button>
                                            </div>
                                        </td>
                                    )}
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            />
        </div>
    );
}

function RbacRolesView() {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const [refresh, setRefresh] = useState(0);
    const rbac = useEndpoint("/rbac", [refresh]);
    const [roleOpen, setRoleOpen] = useState(false);
    const [bindingOpen, setBindingOpen] = useState(false);
    const reload = () => setRefresh((n) => n + 1);

    if (!rbac) return <Skeleton className="h-48 rounded-2xl bg-muted" />;

    const deleteRole = async (r) => {
        const ok = await confirm({
            title: t("dialogs:ops.deleteRole.title", { name: r.name }),
            description: t("dialogs:ops.cannotUndo"),
            confirmLabel: t("common:actions.delete"),
        });
        if (!ok) return;
        try {
            await api.delete(`/rbac/roles/${r.namespace}/${encodeURIComponent(r.name)}`);
            toast.success(t("toasts:ops.rbac.deleted", { name: r.name }));
            reload();
        } catch (e) {
            toast.error(t("toasts:ops.rbac.deleteFailed"), { description: e?.response?.data?.error || e.message });
        }
    };
    const deleteRoleBinding = async (r) => {
        const ok = await confirm({
            title: t("dialogs:ops.deleteBinding.title", { name: r.name }),
            description: t("dialogs:ops.cannotUndo"),
            confirmLabel: t("common:actions.delete"),
        });
        if (!ok) return;
        try {
            await api.delete(`/rbac/rolebindings/${r.namespace}/${encodeURIComponent(r.name)}`);
            toast.success(t("toasts:ops.rbac.deleted", { name: r.name }));
            reload();
        } catch (e) {
            toast.error(t("toasts:ops.rbac.deleteFailed"), { description: e?.response?.data?.error || e.message });
        }
    };

    const groups = [
        { id: "clusterRoles", title: t("pages:ops.rbac.clusterRoles"), items: rbac.clusterRoles, deletable: false },
        { id: "clusterRoleBindings", title: t("pages:ops.rbac.clusterRoleBindings"), items: rbac.clusterRoleBindings, deletable: false },
        { id: "roles", title: t("pages:ops.rbac.roles"), items: rbac.roles, deletable: true, onDelete: deleteRole, onCreate: () => setRoleOpen(true), createLabel: t("pages:ops.rbac.newRole") },
        { id: "roleBindings", title: t("pages:ops.rbac.roleBindings"), items: rbac.roleBindings, deletable: true, onDelete: deleteRoleBinding, onCreate: () => setBindingOpen(true), createLabel: t("pages:ops.rbac.newBinding") },
    ];
    return (
        <div className="grid g2">
            {groups.map((g) => (
                <RbacGroupCard key={g.id} {...g} />
            ))}
            <CreateRoleDialog open={roleOpen} onOpenChange={setRoleOpen} onCreated={reload} />
            <CreateRoleBindingDialog open={bindingOpen} onOpenChange={setBindingOpen} onCreated={reload} />
        </div>
    );
}

// Access = the existing roles/bindings management plus the read-only
// "By subject" and "Who can…" views (components/RbacExplorer.jsx). The active
// view lives in ?view= so it survives reloads and can be linked.
const RBAC_VIEWS = [
    { key: "roles", labelKey: "pages:ops.rbac.views.roles", Panel: RbacRolesView },
    { key: "subjects", labelKey: "pages:ops.rbac.views.subjects", Panel: RbacBySubject },
    { key: "who-can", labelKey: "pages:ops.rbac.views.whoCan", Panel: RbacWhoCan },
];

function RbacTab() {
    const { t } = useTranslation();
    const [params, setParams] = useSearchParams();
    const active = RBAC_VIEWS.find((v) => v.key === params.get("view")) || RBAC_VIEWS[0];
    const Panel = active.Panel;
    return (
        <div className="stack">
            <div className="seg" role="group" aria-label={t("pages:ops.rbac.accessView")} data-testid={RBAC.viewSwitch}>
                {RBAC_VIEWS.map((v) => (
                    <button
                        key={v.key}
                        type="button"
                        aria-pressed={v.key === active.key}
                        data-testid={RBAC.viewTab(v.key)}
                        onClick={() => setParams(v.key === "roles" ? {} : { view: v.key }, { replace: true })}
                    >
                        {t(v.labelKey)}
                    </button>
                ))}
            </div>
            <Panel />
        </div>
    );
}

function WebhooksTab() {
    const { t } = useTranslation();
    const webhooks = useEndpoint("/admission-webhooks");
    if (!webhooks) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    const groups = [
        ["Validating", t("pages:ops.webhooks.validating"), webhooks.filter((w) => w.type === "Validating")],
        ["Mutating", t("pages:ops.webhooks.mutating"), webhooks.filter((w) => w.type === "Mutating")],
    ];
    return (
        <div className="stack">
            {groups.map(([type, title, items]) => (
                <div key={type} className="card p-0">
                    <div className="panel-h">{title}</div>
                    {items.length === 0 ? (
                        <div className="empty">{t("pages:ops.noneFound")}</div>
                    ) : (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("pages:ops.webhooks.columns.webhook")}</th>
                                    <th>{t("pages:ops.webhooks.columns.rules")}</th>
                                    <th>{t("pages:ops.webhooks.columns.failurePolicy")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {items.map((w, i) => (
                                    <tr key={i}>
                                        <td className="name">
                                            {w.webhookName}
                                            <small>{w.configName}</small>
                                        </td>
                                        <td className="mono wrap text-[11.5px] text-ink-3">{(w.rules || []).join(", ") || "—"}</td>
                                        <td>
                                            <span className="pill" style={{ "--c": w.failurePolicy === "Fail" ? "var(--amber)" : "var(--ink-3)" }}>
                                                <i aria-hidden="true" />
                                                {w.failurePolicy}
                                            </span>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    )}
                </div>
            ))}
        </div>
    );
}

function CostTab() {
    const { t } = useTranslation();
    const [windowSel, setWindowSel] = useState("30d");
    const cost = useEndpoint(`/cost?window=${windowSel}`, [windowSel]);
    if (!cost) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    const actual = cost.source === "opencost";
    return (
        <div className="stack">
            <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-2 text-[12.5px] text-ink-3" data-testid={OPENCOST.source}>
                    <span className="pill" style={{ "--c": actual ? "var(--green)" : "var(--blue)" }}>
                        <i aria-hidden="true" />
                        {actual ? t("pages:ops.cost.sourceActual") : t("pages:ops.cost.sourceEstimate")}
                    </span>
                    {cost.note ? <span data-testid={OPENCOST.note}>{cost.note}</span> : null}
                </div>
                {actual && (
                    <select aria-label={t("pages:ops.cost.window")} data-testid={OPENCOST.window} className="select" value={windowSel} onChange={(e) => setWindowSel(e.target.value)}>
                        <option value="7d">{t("pages:ops.cost.last7d")}</option>
                        <option value="14d">{t("pages:ops.cost.last14d")}</option>
                        <option value="30d">{t("pages:ops.cost.last30d")}</option>
                    </select>
                )}
            </div>
            <div className="stats c3">
                <div className="stat">
                    <b>${cost.totalMonthlyUsd}</b>
                    <span>{actual ? t("pages:ops.cost.totalMonthlyActual") : t("pages:ops.cost.totalMonthly")}</span>
                </div>
                {actual ? (
                    <div className="stat" data-testid={OPENCOST.idle}>
                        <b>${cost.idleMonthlyUsd ?? 0}</b>
                        <span>{t("pages:ops.cost.idle")}</span>
                    </div>
                ) : (
                    <div className="stat">
                        <b>
                            ${cost.cpuRatePerCore}
                            <small>{t("pages:ops.cost.perCore")}</small>
                        </b>
                        <span>{t("pages:ops.cost.cpuRate")}</span>
                    </div>
                )}
                {!actual && (
                    <div className="stat">
                        <b>
                            ${cost.memoryRatePerGi}
                            <small>/GiB</small>
                        </b>
                        <span>{t("pages:ops.cost.memoryRate")}</span>
                    </div>
                )}
            </div>
            <CostBudgets />
            <CostRightsizing />
            <div className="card p-0">
                <div className="panel-h">{t("pages:ops.cost.byNamespace")}</div>
                <TablePanel
                    items={cost.namespaces}
                    empty={t("pages:ops.cost.noNamespaceData")}
                    table={(rows) => (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("common:fields.namespace")}</th>
                                    <th>{t("common:fields.cpu")}</th>
                                    <th>{t("common:fields.memory")}</th>
                                    {actual && <th>{t("pages:ops.cost.columns.whereItGoes")}</th>}
                                    <th>{t("pages:ops.cost.columns.monthlyCost")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map((n) => (
                                    <tr key={n.namespace}>
                                        <td className="name">{n.namespace}</td>
                                        <td className="text-[12.5px] text-ink-3">{t("pages:ops.cost.cores", { n: n.cpuCores })}</td>
                                        <td className="text-[12.5px] text-ink-3">{n.memoryGi} GiB</td>
                                        {actual && (
                                            <td className="mono text-[11.5px] text-ink-3" data-testid={OPENCOST.breakdown(n.namespace)}>
                                                cpu ${n.cpuUsd ?? 0} · mem ${n.memoryUsd ?? 0} · storage ${n.storageUsd ?? 0} · net ${n.networkUsd ?? 0}
                                            </td>
                                        )}
                                        <td className="mono num">${n.monthlyUsd}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    )}
                />
            </div>
            <div className="card p-0">
                <div className="panel-h">{t("pages:ops.cost.byWorkload")}</div>
                <TablePanel
                    items={cost.workloads}
                    empty={t("pages:ops.cost.noWorkloadData")}
                    table={(rows) => (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("pages:ops.cost.columns.workload")}</th>
                                    <th>{t("common:fields.cpu")}</th>
                                    <th>{t("common:fields.memory")}</th>
                                    <th>{t("pages:ops.cost.columns.monthlyCost")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map((w) => (
                                    <tr key={`${w.namespace}/${w.workload}`}>
                                        <td className="name">
                                            {w.workload}
                                            <small>{w.namespace} · {w.kind}</small>
                                        </td>
                                        <td className="text-[12.5px] text-ink-3">{t("pages:ops.cost.cores", { n: w.cpuCores })}</td>
                                        <td className="text-[12.5px] text-ink-3">{w.memoryGi} GiB</td>
                                        <td className="mono num">${w.monthlyUsd}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    )}
                />
            </div>
        </div>
    );
}

const PANELS = {
    helm: HelmTab,
    gitops: GitOpsTab,
    agents: AgentsTab,
    identity: IdentityTab,
    audit: AuditTab,
    access: RbacTab,
    webhooks: WebhooksTab,
    cost: CostTab,
};

export default function Ops() {
    const { t } = useTranslation();
    const { tab } = useParams();
    const navigate = useNavigate();
    const active = TABS.some((tb) => tb.key === tab) ? tab : "helm";
    const Panel = PANELS[active];
    // runs after PageHeader's own title effect, so it wins and adds the tab name
    const activeTab = TABS.find((tb) => tb.key === active);
    usePageTitle(t("pages:ops.title"), activeTab ? t(activeTab.labelKey) : undefined);

    return (
        <div className="page">
            <PageHeader
                title={t("pages:ops.title")}
                description={t("pages:ops.description")}
            />

            <Tabs value={active} onValueChange={(v) => navigate(`/ops/${v}`)}>
                <TabsList className="tabs">
                    {TABS.map((tb) => (
                        <TabsTrigger key={tb.key} value={tb.key} className="gap-1.5">
                            <tb.icon className="h-3.5 w-3.5" /> {t(tb.labelKey)}
                        </TabsTrigger>
                    ))}
                </TabsList>
                <TabsContent value={active} className="mt-4">
                    <Panel />
                </TabsContent>
            </Tabs>
        </div>
    );
}
