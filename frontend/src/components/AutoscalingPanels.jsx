import { useEffect, useState } from "react";
import { Boxes, Layers3, Pencil, Plus, Trash2, Zap } from "lucide-react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import i18n from "@/lib/i18n";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { AUTOSCALE as T, VPA_CREATE as V, KEDA_CREATE as K, AUTOSCALE_EDIT as AE } from "@/constants/testIds";
import { toast } from "sonner";
import StatusBadge from "@/components/StatusBadge";
import { useAuth } from "@/lib/auth";
import { useConfirm } from "@/components/ConfirmDialog";
import { VpaCreateDialog, KedaCreateDialog } from "@/components/AutoscalingCreate";
import { VpaEditDialog, KedaEditDialog } from "@/components/AutoscalingEdit";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("errors:autoscalingPanels.couldNotLoad");

function useList(path) {
    const [state, setState] = useState({ data: null, error: "" });
    const [tick2, setTick2] = useState(0);
    useEffect(() => {
        let alive = true;
        const tick = () =>
            api
                .get(path)
                .then((r) => alive && setState({ data: r.data, error: "" }))
                .catch((e) => alive && setState({ data: null, error: errText(e) }));
        tick();
        const timer = setInterval(tick, 15000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, [path, tick2]);
    return { ...state, reload: () => setTick2((n) => n + 1) };
}

function useRemove(path, reload) {
    const confirm = useConfirm();
    const { t } = useTranslation();
    return async (ns, name, what) => {
        const ok = await confirm({
            title: t("dialogs:autoscalingPanels.deleteTitle", { kind: what, name }),
            description: t("dialogs:autoscalingPanels.deleteDescription"),
            confirmLabel: t("common:actions.delete"),
        });
        if (!ok) return;
        try {
            await api.delete(`/${path}/${encodeURIComponent(ns)}/${encodeURIComponent(name)}`);
            toast.success(t("toasts:autoscalingPanels.deleted", { name }));
            reload();
        } catch (e) {
            toast.error(t("toasts:autoscalingPanels.deleteFailed"), { description: errText(e) });
        }
    };
}

function Conditions({ items }) {
    const bad = (items || []).filter((c) => c.status === "False" && c.type !== "Active");
    if (bad.length === 0) return null;
    return (
        <ul className="mt-1 space-y-0.5 text-[11.5px] text-tone-amber">
            {bad.map((c) => (
                <li key={c.type}>
                    {c.type}: {c.message || c.reason}
                </li>
            ))}
        </ul>
    );
}

function NotInstalled({ testId, title, hint }) {
    return (
        <GlassCard className="p-6">
            <div className="empty" data-testid={testId}>
                <b>{title}</b>
                <div className="mt-1 text-[12.5px] text-ink-3">{hint}</div>
            </div>
        </GlassCard>
    );
}

const q = (v) => (v == null || v === "" ? "—" : v);

/** Read-only view of VerticalPodAutoscalers and what they recommend. */
export function VpaPanel() {
    const { t } = useTranslation();
    const { data, error, reload } = useList("/vpas");
    const { isViewer } = useAuth();
    const [creating, setCreating] = useState(false);
    const [editing, setEditing] = useState(null);
    const remove = useRemove("vpas", reload);
    if (error) return <div className="notice danger" role="alert">{error}</div>;
    if (!data) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    if (!data.installed) {
        return (
            <NotInstalled
                testId={T.vpaNotInstalled}
                title={t("pages:autoscalingPanels.vpa.notInstalled")}
                hint={t("pages:autoscalingPanels.vpa.notInstalledHint")}
            />
        );
    }
    return (
        <GlassCard className="overflow-hidden" data-testid={T.vpaInstalled}>
            <div className="card-h">
                <h2 className="flex items-center gap-2 text-[15px]">
                    <Layers3 size={16} className="text-tone-link" /> VerticalPodAutoscalers
                </h2>
                <span className="r flex items-center gap-2 text-[12.5px] text-ink-3">
                    {data.items.length}
                    {!isViewer && (
                        <button type="button" className="btn btn-fill btn-sm" data-testid={V.open} onClick={() => setCreating(true)}>
                            <Plus size={13} /> {t("pages:autoscalingPanels.vpa.new")}
                        </button>
                    )}
                </span>
            </div>
            <VpaCreateDialog open={creating} onOpenChange={setCreating} onCreated={reload} />
            <VpaEditDialog target={editing} onClose={() => setEditing(null)} onSaved={reload} />
            {data.items.length === 0 ? (
                <div className="empty">{t("pages:autoscalingPanels.vpa.empty")}</div>
            ) : (
                <div className="max-h-[65vh] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:autoscalingPanels.vpa.region")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("common:fields.target")}</th>
                                <th>{t("pages:autoscalingPanels.vpa.columns.mode")}</th>
                                <th>{t("pages:autoscalingPanels.vpa.columns.container")}</th>
                                <th>{t("pages:autoscalingPanels.vpa.columns.cpu")}</th>
                                <th>{t("pages:autoscalingPanels.vpa.columns.memory")}</th>
                                {!isViewer && <th><span className="sr-only">{t("common:fields.actions")}</span></th>}
                            </tr>
                        </thead>
                        <tbody>
                            {data.items.flatMap((v) =>
                                (v.containers?.length ? v.containers : [null]).map((c, i) => (
                                    <tr key={`${v.namespace}/${v.name}/${c?.name || i}`} data-testid={i === 0 ? T.vpaRow(v.namespace, v.name) : undefined}>
                                        <td className="name">
                                            {i === 0 ? v.name : ""}
                                            {i === 0 && <small>{v.namespace}</small>}
                                            {i === 0 && <Conditions items={v.conditions} />}
                                        </td>
                                        <td>{i === 0 ? `${v.targetKind}/${v.targetName}` : ""}</td>
                                        <td>{i === 0 ? <span className="pill soft">{v.updateMode || "Auto"}</span> : ""}</td>
                                        <td className="mono">{c?.name || "—"}</td>
                                        <td className="mono text-[12.5px]" data-testid={i === 0 ? T.vpaRecommendation(v.namespace, v.name) : undefined}>
                                            {c ? `${q(c.lowerBound?.cpu)} · ${q(c.target?.cpu)} · ${q(c.upperBound?.cpu)}` : "—"}
                                        </td>
                                        <td className="mono text-[12.5px]">
                                            {c ? `${q(c.lowerBound?.memory)} · ${q(c.target?.memory)} · ${q(c.upperBound?.memory)}` : "—"}
                                        </td>
                                        {!isViewer && (
                                            <td>
                                                {i === 0 && (
                                                    <>
                                                    <button type="button" className="btn btn-line btn-sm mr-1.5" data-testid={AE.vpaOpen(v.namespace, v.name)} aria-label={t("pages:autoscalingPanels.vpa.editAria", { name: v.name })} onClick={() => setEditing({ namespace: v.namespace, name: v.name })}>
                                                        <Pencil size={13} />
                                                    </button>
                                                    <button type="button" className="btn btn-danger btn-sm" data-testid={V.remove(v.namespace, v.name)} aria-label={t("pages:autoscalingPanels.vpa.deleteAria", { name: v.name })} onClick={() => remove(v.namespace, v.name, "VPA")}>
                                                        <Trash2 size={13} />
                                                    </button>
                                                    </>
                                                )}
                                            </td>
                                        )}
                                    </tr>
                                ))
                            )}
                        </tbody>
                    </table>
                </div>
            )}
        </GlassCard>
    );
}

