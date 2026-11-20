import { useEffect, useState } from "react";
import { Rocket } from "lucide-react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { ROLLOUTS as T } from "@/constants/testIds";

const q = (v) => (v == null || v === "" ? "—" : v);
const P = "pages:rollouts";

/** Read-only Argo Rollouts and Flagger Canaries (progressive delivery). */
export default function RolloutsPanel() {
    const { t } = useTranslation();
    const [state, setState] = useState({ data: null, error: "" });
    useEffect(() => {
        let alive = true;
        const tick = () =>
            api
                .get("/rollouts")
                .then((r) => alive && setState({ data: r.data, error: "" }))
                .catch((e) => alive && setState({ data: null, error: e?.response?.data?.error || e?.message || "" }));
        tick();
        const timer = setInterval(tick, 15000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);
    const { data, error } = state;
    if (error) return <div className="notice danger" role="alert">{error}</div>;
    if (!data) return <Skeleton className="h-32 rounded-2xl bg-muted" />;
    if (!data.available) {
        return (
            <GlassCard className="p-6">
                <div className="empty" data-testid={T.notInstalled}>
                    <b>{t(`${P}.notInstalled`)}</b>
                    <div className="mt-1 text-[12.5px] text-ink-3">{data.note || t(`${P}.notInstalledHint`)}</div>
                </div>
            </GlassCard>
        );
    }
    const rollouts = data.rollouts || [];
    const canaries = data.canaries || [];
    return (
        <div className="space-y-4" data-testid={T.root}>
            <GlassCard className="overflow-hidden">
                <div className="card-h">
                    <h2 className="flex items-center gap-2 text-[15px]">
                        <Rocket size={16} className="text-tone-link" /> {t(`${P}.argoTitle`)}
                    </h2>
                    <span className="r text-[12.5px] text-ink-3">{rollouts.length}</span>
                </div>
                {rollouts.length === 0 ? (
                    <div className="empty">{t(`${P}.emptyArgo`)}</div>
                ) : (
                    <div className="max-h-[65vh] overflow-auto" tabIndex={0} role="region" aria-label={t(`${P}.region`)}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("common:fields.namespace")}</th>
                                    <th>{t("common:fields.name")}</th>
                                    <th>{t(`${P}.columns.strategy`)}</th>
                                    <th>{t(`${P}.columns.phase`)}</th>
                                    <th>{t(`${P}.columns.step`)}</th>
                                    <th>{t(`${P}.columns.weight`)}</th>
                                    <th title={t(`${P}.columns.replicas`)}>{t("common:fields.replicas")}</th>
                                    <th>{t(`${P}.columns.pauseReason`)}</th>
                                    <th>{t(`${P}.columns.analysis`)}</th>
                                    <th>{t(`${P}.columns.stable`)} / {t(`${P}.columns.canary`)}</th>
                                    <th>{t(`${P}.columns.images`)}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {rollouts.map((r) => (
                                    <tr key={`${r.namespace}/${r.name}`} data-testid={T.argoRow(r.namespace, r.name)}>
                                        <td>{r.namespace}</td>
                                        <td className="name">{r.name}</td>
                                        <td>{r.strategy}</td>
                                        <td data-testid={T.argoPhase(r.namespace, r.name)} title={r.message || undefined}>
                                            <StatusBadge status={r.phase} />
                                        </td>
                                        <td className="mono">{r.strategy === "canary" ? `${r.currentStep}/${r.totalSteps}` : "—"}</td>
                                        <td className="mono">{r.strategy === "canary" ? `${r.weight}%` : "—"}</td>
                                        <td className="mono" title={t(`${P}.columns.replicas`)}>
                                            {r.desired} / {r.updated} / {r.ready} / {r.available}
                                        </td>
                                        <td>{q(r.pauseReason)}</td>
                                        <td data-testid={T.argoAnalysis(r.namespace, r.name)} title={r.analysisRun || undefined}>
                                            {r.analysisResult ? <StatusBadge status={r.analysisResult === "Successful" ? "Healthy" : r.analysisResult} /> : "—"}
                                        </td>
                                        <td className="mono">{q(r.stableRevision)} / {q(r.canaryRevision)}</td>
                                        <td className="mono">{(r.images || []).join(", ") || "—"}</td>
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
                        <Rocket size={16} className="text-tone-link" /> {t(`${P}.flaggerTitle`)}
                    </h2>
                    <span className="r text-[12.5px] text-ink-3">{canaries.length}</span>
                </div>
                {canaries.length === 0 ? (
                    <div className="empty">{t(`${P}.emptyFlagger`)}</div>
                ) : (
                    <div className="max-h-[65vh] overflow-auto" tabIndex={0} role="region" aria-label={t(`${P}.flaggerRegion`)}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("common:fields.namespace")}</th>
                                    <th>{t("common:fields.name")}</th>
                                    <th>{t(`${P}.columns.target`)}</th>
                                    <th>{t(`${P}.columns.phase`)}</th>
                                    <th>{t(`${P}.columns.weight`)}</th>
                                    <th>{t(`${P}.columns.failedChecks`)}</th>
                                    <th>{t(`${P}.columns.lastTransition`)}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {canaries.map((c) => (
                                    <tr key={`${c.namespace}/${c.name}`} data-testid={T.canaryRow(c.namespace, c.name)}>
                                        <td>{c.namespace}</td>
                                        <td className="name">{c.name}</td>
                                        <td>{q(c.target)}</td>
                                        <td data-testid={T.canaryPhase(c.namespace, c.name)}>
                                            <StatusBadge status={c.phase === "Succeeded" ? "Healthy" : c.phase} />
                                        </td>
                                        <td className="mono">{c.canaryWeight}%</td>
                                        <td className="mono">{c.failedChecks}</td>
                                        <td className="mono">{q(c.lastTransitionTime)}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </GlassCard>
            <p className="text-[12px] text-ink-3">{t(`${P}.readOnlyNote`)}</p>
        </div>
    );
}
