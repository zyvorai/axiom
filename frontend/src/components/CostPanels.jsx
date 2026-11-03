import { useCallback, useEffect, useMemo, useState } from "react";
import { Pencil, Plus, Trash2, Wand2 } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import i18n, { formatCurrency } from "@/lib/i18n";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import { useConfirm } from "@/components/ConfirmDialog";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { COST as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("errors:costPanels.generic");
const usd = (v) => formatCurrency(v);

const STATE_TONE = { ok: "var(--green)", warning: "var(--orange)", over: "var(--red)" };
const STATE_LABEL = { ok: "pages:costPanels.budgetState.ok", warning: "pages:costPanels.budgetState.warning", over: "pages:costPanels.budgetState.over" };

function BudgetDialog({ open, onOpenChange, initial, onSaved }) {
    const { t } = useTranslation();
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
            toast.success(t("toasts:costPanels.budgetSaved", { namespace: ns.trim() }));
            onSaved();
            onOpenChange(false);
        } catch (e) {
            toast.error(t("toasts:costPanels.budgetSaveFailed"), { description: errText(e) });
        } finally {
            setBusy(false);
        }
    };

    return (
        <Dialog open={open} onOpenChange={onOpenChange}>
            <DialogContent className="max-w-sm" data-testid={T.budgetDialog}>
                <DialogHeader>
                    <DialogTitle>{initial?.namespace ? t("dialogs:costPanels.budget.editTitle", { namespace: initial.namespace }) : t("dialogs:costPanels.budget.addTitle")}</DialogTitle>
                    <DialogDescription>{t("dialogs:costPanels.budget.description")}</DialogDescription>
                </DialogHeader>
                <div className="space-y-3">
                    <div className="space-y-1.5">
                        <Label htmlFor="cost-budget-ns" className="text-xs text-muted-foreground">{t("common:fields.namespace")}</Label>
                        <Input id="cost-budget-ns" data-testid={T.budgetNamespace} value={ns} onChange={(e) => setNs(e.target.value)} disabled={!!initial?.namespace} placeholder="payments" className="bg-muted/30" />
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                        <div className="space-y-1.5">
                            <Label htmlFor="cost-budget-amount" className="text-xs text-muted-foreground">{t("dialogs:costPanels.budget.amount")}</Label>
                            <Input id="cost-budget-amount" data-testid={T.budgetAmount} type="number" min={1} value={amount} onChange={(e) => setAmount(e.target.value)} className="bg-muted/30" />
                        </div>
                        <div className="space-y-1.5">
                            <Label htmlFor="cost-budget-warn" className="text-xs text-muted-foreground">{t("dialogs:costPanels.budget.warnAt")}</Label>
                            <Input id="cost-budget-warn" data-testid={T.budgetWarn} type="number" min={1} max={100} value={warn} onChange={(e) => setWarn(e.target.value)} className="bg-muted/30" />
                        </div>
                    </div>
                </div>
                <DialogFooter>
                    <button type="button" className="btn btn-fill" data-testid={T.budgetSave} onClick={save} disabled={busy || !ns.trim() || !(Number(amount) > 0)}>
                        {busy ? t("common:actions.saving") : t("dialogs:costPanels.budget.save")}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

/** Per-namespace monthly budgets against the estimated cost. */
export function CostBudgets() {
    const { t } = useTranslation();
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
        const ok = await confirm({ title: t("dialogs:costPanels.removeBudget.title", { namespace: b.namespace }), description: t("dialogs:costPanels.removeBudget.description"), confirmLabel: t("common:actions.remove") });
        if (!ok) return;
        try {
            await api.delete(`/cost/budgets/${encodeURIComponent(b.namespace)}`);
            toast.success(t("toasts:costPanels.budgetRemoved", { namespace: b.namespace }));
            load();
        } catch (e) {
            toast.error(t("toasts:costPanels.removeFailed"), { description: errText(e) });
        }
    };

    if (error) return <div className="notice danger" role="alert">{error}</div>;
    if (!data) return <Skeleton className="h-40 rounded-2xl bg-muted" />;

    return (
        <div className="card p-0" data-testid={T.budgets}>
            <div className="panel-h flex items-center justify-between">
                <span>{t("pages:costPanels.budgets")}</span>
                {!isViewer && (
                    <button type="button" className="btn btn-line btn-sm" data-testid={T.addBudget} onClick={() => setDialog({ open: true, initial: null })}>
                        <Plus size={13} /> {t("pages:costPanels.addBudget")}
                    </button>
                )}
            </div>
            {data.budgets.length === 0 ? (
                <div className="empty">{t("pages:costPanels.noBudgets")}</div>
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
                                            {t("pages:costPanels.budgetOf", { current: usd(b.currentUsd), budget: usd(b.monthlyUsd), percent: b.percent })}
                                        </span>
                                    </div>
                                    <div className="flex shrink-0 items-center gap-2">
                                        <span className="pill" data-testid={T.budgetState(b.namespace)} style={{ "--c": STATE_TONE[b.state] }}>
                                            <i aria-hidden="true" />
                                            {STATE_LABEL[b.state] ? t(STATE_LABEL[b.state]) : b.state}
                                        </span>
                                        {!isViewer && (
                                            <>
                                                <button type="button" className="btn btn-line btn-sm" data-testid={T.budgetEdit(b.namespace)} aria-label={t("pages:costPanels.editBudgetAria", { namespace: b.namespace })} onClick={() => setDialog({ open: true, initial: b })}>
                                                    <Pencil size={13} />
                                                </button>
                                                <button type="button" className="btn btn-danger btn-sm" data-testid={T.budgetRemove(b.namespace)} aria-label={t("pages:costPanels.removeBudgetAria", { namespace: b.namespace })} onClick={() => remove(b)}>
                                                    <Trash2 size={13} />
                                                </button>
                                            </>
                                        )}
                                    </div>
                                </div>
                                <div className="meter !mt-2" role="img" aria-label={t("pages:costPanels.budgetBarAria", { namespace: b.namespace, percent: b.percent })} data-testid={T.budgetBar(b.namespace)}>
                                    <i style={{ width: `${pct}%`, background: STATE_TONE[b.state] }} />
                                </div>
                            </li>
                        );
                    })}
                </ul>
            )}
            {data.budgets.length > 0 && (
                <p className="border-t border-border px-4 py-2 text-xs text-ink-3" data-testid={T.totals}>
                    {t("pages:costPanels.totals", { current: usd(data.totalCurrentUsd), budget: usd(data.totalBudgetUsd) })}
                </p>
            )}
            {(data.unbudgeted || []).length > 0 && !isViewer && (
                <div className="border-t border-border px-4 py-3">
                    <div className="mb-1.5 text-xs text-ink-3">{t("pages:costPanels.withoutBudget")}</div>
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


/** Review, then apply, one rightsizing recommendation. The server recomputes it; nothing here supplies target numbers. */
function ApplyRightsizingDialog({ item, windowSel, onClose, onApplied }) {
    const { t } = useTranslation();
    const [cpu, setCpu] = useState(true);
    const [mem, setMem] = useState(true);
    const [preview, setPreview] = useState(null);
    const [error, setError] = useState("");
    const [busy, setBusy] = useState("");

    const body = useMemo(
        () =>
            item && {
                namespace: item.namespace,
                kind: item.kind,
                workload: item.workload,
                container: item.container,
                resources: [cpu && "cpu", mem && "memory"].filter(Boolean),
                window: windowSel,
                observed: { cpuRequestMilli: item.cpu.requestMilli, memoryRequestMiB: item.memory.requestMiB },
            },
        [item, cpu, mem, windowSel]
    );

    // Always dry-run first: the person sees exactly what would change.
    useEffect(() => {
        if (!item) return undefined;
        if (body.resources.length === 0) {
            setPreview(null);
            setError("");
            return undefined;
        }
        let alive = true;
        setBusy("preview");
        setError("");
        api
            .post("/cost/rightsizing/apply", { ...body, dryRun: true })
            .then((r) => alive && setPreview(r.data))
            .catch((e) => {
                if (alive) {
                    setPreview(null);
                    setError(errText(e));
                }
            })
            .finally(() => alive && setBusy(""));
        return () => {
            alive = false;
        };
    }, [item, body]);

    useEffect(() => {
        if (item) {
            setCpu(true);
            setMem(true);
        }
    }, [item]);

    const apply = async () => {
        setBusy("apply");
        setError("");
        try {
            await api.post("/cost/rightsizing/apply", { ...body, dryRun: false });
            toast.success(t("toasts:costPanels.requestsUpdated", { target: `${item.workload}/${item.container}` }), { description: t("toasts:costPanels.requestsUpdatedDescription") });
            onApplied();
            onClose();
        } catch (e) {
            setError(errText(e));
        } finally {
            setBusy("");
        }
    };

    if (!item) return null;
    return (
        <Dialog open={!!item} onOpenChange={(o) => !o && onClose()}>
            <DialogContent className="max-w-lg" data-testid={T.applyDialog}>
                <DialogHeader>
                    <DialogTitle>{t("dialogs:costPanels.apply.title")}</DialogTitle>
                    <DialogDescription>
                        {t("dialogs:costPanels.apply.description", { workload: item.workload, container: item.container, namespace: item.namespace })}
                    </DialogDescription>
                </DialogHeader>
                <div className="flex items-center gap-5">
                    <label className="flex items-center gap-2 text-sm">
                        <input type="checkbox" data-testid={T.applyCpu} checked={cpu} onChange={(e) => setCpu(e.target.checked)} className="h-4 w-4 rounded border-border accent-primary" />
                        {t("dialogs:costPanels.apply.cpuRequest")}
                    </label>
                    <label className="flex items-center gap-2 text-sm">
                        <input type="checkbox" data-testid={T.applyMemory} checked={mem} onChange={(e) => setMem(e.target.checked)} className="h-4 w-4 rounded border-border accent-primary" />
                        {t("dialogs:costPanels.apply.memoryRequest")}
                    </label>
                </div>
                {error ? <div className="notice danger" role="alert" data-testid={T.applyError}>{error}</div> : null}
                {busy === "preview" && !preview ? <Skeleton className="h-20 rounded-xl bg-muted" /> : null}
                {preview && (
                    <div className="space-y-2" data-testid={T.applyChanges}>
                        <ul className="divide-y divide-border rounded-xl border border-border">
                            {preview.changes.map((c) => (
                                <li key={`${c.resource}/${c.field}`} className="px-3 py-2 text-sm">
                                    <span className="font-medium">{c.resource} {c.field}</span>{" "}
                                    <span className="mono">{c.from || t("dialogs:costPanels.apply.none")} → {c.to}</span>
                                    {c.note ? <div className="text-[11.5px] text-tone-amber">{c.note}</div> : null}
                                </li>
                            ))}
                        </ul>
                        <p className="text-xs text-ink-3">
                            {t("dialogs:costPanels.apply.basedOn", { source: preview.source, confidence: preview.confidence })} {preview.rollout}
                        </p>
                    </div>
                )}
                <DialogFooter>
                    <button type="button" className="btn btn-fill" data-testid={T.applyConfirm} onClick={apply} disabled={!!busy || !preview || body.resources.length === 0}>
                        <Wand2 size={14} /> {busy === "apply" ? t("dialogs:costPanels.apply.applying") : t("dialogs:costPanels.apply.confirm")}
                    </button>
                </DialogFooter>
            </DialogContent>
        </Dialog>
    );
}

const VERDICT = {
    over: { tone: "var(--orange)", labelKey: "pages:costPanels.verdict.over" },
    under: { tone: "var(--red)", labelKey: "pages:costPanels.verdict.under" },
    ok: { tone: "var(--green)", labelKey: "pages:costPanels.verdict.ok" },
};
const CONFIDENCE_NOTE = { high: "pages:costPanels.confidence.high", medium: "pages:costPanels.confidence.medium", low: "pages:costPanels.confidence.low" };

/** Request-vs-usage recommendations. A recommendation only; Axiom never changes requests itself. */
export function CostRightsizing() {
    const { t } = useTranslation();
    const { isViewer } = useAuth();
    const [applying, setApplying] = useState(null);
    const [reload, setReload] = useState(0);
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
    }, [windowSel, reload]);

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
                <span>{t("pages:costPanels.rightsizing")}</span>
                <div className="flex items-center gap-2">
                    <select aria-label={t("pages:costPanels.showFilter")} className="select" value={filter} onChange={(e) => setFilter(e.target.value)}>
                        <option value="actionable">{t("pages:costPanels.needsAttention")}</option>
                        <option value="all">{t("pages:costPanels.allContainers")}</option>
                    </select>
                    <select aria-label={t("pages:costPanels.usageWindow")} data-testid={T.window} className="select" value={windowSel} onChange={(e) => setWindowSel(e.target.value)}>
                        {["1d", "3d", "7d", "14d", "30d"].map((w) => (
                            <option key={w} value={w}>{t("pages:costPanels.lastWindow", { window: w })}</option>
                        ))}
                    </select>
                </div>
            </div>
            <div className="border-b border-border px-4 py-2.5 text-[12.5px] text-ink-3" data-testid={T.source}>
                <b className="text-ink-2">{data.source === "none" ? t("pages:costPanels.noUsageData") : t("pages:costPanels.source", { source: data.source })}</b>
                {data.confidence ? ` · ${CONFIDENCE_NOTE[data.confidence] ? t(CONFIDENCE_NOTE[data.confidence]) : data.confidence}` : ""}
                {data.note ? ` — ${data.note}` : ""}
            </div>
            <div className="stats c3 !m-3">
                <div className="stat" data-testid={T.summary("savings")}>
                    <b>{usd(Math.abs(s.monthlySavingsUsd || 0))}</b>
                    <span>{t("pages:costPanels.summary.savings")}</span>
                </div>
                <div className="stat" data-testid={T.summary("over")}>
                    <b>{s.overProvisioned || 0}</b>
                    <span>{t("pages:costPanels.summary.over")}</span>
                </div>
                <div className="stat" data-testid={T.summary("under")}>
                    <b>{s.underProvisioned || 0}</b>
                    <span>{t("pages:costPanels.summary.under")}</span>
                </div>
            </div>
            {items.length === 0 ? (
                <div className="empty" data-testid={T.rsEmpty}>
                    {data.source === "none" ? t("pages:costPanels.connectPrometheus") : t("pages:costPanels.nothingNeedsAttention")}
                </div>
            ) : (
                <div className="max-h-[60vh] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:costPanels.recommendationsRegion")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("pages:costPanels.columns.workload")}</th>
                                <th>{t("pages:costPanels.columns.cpu")}</th>
                                <th>{t("pages:costPanels.columns.memory")}</th>
                                <th>{t("pages:costPanels.columns.verdict")}</th>
                                <th className="text-right">{t("pages:costPanels.columns.monthlyChange")}</th>
                                {!isViewer && <th><span className="sr-only">{t("common:actions.apply")}</span></th>}
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
                                        <td className="mono text-[12.5px]">{i.cpu.requestMilli}m → {i.cpu.recommendedMilli}m <span className="text-ink-3">{t("pages:costPanels.used", { value: `${i.cpu.usageMilli}m` })}</span></td>
                                        <td className="mono text-[12.5px]">{i.memory.requestMiB}Mi → {i.memory.recommendedMiB}Mi <span className="text-ink-3">{t("pages:costPanels.used", { value: `${i.memory.usageMiB}Mi` })}</span></td>
                                        <td>
                                            <span title={i.reason || undefined}>
                                                <span className="pill" data-testid={T.rsVerdict(i.namespace, i.workload, i.container)} style={{ "--c": v.tone }}>
                                                    <i aria-hidden="true" />
                                                    {t(v.labelKey)}
                                                </span>
                                            </span>
                                        </td>
                                        <td className={`mono num text-right ${i.monthlyDeltaUsd < 0 ? "text-tone-green" : i.monthlyDeltaUsd > 0 ? "text-tone-amber" : ""}`}>
                                            {i.monthlyDeltaUsd === 0 ? "—" : `${i.monthlyDeltaUsd < 0 ? "−" : "+"}${usd(Math.abs(i.monthlyDeltaUsd))}`}
                                        </td>
                                        {!isViewer && (
                                            <td>
                                                {i.verdict !== "ok" && (
                                                    <button type="button" className="btn btn-line btn-sm" data-testid={T.rsApply(i.namespace, i.workload, i.container)} onClick={() => setApplying(i)}>
                                                        {t("pages:costPanels.applyEllipsis")}
                                                    </button>
                                                )}
                                            </td>
                                        )}
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                </div>
            )}
            <p className="border-t border-border px-4 py-2 text-xs text-ink-3">
                {t("pages:costPanels.footnote")}
            </p>
            <ApplyRightsizingDialog item={applying} windowSel={windowSel} onClose={() => setApplying(null)} onApplied={() => setReload((n) => n + 1)} />
        </div>
    );
}
