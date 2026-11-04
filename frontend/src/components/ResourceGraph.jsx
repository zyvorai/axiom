import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { GRAPH } from "@/constants/testIds";
import { resourceHref } from "@/lib/resourceHref";
import { useTranslation } from "react-i18next";

// Relationship graph (GET /graph/{ns}/{kind}/{name}): ingress -> service -> workload ->
// replicaset -> pods, plus the autoscaler and the config/secret/volume names it references.
// Names and short statuses only — the backend never sends Secret or ConfigMap contents.

// Lanes headed by a Kubernetes kind keep it in English (`label`); the others
// are translated (`labelKey`).
const LANES = [
    { id: "ingress", label: "Ingress", kinds: ["Ingress"] },
    { id: "service", label: "Service", kinds: ["Service"] },
    { id: "scale", labelKey: "pages:resourceGraph.lanes.scale", kinds: ["HorizontalPodAutoscaler"] },
    { id: "workload", labelKey: "pages:resourceGraph.lanes.workload", kinds: ["Deployment", "StatefulSet", "DaemonSet", "CronJob"] },
    { id: "revision", labelKey: "pages:resourceGraph.lanes.revision", kinds: ["ReplicaSet", "Job"] },
    { id: "pod", label: "Pods", kinds: ["Pod"] },
    { id: "config", labelKey: "pages:resourceGraph.lanes.config", kinds: ["ConfigMap", "Secret", "PersistentVolumeClaim"] },
];

/** Lane heading in the current language. */
function laneLabel(t, lane) {
    return lane.labelKey ? t(lane.labelKey) : lane.label;
}

const NODE_W = 172;
const NODE_H = 36;
const COL_GAP = 70;
const ROW_GAP = 10;
const PAD = 16;
const HEAD = 24;

const LIST_HREFS = { HorizontalPodAutoscaler: "/autoscaling", Ingress: "/ingresses", PersistentVolumeClaim: "/storage" };

/** Where a node links to, or null when there is no page for it. */
export function nodeHref(n) {
    if (n.kind === "ReplicaSet") return null; // no detail page for ReplicaSets
    return resourceHref(n.namespace, `${n.kind}/${n.name}`) || LIST_HREFS[n.kind] || null;
}

function tone(status) {
    const s = String(status || "");
    if (!s) return "stroke-border";
    if (/fail|error|crash|backoff|missing|degraded|unknown|terminat|evict|oom|imagepull/i.test(s)) return "stroke-tone-red";
    if (/pending|creating|init|suspend|0\/\d/i.test(s)) return "stroke-tone-amber";
    if (/running|healthy|bound|complete|active|ready|clusterip|nodeport|loadbalancer|\d+\/\d+/i.test(s)) return "stroke-tone-green";
    return "stroke-border";
}

function clip(text, max) {
    return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}

/** Column per non-empty lane; nodes stacked and vertically centred within the tallest lane. */
export function layoutGraph(graph) {
    const lanes = LANES.map((l) => ({ ...l, nodes: [] }));
    const fallback = lanes.find((l) => l.id === "workload");
    for (const n of graph.nodes) (lanes.find((l) => l.kinds.includes(n.kind)) || fallback).nodes.push(n);
    const used = lanes.filter((l) => l.nodes.length > 0);
    const tallest = Math.max(1, ...used.map((l) => l.nodes.length));
    const innerH = tallest * NODE_H + (tallest - 1) * ROW_GAP;
    const pos = {};
    used.forEach((lane, col) => {
        const laneH = lane.nodes.length * NODE_H + (lane.nodes.length - 1) * ROW_GAP;
        const top = PAD + HEAD + (innerH - laneH) / 2;
        lane.nodes.forEach((n, row) => {
            pos[n.id] = { x: PAD + col * (NODE_W + COL_GAP), y: top + row * (NODE_H + ROW_GAP), col };
        });
    });
    return {
        lanes: used,
        pos,
        width: PAD * 2 + used.length * NODE_W + Math.max(0, used.length - 1) * COL_GAP,
        height: PAD * 2 + HEAD + innerH,
    };
}

function edgePath(a, b) {
    const forward = a.col <= b.col;
    const x1 = forward ? a.x + NODE_W : a.x;
    const x2 = forward ? b.x : b.x + NODE_W;
    const y1 = a.y + NODE_H / 2;
    const y2 = b.y + NODE_H / 2;
    const dx = Math.max(30, Math.abs(x2 - x1) / 2);
    const sign = forward ? 1 : -1;
    return { d: `M${x1},${y1} C${x1 + sign * dx},${y1} ${x2 - sign * dx},${y2} ${x2},${y2}`, mx: (x1 + x2) / 2, my: (y1 + y2) / 2 };
}

