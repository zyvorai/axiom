import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import MetricRing from "@/components/MetricRing";
import StatusBadge from "@/components/StatusBadge";
import Sparkline from "@/components/Sparkline";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { ArrowRight } from "lucide-react";
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
            className="mx-auto flex max-w-xl items-center justify-center gap-2 px-1 py-3 text-center text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
            Finish setup for this instance
            <ArrowRight className="h-3.5 w-3.5 text-i17-cosmic" />
        </Link>
    );
}

/** Centered apple.com section intro — one title, one sentence. */
function SectionIntro({ title, copy, className = "" }) {
    return (
        <div className={`mx-auto max-w-3xl text-center ${className}`}>
            <h2 className="text-display font-bold tracking-tight text-foreground text-balance">{title}</h2>
            {copy && <p className="mt-4 text-lg leading-relaxed text-muted-foreground text-pretty sm:text-xl">{copy}</p>}
        </div>
    );
}

export default function Overview() {
    const [data, setData] = useState(null);
    const [alertCount, setAlertCount] = useState(0);

    useEffect(() => {
        let alive = true;
        const load = () =>
            Promise.all([
                api.get("/overview"),
                api.get("/alerts").catch(() => ({ data: { warnings: [] } })),
            ]).then(([o, a]) => {
                if (!alive) return;
                setData(o.data);
                setAlertCount((a.data.warnings || []).length);
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
            <div data-testid={OVERVIEW.root} className="space-y-24 pb-24">
                <div className="flex flex-col items-center gap-5 pt-16 pb-8">
                    <Skeleton className="h-3 w-40 rounded bg-muted" />
                    <Skeleton className="h-20 w-72 max-w-full rounded-2xl bg-muted" />
                    <Skeleton className="h-5 w-64 rounded bg-muted" />
                    <Skeleton className="mt-4 h-11 w-40 rounded-full bg-muted" />
                </div>
                <div className="flex justify-center gap-16">
                    {[...Array(3)].map((_, i) => (
                        <Skeleton key={i} className="h-40 w-36 rounded-2xl bg-muted" />
                    ))}
                </div>
            </div>
        );
    }

    const cpuSeries = (data.utilization || []).map((u) => u.cpu);
    const memSeries = (data.utilization || []).map((u) => u.memory);
    const storagePct = data.storageCapacityBytes
        ? Math.round((data.storageUsedBytes / data.storageCapacityBytes) * 100)
        : 0;

    const inventory = [
        { key: "nodes", label: "Nodes", value: `${data.readyNodes}/${data.nodes}` },
        { key: "pods", label: "Pods", value: `${data.runningPods}/${data.pods}` },
        { key: "workloads", label: "Workloads", value: data.workloads },
        { key: "vms", label: "VMs", value: `${data.readyVms}/${data.vms}` },
    ];

    return (
        <div data-testid={OVERVIEW.root} className="pb-28">
            <SetupBanner />

            {/* ── Hero ── */}
            <section className="fade-up mx-auto max-w-4xl px-4 pb-24 pt-16 text-center sm:pt-20">
                <p className="text-[12px] font-semibold uppercase tracking-[0.28em] text-i17-cosmic">Axiom</p>
                <h1 className="mt-5 text-hero font-bold text-foreground text-balance">{data.cluster}</h1>
                <p className="mx-auto mt-6 max-w-xl text-lg text-muted-foreground text-pretty sm:text-xl">
                    Your Kubernetes-native private cloud — one console from k3s to production.
                </p>
                <div className="mt-5 flex flex-wrap items-center justify-center gap-3 text-sm text-muted-foreground">
                    <StatusBadge status={data.health} />
                    <span className="text-foreground/25">·</span>
                    <span>{data.region}</span>
                </div>
                <div className="mt-10 flex flex-wrap items-center justify-center gap-3">
                    <Button asChild variant="brand" className="rounded-full px-6">
                        <Link to="/workloads">
                            Explore workloads <ArrowRight className="ml-1.5 h-4 w-4" />
                        </Link>
                    </Button>
                    <Button asChild variant="outline" className="rounded-full px-6 bg-transparent">
                        <Link to="/nodes">View nodes</Link>
                    </Button>
                </div>
            </section>

            {/* ── Capacity ── */}
            <section className="fade-up border-t border-border/60 px-4 py-24" style={{ animationDelay: "60ms" }}>
                <SectionIntro title="Built for the load." copy="Live CPU, memory, and storage against what this cluster can allocate." />
                <div className="mt-16 flex flex-wrap items-start justify-center gap-x-16 gap-y-14 sm:gap-x-20">
                    <MetricRing
                        layout="stack"
                        size={128}
                        stroke={11}
                        value={Math.round(data.cpuPercent)}
                        label="CPU"
                        color="var(--i17-cosmic)"
                        sublabel="of allocatable"
                    />
                    <MetricRing
                        layout="stack"
                        size={128}
                        stroke={11}
                        value={Math.round(data.memoryPercent)}
                        label="Memory"
                        color="var(--i17-sage)"
                        sublabel="of allocatable"
                    />
                    <MetricRing
                        layout="stack"
                        size={128}
                        stroke={11}
                        value={storagePct}
                        label="Storage"
                        color="var(--i17-lavender)"
                        sublabel="of provisioned"
                    />
                </div>
            </section>

            {/* ── Inventory ── */}
            <section className="fade-up border-t border-border/60 px-4 py-24" style={{ animationDelay: "120ms" }}>
                <SectionIntro title="At a glance." copy="Everything running on this cluster, right now." />
                <div className="mx-auto mt-16 grid max-w-5xl gap-12 sm:grid-cols-2 lg:grid-cols-4 lg:gap-8">
                    {inventory.map((c) => (
                        <div key={c.key} data-testid={OVERVIEW.metric(c.key)} className="text-center">
                            <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">{c.label}</div>
                            <div className="mt-4 text-5xl font-bold tracking-[-0.04em] tabular-nums text-foreground sm:text-6xl">
                                {c.value}
                            </div>
                        </div>
                    ))}
                </div>
                <p className="mt-14 text-center text-sm text-muted-foreground">
                    {data.storageUsedTb} of {data.storageCapacityTb} TB storage provisioned
                </p>
            </section>

            {/* ── Utilization ── */}
            <section className="fade-up border-t border-border/60 px-4 py-24" style={{ animationDelay: "180ms" }}>
                <SectionIntro title="Utilization." copy="How hard the cluster has been working." />
                <div className="mx-auto mt-16 grid max-w-5xl gap-16 lg:grid-cols-2 lg:gap-20">
                    <div className="text-center lg:text-left">
                        <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Cluster CPU</div>
                        <div className="mt-3 text-5xl font-bold tracking-[-0.04em] tabular-nums text-foreground">
                            {Math.round(data.cpuPercent)}%
                        </div>
                        <div className="mt-8">
                            <Sparkline data={cpuSeries} color="var(--i17-cosmic)" height={140} />
                        </div>
                    </div>
                    <div className="text-center lg:text-left">
                        <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-muted-foreground">Cluster Memory</div>
                        <div className="mt-3 text-5xl font-bold tracking-[-0.04em] tabular-nums text-foreground">
                            {Math.round(data.memoryPercent)}%
                        </div>
                        <div className="mt-8">
                            <Sparkline data={memSeries} color="var(--i17-sage)" height={140} />
                        </div>
                    </div>
                </div>
            </section>

            {/* ── Observe: links only — logs live on Alerts / Events / Pod Logs ── */}
            <section className="fade-up border-t border-border/60 px-4 py-24" style={{ animationDelay: "240ms" }}>
                <SectionIntro
                    title="Stay informed."
                    copy="Warnings, cluster events, and pod streams live on their own pages — not on the home screen."
                />
                <div className="mx-auto mt-12 flex max-w-2xl flex-col items-center gap-4 sm:flex-row sm:justify-center">
                    <Button asChild variant="outline" className="rounded-full px-6 bg-transparent">
                        <Link to="/alerts">
                            Alerts{alertCount > 0 ? ` · ${alertCount}` : ""} <ArrowRight className="ml-1.5 h-4 w-4" />
                        </Link>
                    </Button>
                    <Button asChild variant="outline" className="rounded-full px-6 bg-transparent">
                        <Link to="/events">
                            Events <ArrowRight className="ml-1.5 h-4 w-4" />
                        </Link>
                    </Button>
                    <Button asChild variant="outline" className="rounded-full px-6 bg-transparent">
                        <Link to="/logs">
                            Pod logs <ArrowRight className="ml-1.5 h-4 w-4" />
                        </Link>
                    </Button>
                </div>
            </section>
        </div>
    );
}
