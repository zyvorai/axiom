import { useEffect, useState } from "react";
import { Layers3, Zap } from "lucide-react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import i18n from "@/lib/i18n";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { AUTOSCALE as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || i18n.t("errors:autoscalingPanels.couldNotLoad");

function useList(path) {
    const [state, setState] = useState({ data: null, error: "" });
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
    }, [path]);
    return state;
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
    const { data, error } = useList("/vpas");
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
                <span className="r text-[12.5px] text-ink-3">{data.items.length}</span>
            </div>
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
    const { data, error } = useList("/scaledobjects");
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
                <span className="r text-[12.5px] text-ink-3">{data.items.length}</span>
            </div>
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
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            )}
        </GlassCard>
    );
}
