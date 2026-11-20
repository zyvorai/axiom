import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import { usePageTitle } from "@/lib/pageTitle";
import StatusBadge from "@/components/StatusBadge";
import Sparkline from "@/components/Sparkline";
import { Skeleton } from "@/components/ui/skeleton";
import { ArrowRight, Rocket } from "lucide-react";
import { OVERVIEW } from "@/constants/testIds";
import { useAlertsCount } from "@/lib/alerts";
import OverviewHealth from "@/components/OverviewHealth";
import { useTranslation } from "react-i18next";

function SetupBanner() {
    const { t } = useTranslation();
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
            {t("pages:overview.setupNotice")}
            <Link to="/getting-started">
                {t("pages:overview.finishSetup")} <ArrowRight size={14} />
            </Link>
        </div>
    );
}

export default function Overview() {
    const { t } = useTranslation();
    usePageTitle(t("nav:items.overview"));
    const [data, setData] = useState(null);
    const alertCount = useAlertsCount();

    useEffect(() => {
        let alive = true;
        const load = () =>
            api.get("/overview").then((o) => {
                if (alive) setData(o.data);
            });
        load();
        const timer = setInterval(load, 6000);
        return () => {
            alive = false;
            clearInterval(timer);
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
        { key: "nodes", label: t("pages:overview.metrics.nodes"), value: data.readyNodes, of: data.nodes },
        { key: "pods", label: t("pages:overview.metrics.pods"), value: data.runningPods, of: data.pods },
        { key: "workloads", label: t("pages:overview.metrics.workloads"), value: data.workloads },
        { key: "vms", label: t("pages:overview.metrics.vms"), value: data.readyVms, of: data.vms },
    ];

    const utilization = [
        { id: "cpu", label: t("common:fields.cpu"), value: Math.round(data.cpuPercent) },
        { id: "memory", label: t("common:fields.memory"), value: Math.round(data.memoryPercent) },
        { id: "storage", label: t("pages:overview.utilization.storage"), value: storagePct },
    ];

    return (
        <div data-testid={OVERVIEW.root} className="page">
            <SetupBanner />

            <section className="hero hero-media">
                <h1>{data.cluster}</h1>
                <p>{t("pages:overview.tagline")}</p>
                <div className="status">
                    <StatusBadge status={data.health} />
                    <span className="text-ink-3">·</span>
                    <span>{data.region}</span>
                </div>
                <div className="acts">
                    <Link to="/workloads" className="btn btn-fill btn-lg">
                        {t("pages:overview.exploreWorkloads")} <ArrowRight size={16} />
                    </Link>
                    <Link to="/nodes" className="btn btn-line btn-lg">
                        {t("pages:overview.viewNodes")}
                    </Link>
                </div>
            </section>

            <div className="section-h">
                <h2>{t("pages:overview.glance.title")}</h2>
                <p>{t("pages:overview.glance.description")}</p>
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
                {t("pages:overview.storageProvisioned", { used: data.storageUsedTb, capacity: data.storageCapacityTb })}
            </p>

            <div className="divider" />
            <div className="section-h">
                <h2>{t("pages:overview.utilization.title")}</h2>
                <p>{t("pages:overview.utilization.description")}</p>
            </div>
            <div className="grid g3">
                {utilization.map((u) => (
                    <div className="stat media" key={u.id}>
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
                <h2>{t("pages:overview.trend.title")}</h2>
                <p>{t("pages:overview.trend.description")}</p>
            </div>
            <div className="grid g2">
                <div className="card media">
                    <div className="text-[13px] text-ink-2 mb-2">{t("pages:overview.trend.clusterCpu")}</div>
                    <Sparkline data={cpuSeries} color="var(--blue)" height={100} />
                </div>
                <div className="card media">
                    <div className="text-[13px] text-ink-2 mb-2">{t("pages:overview.trend.clusterMemory")}</div>
                    <Sparkline data={memSeries} color="var(--green)" height={100} />
                </div>
            </div>

            <div className="divider" />
            <OverviewHealth />

            <div className="divider" />
            <div className="section-h">
                <h2>{t("pages:overview.informed.title")}</h2>
                <p>{t("pages:overview.informed.description")}</p>
            </div>
            <div className="cta-cards">
                <Link to="/alerts" className="cta-card">
                    <b>
                        {t("pages:overview.cards.alerts")}{alertCount > 0 ? ` · ${alertCount}` : ""} <ArrowRight size={18} />
                    </b>
                    <span>{t("pages:overview.cards.alertsText")}</span>
                </Link>
                <Link to="/events" className="cta-card">
                    <b>
                        {t("pages:overview.cards.events")} <ArrowRight size={18} />
                    </b>
                    <span>{t("pages:overview.cards.eventsText")}</span>
                </Link>
                <Link to="/logs" className="cta-card">
                    <b>
                        {t("pages:overview.cards.logs")} <ArrowRight size={18} />
                    </b>
                    <span>{t("pages:overview.cards.logsText")}</span>
                </Link>
            </div>
        </div>
    );
}
