import { useEffect, useState } from "react";
import { Layers3, Zap } from "lucide-react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { AUTOSCALE as T } from "@/constants/testIds";

const errText = (e) => e?.response?.data?.error || e?.message || "Could not load";

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
        const t = setInterval(tick, 15000);
        return () => {
            alive = false;
            clearInterval(t);
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

function NotInstalled({ testId, name, hint }) {
    return (
        <GlassCard className="p-6">
            <div className="empty" data-testid={testId}>
                <b>{name} is not installed in this cluster.</b>
                <div className="mt-1 text-[12.5px] text-ink-3">{hint}</div>
            </div>
        </GlassCard>
    );
}

const q = (v) => (v == null || v === "" ? "—" : v);

/** Read-only view of VerticalPodAutoscalers and what they recommend. */
export function VpaPanel() {
    const { data, error } = useList("/vpas");
    if (error) return <div className="notice danger" role="alert">{error}</div>;
    if (!data) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    if (!data.installed) {
        return (
            <NotInstalled
                testId={T.vpaNotInstalled}
                name="The Vertical Pod Autoscaler"
                hint="Install the VPA recommender to get per-container CPU and memory recommendations here. Axiom only reads them; it never changes requests."
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
                <div className="empty">No VerticalPodAutoscalers.</div>
            ) : (
                <div className="max-h-[65vh] overflow-auto" tabIndex={0} role="region" aria-label="Vertical pod autoscalers">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Target</th>
                                <th>Mode</th>
                                <th>Container</th>
                                <th>CPU (lower · target · upper)</th>
                                <th>Memory (lower · target · upper)</th>
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
    const { data, error } = useList("/scaledobjects");
    if (error) return <div className="notice danger" role="alert">{error}</div>;
    if (!data) return <Skeleton className="h-48 rounded-2xl bg-muted" />;
    if (!data.installed) {
        return (
            <NotInstalled
                testId={T.kedaNotInstalled}
                name="KEDA"
                hint="Install KEDA to scale workloads on queue depth, cron schedules, HTTP load and other event sources. Its ScaledObjects will be listed here."
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
                <div className="empty">No ScaledObjects.</div>
            ) : (
                <div className="max-h-[65vh] overflow-auto" tabIndex={0} role="region" aria-label="KEDA scaled objects">
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Name</th>
                                <th>Target</th>
                                <th>Replicas</th>
                                <th>Triggers</th>
                                <th>State</th>
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
                                            {(s.triggers || []).map((t, i) => (
                                                <span key={`${t.type}-${i}`} className="pill soft mono text-[11px]">
                                                    {t.type}
                                                    {t.name ? ` · ${t.name}` : ""}
                                                </span>
                                            ))}
                                        </div>
                                    </td>
                                    <td>
                                        <span className="pill" style={{ "--c": s.fallback ? "var(--orange)" : s.active ? "var(--green)" : s.ready ? "var(--blue)" : "var(--red)" }}>
                                            <i aria-hidden="true" />
                                            {s.fallback ? "Fallback" : s.active ? "Active" : s.ready ? "Idle" : "Not ready"}
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
