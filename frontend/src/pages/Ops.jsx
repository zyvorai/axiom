import { usePageTitle } from "@/lib/pageTitle";
import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { formatDistanceToNowStrict } from "date-fns";
import { Anchor, GitBranch, Bot, Fingerprint, ScrollText, ShieldCheck, DollarSign, Trash2, RotateCcw, Check, X, Webhook, Plus, Search, Download } from "lucide-react";
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

const TABS = [
    { key: "helm", label: "Helm", icon: Anchor },
    { key: "gitops", label: "GitOps", icon: GitBranch },
    { key: "agents", label: "Agents", icon: Bot },
    { key: "identity", label: "Identity", icon: Fingerprint },
    { key: "audit", label: "Audit", icon: ScrollText },
    { key: "access", label: "Access", icon: ShieldCheck },
    { key: "webhooks", label: "Webhooks", icon: Webhook },
    { key: "cost", label: "Cost", icon: DollarSign },
];

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true });
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
    if (!items) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    if (items.length === 0) return <div className="empty">{empty}</div>;
    return <div className="max-h-[520px] overflow-auto" tabIndex={0} role="region" aria-label="Results">{items.map(render)}</div>;
}

// Renders a `.tbl` once `items` has rows; a loading skeleton or `.empty` block
// otherwise (kept outside any <table> so the DOM stays valid either way).
function TablePanel({ items, empty, table }) {
    if (!items) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    if (items.length === 0) return <div className="empty">{empty}</div>;
    return table(items);
}

