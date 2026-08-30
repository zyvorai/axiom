import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import MetricRing from "@/components/MetricRing";
import StatusBadge from "@/components/StatusBadge";
import Sparkline from "@/components/Sparkline";
import { Skeleton } from "@/components/ui/skeleton";
import { AlertTriangle, Activity, Boxes, Server, MonitorSmartphone, ShieldCheck, Rocket, ArrowRight } from "lucide-react";
import { formatDistanceToNowStrict } from "date-fns";
import { OVERVIEW } from "@/constants/testIds";

function SetupBanner() {
    const [needsSetup, setNeedsSetup] = useState(false);

    useEffect(() => {
        let alive = true;
        Promise.all([api.get("/auth/providers").catch(() => ({ data: {} })), api.get("/session").catch(() => ({ data: {} }))]).then(
            ([providers, session]) => {
                if (!alive) return;
                const ssoConfigured = !!(providers.data?.oidc?.enabled || providers.data?.saml?.enabled);
                const usingDemoDefaults = session.data?.mode === "demo";
                setNeedsSetup(!ssoConfigured || usingDemoDefaults);
            }
        );
        return () => {
            alive = false;
        };
    }, []);

    if (!needsSetup) return null;

    return (
        <Link
            to="/getting-started"
            className="flex items-center gap-3 rounded-2xl border border-i17-cosmic/25 bg-i17-cosmic/[0.06] px-4 py-3 transition-colors hover:bg-i17-cosmic/[0.1]"
        >
            <div className="grid h-8 w-8 shrink-0 place-items-center rounded-xl bg-i17-cosmic/15 text-i17-cosmic">
                <Rocket className="h-4 w-4" />
            </div>
            <div className="min-w-0 flex-1">
                <div className="text-sm font-medium">This instance still has setup steps to finish</div>
                <div className="text-xs text-muted-foreground">SSO, storage and networking integrations aren't fully wired up yet.</div>
            </div>
            <div className="flex shrink-0 items-center gap-1 text-xs font-medium text-i17-cosmic">
                View checklist <ArrowRight className="h-3 w-3" />
            </div>
        </Link>
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
            <SetupBanner />
            {/* Hero */}
            <GlassCard className="p-6">
                <div className="flex flex-wrap items-end justify-between gap-4">
                    <div>
                        <div className="text-[11px] uppercase tracking-[0.18em] text-i17-cosmic/90">
                            zyvor · Axiom · Kubernetes · {data.region}
                        </div>
                        <h1 className="mt-1 text-4xl font-bold tracking-[-0.03em] sm:text-5xl">{data.cluster}</h1>
                        <div className="mt-2 flex items-center gap-3 text-sm text-muted-foreground">
                            <StatusBadge status={data.health} />
                            <span>·</span>
                            <span>{data.workloads} workloads</span>
                            <span>·</span>
                            <span>{data.vms} VMs</span>
                        </div>
                    </div>
                    <div className="grid grid-cols-2 gap-6 sm:grid-cols-3">
                        <MetricRing value={Math.round(data.cpuPercent)} label="CPU" color="var(--i17-cosmic)" sublabel="of allocatable" />
                        <MetricRing value={Math.round(data.memoryPercent)} label="Memory" color="var(--i17-sage)" sublabel="of allocatable" />
                        <MetricRing value={storagePct} label="Storage" color="var(--i17-lavender)" sublabel="of provisioned" />
                    </div>
                </div>
            </GlassCard>

            {/* Counters */}
            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                {[
                    { key: "nodes", label: "Nodes", value: `${data.readyNodes}/${data.nodes}`, icon: Server, color: "var(--i17-cosmic)", series: cpuSeries },
                    { key: "pods", label: "Pods", value: `${data.runningPods}/${data.pods}`, icon: Boxes, color: "var(--i17-sage)", series: memSeries },
                    { key: "workloads", label: "Workloads", value: data.workloads, icon: Activity, color: "var(--i17-gold)", series: cpuSeries },
                    { key: "vms", label: "VMs", value: `${data.readyVms}/${data.vms}`, icon: MonitorSmartphone, color: "var(--i17-sky-blue)", series: memSeries },
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
                    <Sparkline data={cpuSeries} color="var(--i17-cosmic)" />
                </GlassCard>
                <GlassCard className="p-5">
                    <div className="mb-3 flex items-center justify-between">
                        <div>
                            <div className="text-sm font-medium">Cluster Memory</div>
                            <div className="text-xs text-muted-foreground">Recent utilisation</div>
                        </div>
                        <span className="mono text-xs text-muted-foreground">{Math.round(data.memoryPercent)}%</span>
                    </div>
                    <Sparkline data={memSeries} color="var(--i17-sage)" />
                </GlassCard>
            </div>

            {/* Health + Alerts + Activity */}
            <div className="grid gap-4 lg:grid-cols-3">
                <GlassCard className="p-5">
                    <div className="mb-3 flex items-center gap-2">
                        <ShieldCheck className="h-4 w-4 text-i17-sage" />
                        <div className="text-sm font-medium">Cluster health</div>
                    </div>
                    <div className="space-y-2 text-sm">
                        {[
                            ["Status", data.health],
                            ["Region", data.region],
                            ["Storage used", `${data.storageUsedTb} / ${data.storageCapacityTb} TB`],
                            ["Mode", "Kubernetes"],
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
                                    <span className="mono text-xs text-i17-sky">
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
