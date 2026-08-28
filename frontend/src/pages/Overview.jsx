import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { useCluster } from "@/lib/cluster";
import GlassCard from "@/components/GlassCard";
import MetricRing from "@/components/MetricRing";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import {
    Area,
    AreaChart,
    ResponsiveContainer,
    Tooltip,
    XAxis,
    YAxis,
} from "recharts";
import { AlertTriangle, Activity, Boxes, Server, MonitorSmartphone, ShieldCheck } from "lucide-react";
import { formatDistanceToNowStrict } from "date-fns";
import { OVERVIEW } from "@/constants/testIds";

function Sparkline({ data, color = "#0A84FF" }) {
    const rows = (data || []).map((v, i) => ({ i, v }));
    return (
        <ResponsiveContainer width="100%" height={80}>
            <AreaChart data={rows} margin={{ top: 5, right: 0, left: 0, bottom: 0 }}>
                <defs>
                    <linearGradient id={`sp-${color}`} x1="0" y1="0" x2="0" y2="1">
                        <stop offset="0%" stopColor={color} stopOpacity={0.55} />
                        <stop offset="100%" stopColor={color} stopOpacity={0} />
                    </linearGradient>
                </defs>
                <Area
                    type="monotone"
                    dataKey="v"
                    stroke={color}
                    strokeWidth={2}
                    fill={`url(#sp-${color})`}
                    isAnimationActive={false}
                />
                <XAxis dataKey="i" hide />
                <YAxis hide domain={[0, 100]} />
                <Tooltip
                    contentStyle={{
                        background: "rgba(20,20,22,0.85)",
                        border: "1px solid rgba(255,255,255,0.1)",
                        borderRadius: 12,
                        color: "#fff",
                    }}
                    formatter={(v) => `${v}%`}
                    labelFormatter={() => ""}
                />
            </AreaChart>
        </ResponsiveContainer>
    );
}

function relTime(iso) {
    try {
        return formatDistanceToNowStrict(new Date(iso), { addSuffix: true });
    } catch {
        return iso;
    }
}