/** Read-only view of KEDA ScaledObjects. Trigger settings are never shown, only the trigger types. */
export function KedaPanel() {
    const { t } = useTranslation();
    const { data, error, reload } = useList("/scaledobjects");
    const { isViewer } = useAuth();
    const [creating, setCreating] = useState(false);
    const [editing, setEditing] = useState(null);
    const remove = useRemove("scaledobjects", reload);
    if (error) return <div className="notice danger" role="alert">{error}</div>;
    if (!data) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    if (!data.installed) {
        return (
            <NotInstalled
                testId={T.kedaNotInstalled}
                title={t("pages:autoscalingPanels.keda.notInstalled")}
                hint={t("pages:autoscalingPanels.keda.notInstalledHint")}
            />
        );
    }
    return (
        <GlassCard className="overflow-hidden">
            <div className="card-h">
                <h2 className="flex items-center gap-2 text-[15px]">
                    <Zap size={16} className="text-tone-link" /> ScaledObjects
                </h2>
                <span className="r flex items-center gap-2 text-[12.5px] text-ink-3">
                    {data.items.length}
                    {!isViewer && (
                        <button type="button" className="btn btn-fill btn-sm" data-testid={K.open} onClick={() => setCreating(true)}>
                            <Plus size={13} /> {t("pages:autoscalingPanels.keda.new")}
                        </button>
                    )}
                </span>
            </div>
            <KedaCreateDialog open={creating} onOpenChange={setCreating} onCreated={reload} />
            <KedaEditDialog target={editing} onClose={() => setEditing(null)} onSaved={reload} />
            {data.items.length === 0 ? (
                <div className="empty">{t("pages:autoscalingPanels.keda.empty")}</div>
            ) : (
                <div className="max-h-[65vh] overflow-auto" tabIndex={0} role="region" aria-label={t("pages:autoscalingPanels.keda.region")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("common:fields.target")}</th>
                                <th>{t("common:fields.replicas")}</th>
                                <th>{t("pages:autoscalingPanels.keda.columns.triggers")}</th>
                                <th>{t("pages:autoscalingPanels.keda.columns.state")}</th>
                                {!isViewer && <th><span className="sr-only">{t("common:fields.actions")}</span></th>}
                            </tr>
                        </thead>
                        <tbody>
                            {data.items.map((s) => (
                                <tr key={`${s.namespace}/${s.name}`} data-testid={T.kedaRow(s.namespace, s.name)}>
                                    <td className="name">
                                        {s.name}
                                        <small>{s.namespace}</small>
                                        <Conditions items={s.conditions} />
                                    </td>
                                    <td>{s.targetKind}/{s.targetName}</td>
                                    <td className="mono">
                                        {s.currentReplicas} <span className="text-ink-3">({s.minReplicas} – {s.maxReplicas})</span>
                                    </td>
                                    <td data-testid={T.kedaTriggers(s.namespace, s.name)}>
                                        <div className="flex flex-wrap gap-1">
                                            {(s.triggers || []).map((tr, i) => (
                                                <span key={`${tr.type}-${i}`} className="pill soft mono text-[11px]">
                                                    {tr.type}
                                                    {tr.name ? ` · ${tr.name}` : ""}
                                                </span>
                                            ))}
                                        </div>
                                    </td>
                                    <td>
                                        <span className="pill" style={{ "--c": s.fallback ? "var(--orange)" : s.active ? "var(--green)" : s.ready ? "var(--blue)" : "var(--red)" }}>
                                            <i aria-hidden="true" />
                                            {s.fallback ? t("pages:autoscalingPanels.keda.state.fallback") : s.active ? t("pages:autoscalingPanels.keda.state.active") : s.ready ? t("pages:autoscalingPanels.keda.state.idle") : t("pages:autoscalingPanels.keda.state.notReady")}
                                        </span>
                                    </td>
                                    {!isViewer && (
                                        <td>
                                            <button type="button" className="btn btn-line btn-sm mr-1.5" data-testid={AE.kedaOpen(s.namespace, s.name)} aria-label={t("pages:autoscalingPanels.keda.editAria", { name: s.name })} onClick={() => setEditing({ namespace: s.namespace, name: s.name })}>
                                                <Pencil size={13} />
                                            </button>
                                            <button type="button" className="btn btn-danger btn-sm" data-testid={K.remove(s.namespace, s.name)} aria-label={t("pages:autoscalingPanels.keda.deleteAria", { name: s.name })} onClick={() => remove(s.namespace, s.name, "ScaledObject")}>
                                                <Trash2 size={13} />
                                            </button>
                                        </td>
                                    )}
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </GlassCard>
    );
}

const readyStatus = (v) => (v === "True" ? "Ready" : v === "False" ? "NotReady" : "Pending");
const phaseStatus = (v) => (v === "Initialized" ? "Ready" : v === "Pending" || v === "Launched" || v === "Registered" ? "Pending" : v);
const KP = "pages:autoscalingPanels.karpenter";

/** Read-only view of Karpenter NodePools and NodeClaims. */
export function KarpenterPanel() {
    const { t } = useTranslation();
    const { data, error } = useList("/karpenter");
    if (error) return <div className="notice danger" role="alert">{error}</div>;
    if (!data) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    if (!data.available) {
        return (
            <NotInstalled
                testId={T.karpenterNotInstalled}
                title={t(`${KP}.notInstalled`)}
                hint={data.note || t(`${KP}.notInstalledHint`)}
            />
        );
    }
    const pools = data.nodePools || [];
    const claims = data.nodeClaims || [];
    return (
        <div className="space-y-4">
            <GlassCard className="overflow-hidden">
                <div className="card-h">
                    <h2 className="flex items-center gap-2 text-[15px]">
                        <Boxes size={16} className="text-tone-link" /> {t(`${KP}.pools`)}
                    </h2>
                    <span className="r text-[12.5px] text-ink-3">{pools.length}</span>
                </div>
                {pools.length === 0 ? (
                    <div className="empty">{t(`${KP}.emptyPools`)}</div>
                ) : (
                    <div className="max-h-[65vh] overflow-auto" tabIndex={0} role="region" aria-label={t(`${KP}.region`)}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("common:fields.name")}</th>
                                    <th>{t(`${KP}.columns.nodeClass`)}</th>
                                    <th>{t(`${KP}.columns.policy`)}</th>
                                    <th>{t(`${KP}.columns.consolidateAfter`)}</th>
                                    <th>{t(`${KP}.columns.cpuLimit`)}</th>
                                    <th>{t(`${KP}.columns.memoryLimit`)}</th>
                                    <th>{t(`${KP}.columns.nodes`)}</th>
                                    <th>{t("common:fields.cpu")}</th>
                                    <th>{t("common:fields.ready")}</th>
                                    <th>{t("common:fields.age")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {pools.map((p) => (
                                    <tr key={p.name} data-testid={T.karpenterPoolRow(p.name)}>
                                        <td className="name">{p.name}</td>
                                        <td>{q(p.nodeClass)}</td>
                                        <td>{q(p.policy)}</td>
                                        <td className="mono">{q(p.consolidateAfter)}</td>
                                        <td className="mono">{q(p.cpuLimit)}</td>
                                        <td className="mono">{q(p.memoryLimit)}</td>
                                        <td className="mono">{q(p.nodes)}</td>
                                        <td className="mono">{q(p.cpu)}</td>
                                        <td><StatusBadge status={readyStatus(p.ready)} /></td>
                                        <td>{q(p.age)}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </GlassCard>
            <GlassCard className="overflow-hidden">
                <div className="card-h">
                    <h2 className="flex items-center gap-2 text-[15px]">
                        <Layers3 size={16} className="text-tone-link" /> {t(`${KP}.claims`)}
                    </h2>
                    <span className="r text-[12.5px] text-ink-3">{claims.length}</span>
                </div>
                {claims.length === 0 ? (
                    <div className="empty">{t(`${KP}.emptyClaims`)}</div>
                ) : (
                    <div className="max-h-[65vh] overflow-auto" tabIndex={0} role="region" aria-label={t(`${KP}.claimsRegion`)}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("common:fields.name")}</th>
                                    <th>{t(`${KP}.columns.nodePool`)}</th>
                                    <th>{t("common:fields.node")}</th>
                                    <th>{t(`${KP}.columns.instanceType`)}</th>
                                    <th>{t(`${KP}.columns.capacity`)}</th>
                                    <th>{t(`${KP}.columns.zone`)}</th>
                                    <th>{t(`${KP}.columns.phase`)}</th>
                                    <th>{t("common:fields.age")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {claims.map((c) => (
                                    <tr key={c.name} data-testid={T.karpenterClaimRow(c.name)}>
                                        <td className="name">{c.name}</td>
                                        <td>{q(c.nodePool)}</td>
                                        <td>{q(c.node)}</td>
                                        <td className="mono">{q(c.instanceType)}</td>
                                        <td>{q(c.capacityType)}</td>
                                        <td>{q(c.zone)}</td>
                                        <td><StatusBadge status={phaseStatus(c.phase)} /></td>
                                        <td>{q(c.age)}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </GlassCard>
        </div>
    );
}
