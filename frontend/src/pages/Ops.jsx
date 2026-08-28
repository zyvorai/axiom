import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { formatDistanceToNowStrict } from "date-fns";
import { Anchor, GitBranch, Bot, Fingerprint, ScrollText, ShieldCheck, DollarSign, Trash2, RotateCcw, Check, X, Webhook, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
    DialogFooter,
} from "@/components/ui/dialog";
import { toast } from "sonner";

const TABS = [
    { key: "helm", label: "Helm", icon: Anchor },
    { key: "gitops", label: "GitOps", icon: GitBranch },
    { key: "agents", label: "Agents", icon: Bot },
    { key: "identity", label: "Identity", icon: Fingerprint },
    { key: "audit", label: "Audit", icon: ScrollText },
    { key: "rbac", label: "Access", icon: ShieldCheck },
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

function Rows({ items, empty, render }) {
    if (!items) return <Skeleton className="h-48 rounded-2xl bg-white/[0.04]" />;
    if (items.length === 0) return <div className="px-4 py-8 text-center text-sm text-muted-foreground">{empty}</div>;
    return <div className="max-h-[520px] overflow-auto scrollbar-thin">{items.map(render)}</div>;
}

function HelmRow({ r, onChanged }) {
    const [revision, setRevision] = useState("");
    const [busy, setBusy] = useState("");

    const uninstall = async () => {
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
        <div className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]">
            <div className="col-span-3 min-w-0">
                <div className="mono truncate text-sm">{r.name}</div>
                <div className="text-[11px] text-muted-foreground">{r.namespace}</div>
            </div>
            <div className="col-span-3 mono truncate text-xs text-muted-foreground">{r.chart}</div>
            <div className="col-span-1 text-xs text-muted-foreground">rev {r.revision}</div>
            <div className="col-span-1">
                <StatusBadge status={r.status === "deployed" ? "Healthy" : r.status} />
            </div>
            <div className="col-span-1 text-right text-xs text-muted-foreground">{r.age}</div>
            <div className="col-span-3 flex items-center justify-end gap-1.5">
                <Input
                    value={revision}
                    onChange={(e) => setRevision(e.target.value)}
                    placeholder="rev"
                    className="h-7 w-14 border-white/10 bg-white/[0.03] px-2 text-xs"
                />
                <Button size="sm" variant="outline" disabled={!!busy} onClick={rollback} className="h-7 rounded-lg border-white/10 bg-white/[0.03] px-2">
                    <RotateCcw className="h-3.5 w-3.5" />
                </Button>
                <Button size="sm" variant="outline" disabled={!!busy} onClick={uninstall} className="h-7 rounded-lg border-rose-500/30 bg-rose-500/10 px-2 text-rose-300 hover:bg-rose-500/20">
                    <Trash2 className="h-3.5 w-3.5" />
                </Button>
            </div>
        </div>
    );
}