function HelmRow({ r, onChanged }) {
    const confirm = useConfirm();
    const [revision, setRevision] = useState("");
    const [busy, setBusy] = useState("");

    const uninstall = async () => {
        const ok = await confirm({
            title: `Uninstall ${r.name}?`,
            description: "This cannot be undone.",
            confirmLabel: "Uninstall",
        });
        if (!ok) return;
        setBusy("uninstall");
        try {
            await api.post(`/helm/releases/${r.namespace}/${encodeURIComponent(r.name)}/uninstall`);
            toast.success(`${r.name} uninstalled`);
            onChanged();
        } catch (e) {
            toast.error("Uninstall failed", { description: e?.response?.data?.error || e.message });
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
            toast.success(`${r.name} rolled back`);
            onChanged();
        } catch (e) {
            toast.error("Rollback failed", { description: e?.response?.data?.error || e.message });
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
            <td className="num">rev {r.revision}</td>
            <td>
                <StatusBadge status={r.status === "deployed" ? "Healthy" : r.status} />
            </td>
            <td className="num">{r.age}</td>
            <td>
                <div className="flex items-center justify-end gap-1.5">
                    <Input
                        value={revision}
                        onChange={(e) => setRevision(e.target.value)}
                        placeholder="rev"
                        className="h-7 w-14 bg-muted/30 px-2 text-xs"
                    />
                    <button type="button" className="btn btn-line btn-sm" disabled={!!busy} onClick={rollback} title="Rollback">
                        <RotateCcw size={13} />
                    </button>
                    <button type="button" className="btn btn-danger btn-sm" disabled={!!busy} onClick={uninstall} title="Uninstall">
                        <Trash2 size={13} />
                    </button>
                </div>
            </td>
        </tr>
    );
}

function InstallChartDialog({ open, onOpenChange, onInstalled }) {
    const [form, setForm] = useState({ namespace: "default", releaseName: "", repoUrl: "", chartName: "", chartVersion: "", values: "" });
    const [busy, setBusy] = useState(false);
    const set = (patch) => setForm((f) => ({ ...f, ...patch }));

    const submit = async () => {
        setBusy(true);
        try {
            await api.post("/helm/install", form);
            toast.success(`Install started for ${form.releaseName}`);
            onOpenChange(false);
            onInstalled();
        } catch (e) {
            toast.error("Install failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-lg">
                <DialogHeader>
                    <DialogTitle>Install chart</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Namespace</Label>
                            <Input value={form.namespace} onChange={(e) => set({ namespace: e.target.value })} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Release name</Label>
                            <Input value={form.releaseName} onChange={(e) => set({ releaseName: e.target.value })} placeholder="my-release" className="bg-muted/30" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Chart repo URL</Label>
                        <Input value={form.repoUrl} onChange={(e) => set({ repoUrl: e.target.value })} placeholder="https://charts.example.com" className="bg-muted/30" />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Chart name</Label>
                            <Input value={form.chartName} onChange={(e) => set({ chartName: e.target.value })} placeholder="ingress-nginx" className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Version (optional)</Label>
                            <Input value={form.chartVersion} onChange={(e) => set({ chartVersion: e.target.value })} placeholder="4.11.0" className="bg-muted/30" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Values YAML (optional)</Label>
                        <MacTerm title="values.yaml" bodyClassName="!p-0">
                            <Textarea value={form.values} onChange={(e) => set({ values: e.target.value })} className="macos-term-body mono min-h-[100px] resize-none rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0" spellCheck={false} />
                        </MacTerm>
                    </div>
                    <p className="text-[11px] text-muted-foreground">
                        Runs as a background Job using Axiom's own permissions — the chart can only create resources Axiom itself is allowed to.
                    </p>
                </div>
                <DialogFooter>
                    <button type="button" className="btn btn-fill" onClick={submit} disabled={busy || !form.releaseName || !form.repoUrl || !form.chartName}>
                        {busy ? "Starting…" : "Install"}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function HelmTab() {
    const [reload, setReload] = useState(0);
    const releases = useEndpoint("/helm/releases", [reload]);
    const [installOpen, setInstallOpen] = useState(false);
    return (
        <div className="stack">
            <div className="flex justify-end">
                <button type="button" className="btn btn-fill btn-sm" onClick={() => setInstallOpen(true)}>
                    <Plus size={14} /> Install chart
                </button>
            </div>
            <div className="card p-0">
                <TablePanel
                    items={releases}
                    empty="No Helm releases."
                    table={(rows) => (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>Release</th>
                                    <th>Chart</th>
                                    <th>Revision</th>
                                    <th>Status</th>
                                    <th>Age</th>
                                    <th></th>
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
            <InstallChartDialog open={installOpen} onOpenChange={setInstallOpen} onInstalled={() => setReload((n) => n + 1)} />
        </div>
    );
}

function GitOpsTab() {
    const [refresh, setRefresh] = useState(0);
    const apps = useEndpoint("/gitops", [refresh]);
    const [syncing, setSyncing] = useState("");

    const sync = async (a) => {
        const busyKey = `${a.namespace}/${a.name}`;
        setSyncing(busyKey);
        try {
            await api.post(`/gitops/${a.kind}/${a.namespace}/${encodeURIComponent(a.name)}/sync`);
            toast.success(`${a.name}: sync requested`);
            setRefresh((n) => n + 1);
        } catch (e) {
            toast.error("Sync failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setSyncing("");
        }
    };

    return (
        <div className="card p-0">
            <TablePanel
                items={apps}
                empty="No GitOps applications."
                table={(rows) => (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Application</th>
                                <th>Source</th>
                                <th>Status</th>
                                <th>Age</th>
                                <th></th>
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
                                        <div className="flex justify-end">
                                            <button
                                                type="button"
                                                className="btn btn-line btn-sm"
                                                disabled={syncing === `${a.namespace}/${a.name}`}
                                                onClick={() => sync(a)}
                                                title="Sync"
                                            >
                                                <RotateCcw size={13} />
                                            </button>
                                        </div>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            />
        </div>
    );
}

function AgentRunRow({ r, onChanged }) {
    const [busy, setBusy] = useState("");
    const pending = r.approval === "Pending";

    const decide = async (action) => {
        setBusy(action);
        try {
            await api.post(`/agent-runs/${r.namespace}/${encodeURIComponent(r.name)}/${action}`);
            toast.success(`${r.name}: ${action}d`);
            onChanged();
        } catch (e) {
            toast.error(`${action} failed`, { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    return (
        <div className="panel-h">
            <div className="flex items-center justify-between gap-3">
                <div className="min-w-[0px]">
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
                                aria-label="Approve proposal" title="Approve proposal" onClick={() => decide("approve")}
                                
                            >
                                <Check size={13} />
                            </button>
                            <button type="button" className="btn btn-danger btn-sm" disabled={!!busy} aria-label="Deny proposal" title="Deny proposal" onClick={() => decide("deny")}>
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
                <span>autonomy: {r.autonomy}</span>
                <span>·</span>
                <span>risk: {r.risk}</span>
                <span>·</span>
                <span>approval: {r.approval}</span>
            </div>
        </div>
    );
}

function AgentsTab() {
    const [reload, setReload] = useState(0);
    const runs = useEndpoint("/agent-runs", [reload]);
    return (
        <div className="card p-0">
            <Rows
                items={runs}
                empty="No agent runs."
                render={(r) => <AgentRunRow key={`${r.namespace}/${r.name}`} r={r} onChanged={() => setReload((n) => n + 1)} />}
            />
        </div>
    );
}

function IdentityTab() {
    const identity = useEndpoint("/haven/identity");
    if (!identity) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    return (
        <div className="stack">
            <div className="card p-0">
                <div className="panel-h">Identity planes</div>
                <TablePanel
                    items={identity.planes}
                    empty="No identity planes."
                    table={(rows) => (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>Plane</th>
                                    <th>Hostname</th>
                                    <th>Status</th>
                                    <th>Age</th>
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
                <div className="mb-2 text-[14px] font-[500]">Admin credentials</div>
                <div className="text-[12.5px] text-ink-3">
                    {identity.admin?.exists ? `Secret ${identity.admin.secretName} in ${identity.admin.namespace}` : "Not configured"}
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

    return (
        <div className="stack">
            <div className="flex items-center gap-2.5">
                <div className="relative flex-1 min-w-[220px]">
                    <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-ink-3" />
                    <Input
                        placeholder="Search action, target, detail, or user"
                        value={q}
                        onChange={(e) => setQ(e.target.value)}
                        className="bg-muted/30 pl-[34px]"
                    />
                </div>
                <button type="button" className="btn btn-line btn-sm" onClick={exportCSV} disabled={!filtered || filtered.length === 0}>
                    <Download size={13} /> Export CSV
                </button>
            </div>
            <div className="card p-0">
                <TablePanel
                    items={filtered}
                    empty={q ? "No audit entries match." : "No audit entries recorded yet."}
                    table={(rows) => (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>Action</th>
                                    <th>Target</th>
                                    <th>Detail</th>
                                    <th>User</th>
                                    <th>Time</th>
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
    const [namespace, setNamespace] = useState("default");
    const [name, setName] = useState("");
    const [rulesText, setRulesText] = useState('[\n  {"apiGroups": [""], "resources": ["pods"], "verbs": ["get", "list"]}\n]');
    const [busy, setBusy] = useState(false);

    const submit = async () => {
        let rules;
        try {
            rules = JSON.parse(rulesText);
        } catch {
            toast.error("Rules must be valid JSON");
            return;
        }
        setBusy(true);
        try {
            await api.post("/rbac/roles", { namespace, name, rules });
            toast.success(`Role ${name} created`);
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error("Create role failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-lg">
                <DialogHeader>
                    <DialogTitle>New Role</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Namespace</Label>
                            <Input value={namespace} onChange={(e) => setNamespace(e.target.value)} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Name</Label>
                            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="app-editor" className="bg-muted/30" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Rules (JSON array of PolicyRule)</Label>
                        <MacTerm title="rules.json" bodyClassName="!p-0">
                            <Textarea value={rulesText} onChange={(e) => setRulesText(e.target.value)} className="macos-term-body mono min-h-[140px] resize-none rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0" spellCheck={false} />
                        </MacTerm>
                    </div>
                </div>
                <DialogFooter>
                    <button type="button" className="btn btn-fill" onClick={submit} disabled={busy || !name}>
                        {busy ? "Creating…" : "Create"}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function CreateRoleBindingDialog({ open, onOpenChange, onCreated }) {
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
            toast.error("Subjects must be valid JSON");
            return;
        }
        setBusy(true);
        try {
            await api.post("/rbac/rolebindings", { namespace, name, roleName, subjects });
            toast.success(`RoleBinding ${name} created`);
            onOpenChange(false);
            onCreated();
        } catch (e) {
            toast.error("Create role binding failed", { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-lg">
                <DialogHeader>
                    <DialogTitle>New RoleBinding</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-3 gap-3">
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Namespace</Label>
                            <Input value={namespace} onChange={(e) => setNamespace(e.target.value)} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Name</Label>
                            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="app-editor-binding" className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Role name</Label>
                            <Input value={roleName} onChange={(e) => setRoleName(e.target.value)} placeholder="app-editor" className="bg-muted/30" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Subjects (JSON array)</Label>
                        <MacTerm title="subjects.json" bodyClassName="!p-0">
                            <Textarea value={subjectsText} onChange={(e) => setSubjectsText(e.target.value)} className="macos-term-body mono min-h-[100px] resize-none rounded-none border-none px-4 py-3 text-xs leading-[1.55] focus-visible:ring-0" spellCheck={false} />
                        </MacTerm>
                    </div>
                </div>
                <DialogFooter>
                    <button type="button" className="btn btn-fill" onClick={submit} disabled={busy || !name || !roleName}>
                        {busy ? "Creating…" : "Create"}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function RbacGroupCard({ title, items, deletable, onDelete, onCreate, createLabel }) {
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
                empty="None found."
                table={(rows) => (
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th></th>
                                {deletable && <th></th>}
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
                                    <td className="num">{r.rules != null ? `${r.rules} rules` : `${r.subjects} subjects`}</td>
                                    {deletable && (
                                        <td>
                                            <div className="flex justify-end">
                                                <button type="button" className="btn btn-danger btn-sm" aria-label="Delete" title="Delete" onClick={() => onDelete(r)}>
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

function RbacTab() {
    const confirm = useConfirm();
    const [refresh, setRefresh] = useState(0);
    const rbac = useEndpoint("/rbac", [refresh]);
    const [roleOpen, setRoleOpen] = useState(false);
    const [bindingOpen, setBindingOpen] = useState(false);
    const reload = () => setRefresh((n) => n + 1);

    if (!rbac) return <Skeleton className="h-48 rounded-2xl bg-muted" />;

    const deleteRole = async (r) => {
        const ok = await confirm({
            title: `Delete role ${r.name}?`,
            description: "This cannot be undone.",
            confirmLabel: "Delete",
        });
        if (!ok) return;
        try {
            await api.delete(`/rbac/roles/${r.namespace}/${encodeURIComponent(r.name)}`);
            toast.success(`Deleted ${r.name}`);
            reload();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };
    const deleteRoleBinding = async (r) => {
        const ok = await confirm({
            title: `Delete role binding ${r.name}?`,
            description: "This cannot be undone.",
            confirmLabel: "Delete",
        });
        if (!ok) return;
        try {
            await api.delete(`/rbac/rolebindings/${r.namespace}/${encodeURIComponent(r.name)}`);
            toast.success(`Deleted ${r.name}`);
            reload();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };

    const groups = [
        { title: "Cluster roles", items: rbac.clusterRoles, deletable: false },
        { title: "Cluster role bindings", items: rbac.clusterRoleBindings, deletable: false },
        { title: "Roles", items: rbac.roles, deletable: true, onDelete: deleteRole, onCreate: () => setRoleOpen(true), createLabel: "New role" },
        { title: "Role bindings", items: rbac.roleBindings, deletable: true, onDelete: deleteRoleBinding, onCreate: () => setBindingOpen(true), createLabel: "New binding" },
    ];
    return (
        <div className="grid g2">
            {groups.map((g) => (
                <RbacGroupCard key={g.title} {...g} />
            ))}
            <CreateRoleDialog open={roleOpen} onOpenChange={setRoleOpen} onCreated={reload} />
            <CreateRoleBindingDialog open={bindingOpen} onOpenChange={setBindingOpen} onCreated={reload} />
        </div>
    );
}

function WebhooksTab() {
    const webhooks = useEndpoint("/admission-webhooks");
    if (!webhooks) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    const groups = [
        ["Validating", webhooks.filter((w) => w.type === "Validating")],
        ["Mutating", webhooks.filter((w) => w.type === "Mutating")],
    ];
    return (
        <div className="stack">
            {groups.map(([title, items]) => (
                <div key={title} className="card p-0">
                    <div className="panel-h">{title} webhooks</div>
                    {items.length === 0 ? (
                        <div className="empty">None found.</div>
                    ) : (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>Webhook</th>
                                    <th>Rules</th>
                                    <th>Failure policy</th>
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
    const cost = useEndpoint("/cost");
    if (!cost) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    return (
        <div className="stack">
            <div className="stats c3">
                <div className="stat">
                    <b>${cost.totalMonthlyUsd}</b>
                    <span>Total monthly cost</span>
                </div>
                <div className="stat">
                    <b>
                        ${cost.cpuRatePerCore}
                        <small>/core</small>
                    </b>
                    <span>CPU rate</span>
                </div>
                <div className="stat">
                    <b>
                        ${cost.memoryRatePerGi}
                        <small>/GiB</small>
                    </b>
                    <span>Memory rate</span>
                </div>
            </div>
            <div className="card p-0">
                <div className="panel-h">By namespace</div>
                <TablePanel
                    items={cost.namespaces}
                    empty="No namespace cost data."
                    table={(rows) => (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>Namespace</th>
                                    <th>CPU</th>
                                    <th>Memory</th>
                                    <th>Monthly cost</th>
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map((n) => (
                                    <tr key={n.namespace}>
                                        <td className="name">{n.namespace}</td>
                                        <td className="text-[12.5px] text-ink-3">{n.cpuCores} cores</td>
                                        <td className="text-[12.5px] text-ink-3">{n.memoryGi} GiB</td>
                                        <td className="mono num">${n.monthlyUsd}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    )}
                />
            </div>
            <div className="card p-0">
                <div className="panel-h">By workload</div>
                <TablePanel
                    items={cost.workloads}
                    empty="No workload cost data."
                    table={(rows) => (
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>Workload</th>
                                    <th>CPU</th>
                                    <th>Memory</th>
                                    <th>Monthly cost</th>
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map((w) => (
                                    <tr key={`${w.namespace}/${w.workload}`}>
                                        <td className="name">
                                            {w.workload}
                                            <small>{w.namespace} · {w.kind}</small>
                                        </td>
                                        <td className="text-[12.5px] text-ink-3">{w.cpuCores} cores</td>
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
    const { tab } = useParams();
    const navigate = useNavigate();
    const active = TABS.some((t) => t.key === tab) ? tab : "helm";
    const Panel = PANELS[active];
    // runs after PageHeader's own title effect, so it wins and adds the tab name
    usePageTitle("Ops", TABS.find((t) => t.key === active)?.label);

    return (
        <div className="page">
            <PageHeader
                title="Ops"
                description="Helm, GitOps, agents, identity, audit, access and cost."
            />

            <Tabs value={active} onValueChange={(v) => navigate(`/ops/${v}`)}>
                <TabsList className="tabs">
                    {TABS.map((t) => (
                        <TabsTrigger key={t.key} value={t.key} className="gap-1.5">
                            <t.icon className="h-3.5 w-3.5" /> {t.label}
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