export default function Overview() {
    const { active } = useCluster();
    const [data, setData] = useState(null);
    const [alerts, setAlerts] = useState([]);
    const [activity, setActivity] = useState([]);

    useEffect(() => {
        let alive = true;
        const load = () =>
            Promise.all([
                api.get("/overview"),
                api.get("/alerts").catch(() => ({ data: { warnings: [] } })),
                api.get("/events").catch(() => ({ data: [] })),
            ]).then(([o, a, e]) => {
                if (!alive) return;
                setData(o.data);
                setAlerts(a.data.warnings || []);
                setActivity((e.data || []).slice(0, 4));
            });
        load();
        const t = setInterval(load, 6000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    if (!data) {
        return (
            <div data-testid={OVERVIEW.root} className="grid gap-4 lg:grid-cols-3">
                {[...Array(6)].map((_, i) => (
                    <Skeleton key={i} className="h-40 rounded-2xl bg-white/[0.04]" />
                ))}
            </div>
        );
    }

    const cpuSeries = (data.utilization || []).map((u) => u.cpu);
    const memSeries = (data.utilization || []).map((u) => u.memory);
    const storagePct = data.storageCapacityBytes
        ? Math.round((data.storageUsedBytes / data.storageCapacityBytes) * 100)
        : 0;

    return (
        <div data-testid={OVERVIEW.root} className="space-y-4">
            {/* Hero */}
            <GlassCard className="p-6">
                <div className="flex flex-wrap items-end justify-between gap-4">
                    <div>
                        <div className="text-[11px] uppercase tracking-[0.18em] text-[#ff7550]/90">
                            zyvor · Axiom · {active?.provider} · {data.region}
                        </div>
                        <h1 className="mt-1 text-4xl font-semibold tracking-tight sm:text-5xl">{data.cluster}</h1>
                        <div className="mt-2 flex items-center gap-3 text-sm text-muted-foreground">
                            <StatusBadge status={data.health} />
                            <span>·</span>
                            <span>{data.workloads} workloads</span>
                            <span>·</span>
                            <span>{data.vms} VMs</span>
                        </div>
                    </div>
                    <div className="grid grid-cols-2 gap-6 sm:grid-cols-3">
                        <MetricRing value={Math.round(data.cpuPercent)} label="CPU" color="#ff7550" sublabel="of allocatable" />
                        <MetricRing value={Math.round(data.memoryPercent)} label="Memory" color="#93c88f" sublabel="of allocatable" />
                        <MetricRing value={storagePct} label="Storage" color="#b49bd8" sublabel="of provisioned" />
                    </div>
                </div>
            </GlassCard>

            {/* Counters */}
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                {[
                    { key: "nodes", label: "Nodes", value: `${data.readyNodes}/${data.nodes}`, icon: Server, color: "#ff7550", series: cpuSeries },
                    { key: "pods", label: "Pods", value: `${data.runningPods}/${data.pods}`, icon: Boxes, color: "#93c88f", series: memSeries },
                    { key: "workloads", label: "Workloads", value: data.workloads, icon: Activity, color: "#e0c996", series: cpuSeries },
                    { key: "vms", label: "VMs", value: `${data.readyVms}/${data.vms}`, icon: MonitorSmartphone, color: "#6ab6f0", series: memSeries },
                ].map((c) => (
                    <GlassCard key={c.key} data-testid={OVERVIEW.metric(c.key)} className="p-4">
                        <div className="flex items-center justify-between">
                            <div className="text-xs uppercase tracking-widest text-muted-foreground">{c.label}</div>
                            <c.icon className="h-4 w-4 text-muted-foreground" />
                        </div>
                        <div className="mt-2 text-3xl font-semibold tabular-nums">{c.value}</div>
                        <div className="mt-2">
                            <Sparkline data={c.series} color={c.color} />
                        </div>
                    </GlassCard>
                ))}
            </div>

            {/* CPU / Memory big charts */}
            <div className="grid gap-4 lg:grid-cols-2">
                <GlassCard className="p-5">
                    <div className="mb-3 flex items-center justify-between">
                        <div>
                            <div className="text-sm font-medium">Cluster CPU</div>
                            <div className="text-xs text-muted-foreground">Recent utilisation</div>
                        </div>
                        <span className="mono text-xs text-muted-foreground">{Math.round(data.cpuPercent)}%</span>
                    </div>
                    <Sparkline data={cpuSeries} color="#ff7550" />
                </GlassCard>
                <GlassCard className="p-5">
                    <div className="mb-3 flex items-center justify-between">
                        <div>
                            <div className="text-sm font-medium">Cluster Memory</div>
                            <div className="text-xs text-muted-foreground">Recent utilisation</div>
                        </div>
                        <span className="mono text-xs text-muted-foreground">{Math.round(data.memoryPercent)}%</span>
                    </div>
                    <Sparkline data={memSeries} color="#93c88f" />
                </GlassCard>
            </div>

            {/* Health + Alerts + Activity */}
            <div className="grid gap-4 lg:grid-cols-3">
                <GlassCard className="p-5">
                    <div className="mb-3 flex items-center gap-2">
                        <ShieldCheck className="h-4 w-4 text-[#93c88f]" />
                        <div className="text-sm font-medium">Cluster health</div>
                    </div>
                    <div className="space-y-2 text-sm">
                        {[
                            ["Status", data.health],
                            ["Region", data.region],
                            ["Storage used", `${data.storageUsedTb} / ${data.storageCapacityTb} TB`],
                            ["Mode", active?.provider || "—"],
                        ].map(([k, v]) => (
                            <div key={k} className="flex items-center justify-between">
                                <span className="text-muted-foreground">{k}</span>
                                <span className="mono text-xs">{v}</span>
                            </div>
                        ))}
                    </div>
                </GlassCard>

                <GlassCard className="p-5">
                    <div className="mb-3 text-sm font-medium">Alerts</div>
                    <div className="space-y-2">
                        {alerts.length === 0 && <p className="text-sm text-muted-foreground">No active warnings.</p>}
                        {alerts.map((a, i) => (
                            <div
                                key={i}
                                data-testid={OVERVIEW.alert(i)}
                                className="flex items-start gap-3 rounded-xl border border-white/5 bg-white/[0.02] p-3"
                            >
                                <AlertTriangle className="mt-0.5 h-4 w-4 text-amber-500" />
                                <div className="min-w-0 flex-1">
                                    <div className="text-sm">
                                        {a.reason}: {a.message}
                                    </div>
                                    <div className="mt-0.5 text-xs text-muted-foreground">{relTime(a.time)}</div>
                                </div>
                            </div>
                        ))}
                    </div>
                </GlassCard>

                <GlassCard className="p-5">
                    <div className="mb-3 text-sm font-medium">Activity</div>
                    <div className="space-y-3">
                        {activity.length === 0 && <p className="text-sm text-muted-foreground">No recent activity.</p>}
                        {activity.map((a, i) => (
                            <div key={i} data-testid={OVERVIEW.activity(i)} className="text-sm">
                                <div className="flex items-center gap-2">
                                    <span className={`status-dot ${a.type === "Warning" ? "warn" : "info"}`} />
                                    <span className="mono text-xs text-[#6ab6f0]">
                                        {a.namespace}/{a.object}
                                    </span>
                                    <span className="ml-auto text-[11px] text-muted-foreground">{relTime(a.time)}</span>
                                </div>
                                <div className="mt-1 text-muted-foreground">{a.message}</div>
                            </div>
                        ))}
                    </div>
                </GlassCard>
            </div>
        </div>
    );
}