function HelmTab() {
    const [reload, setReload] = useState(0);
    const releases = useEndpoint("/helm/releases", [reload]);
    return (
        <GlassCard className="p-0 overflow-hidden">
            <Rows
                items={releases}
                empty="No Helm releases."
                render={(r) => <HelmRow key={`${r.namespace}/${r.name}`} r={r} onChanged={() => setReload((n) => n + 1)} />}
            />
        </GlassCard>
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
        <GlassCard className="p-0 overflow-hidden">
            <Rows
                items={apps}
                empty="No GitOps applications."
                render={(a) => (
                    <div key={`${a.namespace}/${a.name}`} className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]">
                        <div className="col-span-4 min-w-0">
                            <div className="mono truncate text-sm">{a.name}</div>
                            <div className="text-[11px] text-muted-foreground">{a.kind} · {a.namespace}</div>
                        </div>
                        <div className="col-span-4 mono truncate text-xs text-muted-foreground">{a.source}</div>
                        <div className="col-span-2"><StatusBadge status={a.status === "Synced" || a.status === "Ready" ? "Healthy" : a.status} /></div>
                        <div className="col-span-1 text-right text-xs text-muted-foreground">{a.age}</div>
                        <div className="col-span-1 flex justify-end">
                            <Button
                                size="sm"
                                variant="ghost"
                                disabled={syncing === `${a.namespace}/${a.name}`}
                                onClick={() => sync(a)}
                                className="h-7 px-2 text-muted-foreground hover:text-[#6ab6f0]"
                                title="Sync"
                            >
                                <RotateCcw className="h-3.5 w-3.5" />
                            </Button>
                        </div>
                    </div>
                )}
            />
        </GlassCard>
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
        <div className="border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]">
            <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                    <div className="mono truncate text-sm">{r.name}</div>
                    <div className="mt-0.5 text-xs text-muted-foreground">{r.goal}</div>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                    <StatusBadge status={r.phase === "Proposed" ? "Pending" : r.phase} />
                    {pending && (
                        <>
                            <Button size="sm" disabled={!!busy} onClick={() => decide("approve")} className="h-7 rounded-lg bg-emerald-500/15 px-2 text-emerald-300 hover:bg-emerald-500/25">
                                <Check className="h-3.5 w-3.5" />
                            </Button>
                            <Button size="sm" disabled={!!busy} onClick={() => decide("deny")} className="h-7 rounded-lg bg-rose-500/15 px-2 text-rose-300 hover:bg-rose-500/25">
                                <X className="h-3.5 w-3.5" />
                            </Button>
                        </>
                    )}
                </div>
            </div>
            {r.plan?.summary && (
                <div className="mt-2 rounded-lg border border-white/5 bg-white/[0.02] px-3 py-2 text-xs text-muted-foreground">
                    {r.plan.summary}
                </div>
            )}
            <div className="mt-2 flex items-center gap-3 text-[11px] text-muted-foreground">
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
        <GlassCard className="p-0 overflow-hidden">
            <Rows
                items={runs}
                empty="No agent runs."
                render={(r) => <AgentRunRow key={`${r.namespace}/${r.name}`} r={r} onChanged={() => setReload((n) => n + 1)} />}
            />
        </GlassCard>
    );
}

function IdentityTab() {
    const identity = useEndpoint("/haven/identity");
    if (!identity) return <Skeleton className="h-48 rounded-2xl bg-white/[0.04]" />;
    return (
        <div className="space-y-4">
            <GlassCard className="p-0 overflow-hidden">
                <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">Identity planes</div>
                <Rows
                    items={identity.planes}
                    empty="No identity planes."
                    render={(p) => (
                        <div key={`${p.namespace}/${p.name}`} className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]">
                            <div className="col-span-4 min-w-0">
                                <div className="mono truncate text-sm">{p.name}</div>
                                <div className="text-[11px] text-muted-foreground">{p.namespace} · {p.profile}</div>
                            </div>
                            <div className="col-span-5 mono truncate text-xs text-muted-foreground">{p.hostname}</div>
                            <div className="col-span-2"><StatusBadge status={p.phase} /></div>
                            <div className="col-span-1 text-right text-xs text-muted-foreground">{p.age}</div>
                        </div>
                    )}
                />
            </GlassCard>
            <GlassCard className="p-4 text-sm">
                <div className="mb-2 font-medium">Admin credentials</div>
                <div className="text-xs text-muted-foreground">
                    {identity.admin?.exists ? `Secret ${identity.admin.secretName} in ${identity.admin.namespace}` : "Not configured"}
                    {identity.admin?.hint && <span> — {identity.admin.hint}</span>}
                </div>
            </GlassCard>
        </div>
    );
}

