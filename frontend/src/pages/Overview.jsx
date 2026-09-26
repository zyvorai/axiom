import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import StatusBadge from "@/components/StatusBadge";
import Sparkline from "@/components/Sparkline";
import { Skeleton } from "@/components/ui/skeleton";
import { ArrowRight, Rocket } from "lucide-react";
import { OVERVIEW } from "@/constants/testIds";
import { useAlertsCount } from "@/lib/alerts";

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
        <div className="notice">
            <Rocket size={16} />
            Setup for this instance isn't finished yet.
            <Link to="/getting-started">
                Finish setup <ArrowRight size={14} />
            </Link>
        </div>
    );
}

export default function Overview() {
    const [data, setData] = useState(null);
    const alertCount = useAlertsCount();

    useEffect(() => {
        let alive = true;
        const load = () =>
            api.get("/overview").then((o) => {
                if (alive) setData(o.data);
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
            <div data-testid={OVERVIEW.root} className="page rhythm">
                <div className="hero">
                    <Skeleton className="mx-auto h-16 w-72 rounded-2xl bg-muted" />
                    <Skeleton className="mx-auto mt-4 h-5 w-64 rounded bg-muted" />
                </div>
                <div className="stats">
                    {[...Array(4)].map((_, i) => (
                        <Skeleton key={i} className="h-32 rounded-2xl bg-muted" />
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
        { key: "nodes", label: "Nodes ready", value: data.readyNodes, of: data.nodes },
        { key: "pods", label: "Pods running", value: data.runningPods, of: data.pods },
        { key: "workloads", label: "Workloads", value: data.workloads },
        { key: "vms", label: "VMs running", value: data.readyVms, of: data.vms },
    ];

    const utilization = [
        { label: "CPU", value: Math.round(data.cpuPercent) },
        { label: "Memory", value: Math.round(data.memoryPercent) },
        { label: "Storage", value: storagePct },
    ];

    return (
        <div data-testid={OVERVIEW.root} className="page">
            <SetupBanner />

            <section className="hero hero-media">
                <h1>{data.cluster}</h1>
                <p>Your Kubernetes-native private cloud. One console from k3s to production.</p>
                <div className="status">
                    <StatusBadge status={data.health} />
                    <span className="text-ink-3">·</span>
                    <span>{data.region}</span>
                </div>
                <div className="acts">
                    <Link to="/workloads" className="btn btn-fill btn-lg">
                        Explore workloads <ArrowRight size={16} />
                    </Link>
                    <Link to="/nodes" className="btn btn-line btn-lg">
                        View nodes
                    </Link>
                </div>
            </section>

            <div className="section-h">
                <h2>At a glance</h2>
                <p>Everything running on this cluster, right now.</p>
            </div>
            <div className="stats">
                {inventory.map((c) => (
                    <div className="stat media" key={c.key} data-testid={OVERVIEW.metric(c.key)}>
                        <b>
                            {c.value}
                            {c.of != null && <small>/{c.of}</small>}
                        </b>
                        <span>{c.label}</span>
                    </div>
                ))}
            </div>
            <p className="text-center text-[13px] text-ink-3 mt-3.5">
                {data.storageUsedTb} of {data.storageCapacityTb} TB storage provisioned
            </p>

            <div className="divider" />
            <div className="section-h">
                <h2>Utilization</h2>
                <p>How hard the cluster has been working against what it can allocate.</p>
            </div>
            <div className="grid g3">
                {utilization.map((u) => (
                    <div className="stat media" key={u.label}>
                        <b>
                            {u.value}
                            <small>%</small>
                        </b>
                        <span>{u.label}</span>
                        <div className="meter">
                            <i style={{ width: `${u.value}%`, background: u.value > 80 ? "var(--orange)" : "var(--blue)" }} />
                        </div>
                    </div>
                ))}
            </div>

            <div className="divider" />
            <div className="section-h">
                <h2>Trend</h2>
                <p>CPU and memory over the last observed window.</p>
            </div>
            <div className="grid g2">
                <div className="card media">
                    <div className="text-[13px] text-ink-2 mb-2">Cluster CPU</div>
                    <Sparkline data={cpuSeries} color="var(--blue)" height={100} />
                </div>
                <div className="card media">
                    <div className="text-[13px] text-ink-2 mb-2">Cluster Memory</div>
                    <Sparkline data={memSeries} color="var(--green)" height={100} />
                </div>
            </div>

            <div className="divider" />
            <div className="section-h">
                <h2>Stay informed</h2>
                <p>Warnings, cluster events, and pod streams live on their own pages.</p>
            </div>
            <div className="cta-cards">
                <Link to="/alerts" className="cta-card">
                    <b>
                        Alerts{alertCount > 0 ? ` · ${alertCount}` : ""} <ArrowRight size={18} />
                    </b>
                    <span>Warnings and rules that need your attention.</span>
                </Link>
                <Link to="/events" className="cta-card">
                    <b>
                        Events <ArrowRight size={18} />
                    </b>
                    <span>What the cluster has been doing, as it happens.</span>
                </Link>
                <Link to="/logs" className="cta-card">
                    <b>
                        Pod logs <ArrowRight size={18} />
                    </b>
                    <span>Stream the output of any pod in real time.</span>
                </Link>
            </div>
        </div>
    );
}