function GraphNodeShape({ node, p, isRoot, href, onNavigate }) {
    const { t } = useTranslation();
    const base = `${node.kind} ${node.name}${node.status ? `, ${node.status}` : ""}`;
    const label = isRoot ? t("pages:resourceGraph.rootLabel", { label: base }) : base;
    const body = (
        <>
            <title>{label}</title>
            <rect
                width={NODE_W}
                height={NODE_H}
                rx={9}
                className={`${isRoot ? "fill-soft-info" : "fill-card"} ${tone(node.status)}`}
                strokeWidth={isRoot ? 2 : 1.25}
            />
            <text x={10} y={14} className="fill-ink-3" fontSize={9.5} style={{ textTransform: "uppercase", letterSpacing: "0.04em" }}>
                {node.kind === "HorizontalPodAutoscaler" ? "HPA" : node.kind === "PersistentVolumeClaim" ? "PVC" : node.kind}
                {node.status ? ` · ${clip(node.status, 22)}` : ""}
            </text>
            <text x={10} y={28} className="fill-foreground" fontSize={11.5} fontWeight={500}>
                {clip(node.name, 24)}
            </text>
        </>
    );
    const common = { transform: `translate(${p.x},${p.y})`, "data-testid": GRAPH.node(node.id), "data-kind": node.kind, className: "graph-node outline-none focus-visible:[&>rect]:stroke-[3]" };
    if (!href || isRoot) {
        return (
            <g {...common} tabIndex={0} role="img" aria-label={label} aria-current={isRoot ? "true" : undefined}>
                {body}
            </g>
        );
    }
    return (
        <a
            {...common}
            href={href}
            aria-label={label}
            onClick={(e) => {
                if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
                e.preventDefault();
                onNavigate(href);
            }}
        >
            {body}
        </a>
    );
}

export default function ResourceGraph({ namespace, kind, name }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const [graph, setGraph] = useState(null);
    const [error, setError] = useState(null);

    useEffect(() => {
        let alive = true;
        setGraph(null);
        setError(null);
        api
            .get(`/graph/${encodeURIComponent(namespace)}/${encodeURIComponent(kind)}/${encodeURIComponent(name)}`)
            .then((r) => alive && setGraph(r.data))
            .catch((e) => alive && setError(e?.response?.data?.error || e.message));
        return () => {
            alive = false;
        };
    }, [namespace, kind, name]);

    const layout = useMemo(() => (graph ? layoutGraph(graph) : null), [graph]);
    const byId = useMemo(() => Object.fromEntries((graph?.nodes || []).map((n) => [n.id, n])), [graph]);

    if (error) {
        return (
            <GlassCard className="p-6 text-center text-sm text-muted-foreground" data-testid={GRAPH.root}>
                {t("pages:resourceGraph.unavailable", { error })}
            </GlassCard>
        );
    }
    if (!graph) return <Skeleton className="h-64 w-full rounded-2xl" data-testid={GRAPH.root} />;

    return (
        <GlassCard className="p-4" data-testid={GRAPH.root}>
            <p className="mb-3 text-[12.5px] text-ink-3">
                {t("pages:resourceGraph.intro")}
            </p>
            <div className="overflow-x-auto scrollbar-thin" tabIndex={0} role="region" aria-label={t("pages:resourceGraph.region")}>
                <svg
                    width={layout.width}
                    height={layout.height}
                    viewBox={`0 0 ${layout.width} ${layout.height}`}
                    role="group"
                    aria-label={t("pages:resourceGraph.svgLabel", { kind, name })}
                    className="block"
                >
                    <defs>
                        <marker id="graph-arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
                            <path d="M0,0 L10,5 L0,10 z" className="fill-ink-3" />
                        </marker>
                    </defs>
                    {layout.lanes.map((lane) => (
                        <text key={lane.id} x={PAD + layout.pos[lane.nodes[0].id].col * (NODE_W + COL_GAP)} y={PAD + 8} className="fill-ink-3" fontSize={10.5} fontWeight={600}>
                            {laneLabel(t, lane)}
                        </text>
                    ))}
                    <g aria-hidden="true">
                        {graph.edges.map((e, i) => {
                            const a = layout.pos[e.from];
                            const b = layout.pos[e.to];
                            if (!a || !b) return null;
                            const { d, mx, my } = edgePath(a, b);
                            return (
                                <g key={i}>
                                    <path d={d} fill="none" className="stroke-ink-4" strokeWidth={1.25} markerEnd="url(#graph-arrow)" />
                                    {e.relation !== "owns" && (
                                        <text x={mx} y={my - 4} textAnchor="middle" fontSize={9.5} className="fill-ink-3">
                                            {e.relation}
                                        </text>
                                    )}
                                </g>
                            );
                        })}
                    </g>
                    {graph.nodes.map((n) => (
                        <GraphNodeShape
                            key={n.id}
                            node={n}
                            p={layout.pos[n.id]}
                            isRoot={n.id === graph.root}
                            href={nodeHref(n)}
                            onNavigate={navigate}
                        />
                    ))}
                </svg>
            </div>
            {graph.truncated && (
                <p className="mt-2 text-[12px] text-tone-amber">{t("pages:resourceGraph.truncated")}</p>
            )}
            <details className="mt-3 text-[12.5px]">
                <summary className="cursor-pointer text-ink-2">{t("pages:resourceGraph.showList")}</summary>
                <ul data-testid={GRAPH.list} className="mt-2 space-y-1">
                    {graph.edges.length === 0 && <li className="text-ink-3">{t("pages:resourceGraph.noRelated")}</li>}
                    {graph.edges.map((e, i) => (
                        <li key={i} className="mono">
                            {byId[e.from]?.kind} {byId[e.from]?.name} <span className="text-ink-3">{e.relation}</span> {byId[e.to]?.kind}{" "}
                            {byId[e.to]?.name}
                        </li>
                    ))}
                </ul>
            </details>
        </GlassCard>
    );
}