function AuditTab() {
    const entries = useEndpoint("/audit");
    return (
        <GlassCard className="p-0 overflow-hidden">
            <Rows
                items={entries}
                empty="No audit entries recorded yet."
                render={(e, i) => (
                    <div key={i} className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]">
                        <div className="col-span-2 mono text-xs text-sky-300">{e.action}</div>
                        <div className="col-span-5 mono truncate text-xs text-foreground/90">{e.target}</div>
                        <div className="col-span-3 truncate text-xs text-muted-foreground">{e.detail}</div>
                        <div className="col-span-1 text-xs text-muted-foreground">{e.user}</div>
                        <div className="col-span-1 text-right text-xs text-muted-foreground">{relTime(e.time)}</div>
                    </div>
                )}
            />
        </GlassCard>
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
            <DialogContent className="glass max-w-lg border-white/10">
                <DialogHeader>
                    <DialogTitle>New Role</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Namespace</Label>
                            <Input value={namespace} onChange={(e) => setNamespace(e.target.value)} className="border-white/10 bg-white/[0.03]" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Name</Label>
                            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="app-editor" className="border-white/10 bg-white/[0.03]" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Rules (JSON array of PolicyRule)</Label>
                        <Textarea value={rulesText} onChange={(e) => setRulesText(e.target.value)} className="min-h-[140px] border-white/10 bg-white/[0.03] mono text-xs" spellCheck={false} />
                    </div>
                </div>
                <DialogFooter>
                    <Button onClick={submit} disabled={busy || !name} className="rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white">
                        {busy ? "Creating…" : "Create"}
                    </Button>
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
            <DialogContent className="glass max-w-lg border-white/10">
                <DialogHeader>
                    <DialogTitle>New RoleBinding</DialogTitle>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="grid grid-cols-3 gap-3">
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Namespace</Label>
                            <Input value={namespace} onChange={(e) => setNamespace(e.target.value)} className="border-white/10 bg-white/[0.03]" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Name</Label>
                            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="app-editor-binding" className="border-white/10 bg-white/[0.03]" />
                        </div>
                        <div className="space-y-1.5">
                            <Label className="text-xs text-muted-foreground">Role name</Label>
                            <Input value={roleName} onChange={(e) => setRoleName(e.target.value)} placeholder="app-editor" className="border-white/10 bg-white/[0.03]" />
                        </div>
                    </div>
                    <div className="space-y-1.5">
                        <Label className="text-xs text-muted-foreground">Subjects (JSON array)</Label>
                        <Textarea value={subjectsText} onChange={(e) => setSubjectsText(e.target.value)} className="min-h-[100px] border-white/10 bg-white/[0.03] mono text-xs" spellCheck={false} />
                    </div>
                </div>
                <DialogFooter>
                    <Button onClick={submit} disabled={busy || !name || !roleName} className="rounded-xl bg-gradient-to-r from-[#ff7550] to-[#6ab6f0] text-white">
                        {busy ? "Creating…" : "Create"}
                    </Button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

function RbacTab() {
    const [refresh, setRefresh] = useState(0);
    const rbac = useEndpoint("/rbac", [refresh]);
    const [roleOpen, setRoleOpen] = useState(false);
    const [bindingOpen, setBindingOpen] = useState(false);
    const reload = () => setRefresh((n) => n + 1);

    if (!rbac) return <Skeleton className="h-48 rounded-2xl bg-white/[0.04]" />;

    const deleteRole = async (r) => {
        try {
            await api.delete(`/rbac/roles/${r.namespace}/${encodeURIComponent(r.name)}`);
            toast.success(`Deleted ${r.name}`);
            reload();
        } catch (e) {
            toast.error("Delete failed", { description: e?.response?.data?.error || e.message });
        }
    };
    const deleteRoleBinding = async (r) => {
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
        <div className="grid gap-4 lg:grid-cols-2">
            {groups.map((g) => (
                <GlassCard key={g.title} className="p-0 overflow-hidden">
                    <div className="flex items-center justify-between border-b border-white/5 px-4 py-3">
                        <div className="text-sm font-medium">{g.title}</div>
                        {g.onCreate && (
                            <Button size="sm" variant="outline" onClick={g.onCreate} className="h-7 rounded-lg border-white/10 bg-white/[0.03] text-xs">
                                <Plus className="mr-1 h-3 w-3" /> {g.createLabel}
                            </Button>
                        )}
                    </div>
                    <Rows
                        items={g.items}
                        empty="None found."
                        render={(r, i) => (
                            <div key={i} className="flex items-center justify-between gap-3 border-b border-white/5 px-4 py-2.5 last:border-none hover:bg-white/[0.03]">
                                <div className="min-w-0">
                                    <div className="mono truncate text-sm">{r.name}</div>
                                    {r.namespace && <div className="text-[11px] text-muted-foreground">{r.namespace}</div>}
                                    {r.roleRef && <div className="mono text-[11px] text-muted-foreground">{r.roleRef}</div>}
                                </div>
                                <div className="flex items-center gap-2">
                                    <div className="mono text-xs text-muted-foreground">
                                        {r.rules != null ? `${r.rules} rules` : `${r.subjects} subjects`}
                                    </div>
                                    {g.deletable && (
                                        <Button size="sm" variant="ghost" onClick={() => g.onDelete(r)} className="h-7 px-2 text-muted-foreground hover:text-rose-400">
                                            <Trash2 className="h-3.5 w-3.5" />
                                        </Button>
                                    )}
                                </div>
                            </div>
                        )}
                    />
                </GlassCard>
            ))}
            <CreateRoleDialog open={roleOpen} onOpenChange={setRoleOpen} onCreated={reload} />
            <CreateRoleBindingDialog open={bindingOpen} onOpenChange={setBindingOpen} onCreated={reload} />
        </div>
    );
}

function WebhooksTab() {
    const webhooks = useEndpoint("/admission-webhooks");
    if (!webhooks) return <Skeleton className="h-48 rounded-2xl bg-white/[0.04]" />;
    const groups = [
        ["Validating", webhooks.filter((w) => w.type === "Validating")],
        ["Mutating", webhooks.filter((w) => w.type === "Mutating")],
    ];
    return (
        <div className="grid gap-4 lg:grid-cols-2">
            {groups.map(([title, items]) => (
                <GlassCard key={title} className="p-0 overflow-hidden">
                    <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">{title} webhooks</div>
                    <Rows
                        items={items}
                        empty="None found."
                        render={(w, i) => (
                            <div key={i} className="border-b border-white/5 px-4 py-2.5 last:border-none hover:bg-white/[0.03]">
                                <div className="flex items-center justify-between gap-3">
                                    <div className="mono truncate text-sm">{w.webhookName}</div>
                                    <span className={`text-[11px] ${w.failurePolicy === "Fail" ? "text-amber-400" : "text-muted-foreground"}`}>
                                        {w.failurePolicy}
                                    </span>
                                </div>
                                <div className="text-[11px] text-muted-foreground">{w.configName}</div>
                                {(w.rules || []).length > 0 && (
                                    <div className="mono mt-1 truncate text-[11px] text-muted-foreground">{w.rules.join(", ")}</div>
                                )}
                            </div>
                        )}
                    />
                </GlassCard>
            ))}
        </div>
    );
}

function CostTab() {
    const cost = useEndpoint("/cost");
    if (!cost) return <Skeleton className="h-48 rounded-2xl bg-white/[0.04]" />;
    return (
        <div className="space-y-4">
            <div className="grid gap-4 md:grid-cols-3">
                <GlassCard className="p-4">
                    <div className="text-xs uppercase tracking-widest text-muted-foreground">Total monthly cost</div>
                    <div className="mt-2 text-3xl font-semibold tabular-nums">${cost.totalMonthlyUsd}</div>
                </GlassCard>
                <GlassCard className="p-4">
                    <div className="text-xs uppercase tracking-widest text-muted-foreground">CPU rate</div>
                    <div className="mt-2 text-3xl font-semibold tabular-nums">${cost.cpuRatePerCore}<span className="text-sm text-muted-foreground">/core</span></div>
                </GlassCard>
                <GlassCard className="p-4">
                    <div className="text-xs uppercase tracking-widest text-muted-foreground">Memory rate</div>
                    <div className="mt-2 text-3xl font-semibold tabular-nums">${cost.memoryRatePerGi}<span className="text-sm text-muted-foreground">/GiB</span></div>
                </GlassCard>
            </div>
            <GlassCard className="p-0 overflow-hidden">
                <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">By namespace</div>
                <Rows
                    items={cost.namespaces}
                    empty="No namespace cost data."
                    render={(n) => (
                        <div key={n.namespace} className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]">
                            <div className="col-span-4 mono text-sm">{n.namespace}</div>
                            <div className="col-span-3 text-xs text-muted-foreground">{n.cpuCores} cores</div>
                            <div className="col-span-3 text-xs text-muted-foreground">{n.memoryGi} GiB</div>
                            <div className="col-span-2 text-right mono text-sm">${n.monthlyUsd}</div>
                        </div>
                    )}
                />
            </GlassCard>
        </div>
    );
}

const PANELS = {
    helm: HelmTab,
    gitops: GitOpsTab,
    agents: AgentsTab,
    identity: IdentityTab,
    audit: AuditTab,
    rbac: RbacTab,
    webhooks: WebhooksTab,
    cost: CostTab,
};

export default function Ops() {
    const { tab } = useParams();
    const navigate = useNavigate();
    const active = TABS.some((t) => t.key === tab) ? tab : "helm";
    const Panel = PANELS[active];

    return (
        <div className="space-y-4">
            <div>
                <h1 className="text-3xl font-semibold tracking-tight">Ops</h1>
                <p className="text-sm text-muted-foreground">Helm, GitOps, agents, identity, audit, access and cost.</p>
            </div>

            <Tabs value={active} onValueChange={(v) => navigate(`/ops/${v}`)}>
                <TabsList className="h-auto flex-wrap gap-1 bg-white/[0.03] p-1">
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
