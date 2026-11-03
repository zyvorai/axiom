import { useCallback, useEffect, useMemo, useState } from "react";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useConfirm } from "@/components/ConfirmDialog";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { COST as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || "Something went wrong";
const usd = (v) => `$${Number(v || 0).toLocaleString(undefined, { maximumFractionDigits: 2 })}`;

const STATE_TONE = { ok: "var(--green)", warning: "var(--orange)", over: "var(--red)" };
const STATE_LABEL = { ok: "On track", warning: "Near budget", over: "Over budget" };

function BudgetDialog({ open, onOpenChange, initial, onSaved }) {
    const [ns, setNs] = useState("");
    const [amount, setAmount] = useState("");
    const [warn, setWarn] = useState("80");
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        if (open) {
            setNs(initial?.namespace || "");
            setAmount(initial?.monthlyUsd ? String(initial.monthlyUsd) : "");
            setWarn(String(initial?.warnPercent || 80));
        }
    }, [open, initial]);

    const save = async () => {
        setBusy(true);
        try {
            await api.put(`/cost/budgets/${encodeURIComponent(ns.trim())}`, { monthlyUsd: Number(amount), warnPercent: Number(warn) || 80 });
            toast.success(`Budget saved for ${ns.trim()}`);
            onSaved();
            onOpenChange(false);
        } catch (e) {
            toast.error("Could not save the budget", { description: errText(e) });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-sm" data-testid={T.budgetDialog}>
                <DialogHeader>
                    <DialogTitle>{initial?.namespace ? `Budget for ${initial.namespace}` : "Add a budget"}</DialogTitle>
                    <DialogDescription>Compared with the estimated monthly cost of the namespace&apos;s resource requests.</DialogDescription>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label htmlFor="cost-budget-ns" className="text-xs text-muted-foreground">Namespace</Label>
                        <Input id="cost-budget-ns" data-testid={T.budgetNamespace} value={ns} onChange={(e) => setNs(e.target.value)} disabled={!!initial?.namespace} placeholder="payments" className="bg-muted/30" />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label htmlFor="cost-budget-amount" className="text-xs text-muted-foreground">Monthly budget (USD)</Label>
                            <Input id="cost-budget-amount" data-testid={T.budgetAmount} type="number" min={1} value={amount} onChange={(e) => setAmount(e.target.value)} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="cost-budget-warn" className="text-xs text-muted-foreground">Warn at (%)</Label>
                            <Input id="cost-budget-warn" data-testid={T.budgetWarn} type="number" min={1} max={100} value={warn} onChange={(e) => setWarn(e.target.value)} className="bg-muted/30" />
                        </div>
                    </div>
                </div>
                <DialogFooter>
                    <button type="button" className="btn btn-fill" data-testid={T.budgetSave} onClick={save} disabled={busy || !ns.trim() || !(Number(amount) > 0)}>
                        {busy ? "Saving…" : "Save budget"}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

/** Per-namespace monthly budgets against the estimated cost. */
export function CostBudgets() {
    const { isViewer } = useAuth();
    const confirm = useConfirm();
    const [data, setData] = useState(null);
    const [error, setError] = useState("");
    const [dialog, setDialog] = useState({ open: false, initial: null });

    const load = useCallback(async () => {
        try {
            const { data: d } = await api.get("/cost/budgets");
            setData(d);
            setError("");
        } catch (e) {
            setError(errText(e));
        }
    }, []);
    useEffect(() => {
        load();
    }, [load]);

    const remove = async (b) => {
        const ok = await confirm({ title: `Remove the budget for ${b.namespace}?`, description: "The namespace's cost is still tracked; it just has no limit.", confirmLabel: "Remove" });
        if (!ok) return;
        try {
            await api.delete(`/cost/budgets/${encodeURIComponent(b.namespace)}`);
            toast.success(`Removed the budget for ${b.namespace}`);
            load();
        } catch (e) {
            toast.error("Remove failed", { description: errText(e) });
        }
    };

    if (error) return <div className="notice danger" role="alert">{error}</div>;
    if (!data) return <Skeleton className="h-40 rounded-2xl bg-muted" />;

    return (
        <div className="card p-0" data-testid={T.budgets}>
            <div className="panel-h flex items-center justify-between">
                <span>Budgets</span>
                {!isViewer && (
                    <button type="button" className="btn btn-line btn-sm" data-testid={T.addBudget} onClick={() => setDialog({ open: true, initial: null })}>
                        <Plus size={13} /> Add budget
                    </button>
                )}
            </div>
            {data.budgets.length === 0 ? (
                <div className="empty">No budgets yet. Set one per namespace to be told when its estimated cost nears the limit.</div>
            ) : (
                <ul className="divide-y divide-border">
                    {data.budgets.map((b) => {
                        const pct = Math.min(100, b.percent);
                        return (
                            <li key={b.namespace} className="px-4 py-3" data-testid={T.budgetRow(b.namespace)}>
                                <div className="flex items-center justify-between gap-3">
                                    <div className="min-w-0">
                                        <span className="font-medium">{b.namespace}</span>
                                        <span className="ml-2 text-[12.5px] text-ink-3">
                                            {usd(b.currentUsd)} of {usd(b.monthlyUsd)} · {b.percent}%
                                        </span>
                                    </div>
                                    <div className="flex shrink-0 items-center gap-2">
                                        <span className="pill" data-testid={T.budgetState(b.namespace)} style={{ "--c": STATE_TONE[b.state] }}>
                                            <i aria-hidden="true" />
                                            {STATE_LABEL[b.state] || b.state}
                                        </span>
                                        {!isViewer && (
                                            <>
                                                <button type="button" className="btn btn-line btn-sm" data-testid={T.budgetEdit(b.namespace)} aria-label={`Edit the budget for ${b.namespace}`} onClick={() => setDialog({ open: true, initial: b })}>
                                                    <Pencil size={13} />
                                                </button>
                                                <button type="button" className="btn btn-danger btn-sm" data-testid={T.budgetRemove(b.namespace)} aria-label={`Remove the budget for ${b.namespace}`} onClick={() => remove(b)}>
                                                    <Trash2 size={13} />
                                                </button>
                                            </>
                                        )}
                                    </div>
                                </div>
                                <div className="meter !mt-2" role="img" aria-label={`${b.namespace} is at ${b.percent}% of its budget`} data-testid={T.budgetBar(b.namespace)}>
                                    <i style={{ width: `${pct}%`, background: STATE_TONE[b.state] }} />
                                </div>
                            </li>
                        );
                    })}
                </ul>
            )}
            {data.budgets.length > 0 && (
                <p className="border-t border-border px-4 py-2 text-xs text-ink-3" data-testid={T.totals}>
                    {usd(data.totalCurrentUsd)} estimated across budgeted namespaces, of {usd(data.totalBudgetUsd)} budgeted.
                </p>
            )}
            {(data.unbudgeted || []).length > 0 && !isViewer && (
                <div className="border-t border-border px-4 py-3">
                    <div className="mb-1.5 text-xs text-ink-3">Without a budget</div>
                    <div className="flex flex-wrap gap-1.5">
                        {data.unbudgeted.slice(0, 8).map((u) => (
                            <button key={u.namespace} type="button" className="pill soft" data-testid={T.setBudget(u.namespace)} onClick={() => setDialog({ open: true, initial: { namespace: u.namespace, monthlyUsd: 0, warnPercent: 80 } })}>
                                <span data-testid={T.unbudgeted(u.namespace)}>{u.namespace} · {usd(u.currentUsd)}</span>
                            </button>
                        ))}
                    </div>
                </div>
            )}
            <BudgetDialog open={dialog.open} onOpenChange={(o) => setDialog((d) => ({ ...d, open: o }))} initial={dialog.initial} onSaved={load} />
        </div>
    );
}

const VERDICT = {
    over: { tone: "var(--orange)", label: "Over-provisioned" },
    under: { tone: "var(--red)", label: "Under-provisioned" },
    ok: { tone: "var(--green)", label: "Right-sized" },
};
const CONFIDENCE_NOTE = { high: "High confidence", medium: "Medium confidence", low: "Low confidence" };

/** Request-vs-usage recommendations. A recommendation only; Axiom never changes requests itself. */
export function CostRightsizing() {
    const [windowSel, setWindowSel] = useState("7d");
    const [filter, setFilter] = useState("actionable");
    const [data, setData] = useState(null);
    const [error, setError] = useState("");

    useEffect(() => {
        let alive = true;
        setData(null);
        api
            .get("/cost/rightsizing", { params: { window: windowSel } })
            .then((r) => alive && (setData(r.data), setError("")))
            .catch((e) => alive && setError(errText(e)));
        return () => {
            alive = false;
        };
    }, [windowSel]);

    const items = useMemo(() => {
        const all = data?.items || [];
        return filter === "actionable" ? all.filter((i) => i.verdict !== "ok") : all;
    }, [data, filter]);

    if (error) return <div className="notice danger" role="alert">{error}</div>;
    if (!data) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    const s = data.summary || {};

    return (
        <div className="card p-0" data-testid={T.rightsizing}>
            <div className="panel-h flex flex-wrap items-center justify-between gap-2">
                <span>Rightsizing</span>
                <div className="flex items-center gap-2">
                    <select aria-label="Show" className="select" value={filter} onChange={(e) => setFilter(e.target.value)}>
                        <option value="actionable">Needs attention</option>
                        <option value="all">All containers</option>
                    </select>
                    <select aria-label="Usage window" data-testid={T.window} className="select" value={windowSel} onChange={(e) => setWindowSel(e.target.value)}>
                        {["1d", "3d", "7d", "14d", "30d"].map((w) => (
                            <option key={w} value={w}>Last {w}</option>
                        ))}
                    </select>
                </div>
            </div>
            <div className="border-b border-border px-4 py-2.5 text-[12.5px] text-ink-3" data-testid={T.source}>
                <b className="text-ink-2">{data.source === "none" ? "No usage data" : `Source: ${data.source}`}</b>
                {data.confidence ? ` · ${CONFIDENCE_NOTE[data.confidence] || data.confidence}` : ""}
                {data.note ? ` — ${data.note}` : ""}
            </div>
            <div className="stats c3 !m-3">
                <div className="stat" data-testid={T.summary("savings")}>
                    <b>{usd(Math.abs(s.monthlySavingsUsd || 0))}</b>
                    <span>Potential monthly savings</span>
                </div>
                <div className="stat" data-testid={T.summary("over")}>
                    <b>{s.overProvisioned || 0}</b>
                    <span>Over-provisioned</span>
                </div>
                <div className="stat" data-testid={T.summary("under")}>
                    <b>{s.underProvisioned || 0}</b>
                    <span>At risk (under-provisioned)</span>
                </div>
            </div>
            {items.length === 0 ? (
                <div className="empty" data-testid={T.rsEmpty}>
                    {data.source === "none" ? "Connect Prometheus (AXIOM_PROMETHEUS_URL) or metrics-server to get recommendations." : "Nothing needs attention."}
                </div>
            ) : (
                <div className="max-h-[60vh] overflow-auto" tabIndex={0} role="region" aria-label="Rightsizing recommendations">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Workload</th>
                                <th>CPU (request → recommended)</th>
                                <th>Memory (request → recommended)</th>
                                <th>Verdict</th>
                                <th className="text-right">Monthly change</th>
                            </tr>
                        </thead>
                        <tbody>
                            {items.map((i) => {
                                const v = VERDICT[i.verdict] || VERDICT.ok;
                                return (
                                    <tr key={`${i.namespace}/${i.workload}/${i.container}`} data-testid={T.rsRow(i.namespace, i.workload, i.container)}>
                                        <td className="name">
                                            {i.workload}
                                            <small>{i.namespace} · {i.container}</small>
                                        </td>
                                        <td className="mono text-[12.5px]">{i.cpu.requestMilli}m → {i.cpu.recommendedMilli}m <span className="text-ink-3">(used {i.cpu.usageMilli}m)</span></td>
                                        <td className="mono text-[12.5px]">{i.memory.requestMiB}Mi → {i.memory.recommendedMiB}Mi <span className="text-ink-3">(used {i.memory.usageMiB}Mi)</span></td>
                                        <td>
                                            <span title={i.reason || undefined}>
                                                <span className="pill" data-testid={T.rsVerdict(i.namespace, i.workload, i.container)} style={{ "--c": v.tone }}>
                                                    <i aria-hidden="true" />
                                                    {v.label}
                                                </span>
                                            </span>
                                        </td>
                                        <td className={`mono num text-right ${i.monthlyDeltaUsd < 0 ? "text-tone-green" : i.monthlyDeltaUsd > 0 ? "text-tone-amber" : ""}`}>
                                            {i.monthlyDeltaUsd === 0 ? "—" : `${i.monthlyDeltaUsd < 0 ? "−" : "+"}${usd(Math.abs(i.monthlyDeltaUsd))}`}
                                        </td>
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                </div>
            )}
            <p className="border-t border-border px-4 py-2 text-xs text-ink-3">
                Recommendations add headroom over observed usage. They are suggestions: Axiom never changes requests on its own.
            </p>
        </div>
    );
}
