import { useEffect, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { formatDistanceToNowStrict } from "date-fns";
import { Anchor, GitBranch, Bot, Fingerprint, ScrollText, ShieldCheck, DollarSign, Trash2, RotateCcw, Check, X, Webhook } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
    const apps = useEndpoint("/gitops");
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
                        <div className="col-span-5 mono truncate text-xs text-muted-foreground">{a.source}</div>
                        <div className="col-span-2"><StatusBadge status={a.status === "Synced" || a.status === "Ready" ? "Healthy" : a.status} /></div>
                        <div className="col-span-1 text-right text-xs text-muted-foreground">{a.age}</div>
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

function RbacTab() {
    const rbac = useEndpoint("/rbac");
    if (!rbac) return <Skeleton className="h-48 rounded-2xl bg-white/[0.04]" />;
    const groups = [
        ["Cluster roles", rbac.clusterRoles],
        ["Cluster role bindings", rbac.clusterRoleBindings],
        ["Roles", rbac.roles],
        ["Role bindings", rbac.roleBindings],
    ];
    return (
        <div className="grid gap-4 lg:grid-cols-2">
            {groups.map(([title, items]) => (
                <GlassCard key={title} className="p-0 overflow-hidden">
                    <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">{title}</div>
                    <Rows
                        items={items}
                        empty="None found."
                        render={(r, i) => (
                            <div key={i} className="flex items-center justify-between gap-3 border-b border-white/5 px-4 py-2.5 last:border-none hover:bg-white/[0.03]">
                                <div className="min-w-0">
                                    <div className="mono truncate text-sm">{r.name}</div>
                                    {r.namespace && <div className="text-[11px] text-muted-foreground">{r.namespace}</div>}
                                    {r.roleRef && <div className="mono text-[11px] text-muted-foreground">{r.roleRef}</div>}
                                </div>
                                <div className="mono text-xs text-muted-foreground">
                                    {r.rules != null ? `${r.rules} rules` : `${r.subjects} subjects`}
                                </div>
                            </div>
                        )}
                    />
                </GlassCard>
            ))}
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
