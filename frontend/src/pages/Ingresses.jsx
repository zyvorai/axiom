import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import { Lock, LockOpen, Waypoints, Router, Radio } from "lucide-react";
import SearchBox from "@/components/SearchBox";
import NamespaceFilter from "@/components/NamespaceFilter";
import SavedViews from "@/components/SavedViews";
import { useUrlState } from "@/lib/urlState";
import { useNamespaceFilter } from "@/lib/namespace";
import { useTranslation } from "react-i18next";

function providerTone(provider) {
    switch (provider) {
        case "envoy":
            return "var(--green)";
        case "nginx":
            return "var(--amber)";
        default:
            return "var(--blue)";
    }
}

export default function Ingresses() {
    const { t } = useTranslation();
    const [data, setData] = useState(null);
    const [gateways, setGateways] = useState(null);
    const [q, setQ] = useUrlState("q", "");
    const [ns, setNs, knownNamespaces, globalNs] = useNamespaceFilter();

    useEffect(() => {
        let alive = true;
        const load = () => {
            api.get("/ingresses").then((r) => alive && setData(r.data));
            api.get("/gateways").then((r) => alive && setGateways(r.data || [])).catch(() => alive && setGateways([]));
        };
        load();
        const timer = setInterval(load, 15000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    const rows = useMemo(
        () => (data || []).filter((i) => (ns === "all" || i.namespace === ns) && (q === "" || i.name.toLowerCase().includes(q.toLowerCase()) || (i.hosts || []).some((h) => h.toLowerCase().includes(q.toLowerCase())))),
        [data, q, ns]
    );
    const namespaces = useMemo(
        () => [...new Set([...knownNamespaces, ...(data || []).map((i) => i.namespace)])].sort(),
        [knownNamespaces, data]
    );

    const gatewayRows = useMemo(() => {
        const list = gateways || [];
        if (ns === "all") return list;
        return list.filter((g) => !g.namespace || g.namespace === ns);
    }, [gateways, ns]);

    const hasEnvoy = useMemo(
        () => (gateways || []).some((g) => g.provider === "envoy" || String(g.controller || "").includes("envoyproxy.io")),
        [gateways]
    );
    const hasNginxIngressClass = useMemo(
        () => (data || []).some((i) => /nginx/i.test(i.class || "")),
        [data]
    );

    if (!data) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-96 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title={t("pages:ingresses.title")}
                description={t("pages:ingresses.description")}
                tools={
                    <>
                        <SearchBox placeholder={t("pages:ingresses.search")} value={q} onChange={setQ} />
                        <NamespaceFilter value={ns} onChange={setNs} namespaces={namespaces} />
                        <SavedViews
                            pageKey="ingresses"
                            filters={{ q, ns }}
                            onApply={(f) => {
                                setQ(f.q ?? "");
                                setNs(f.ns ?? globalNs);
                            }}
                        />
                    </>
                }
            />

            <GlassCard className="space-y-2 p-4">
                <div className="flex flex-wrap items-center gap-2 text-sm font-medium">
                    <Radio size={15} className="text-tone-link" />
                    {t("pages:ingresses.envoy.title")}
                    {hasEnvoy ? (
                        <span className="pill" style={{ "--c": "var(--green)" }}>
                            <i aria-hidden="true" />
                            {t("pages:ingresses.envoy.detected")}
                        </span>
                    ) : (
                        <span className="pill" style={{ "--c": "var(--ink-3)" }}>
                            <i aria-hidden="true" />
                            {t("pages:ingresses.envoy.notDetected")}
                        </span>
                    )}
                </div>
                <p className="text-[13px] text-ink-3">{t("pages:ingresses.envoy.lead")}</p>
                {hasNginxIngressClass && !hasEnvoy ? (
                    <p className="text-[13px] text-tone-amber" role="status">
                        {t("pages:ingresses.envoy.nginxRetiring")}
                    </p>
                ) : null}
            </GlassCard>

            <GlassCard className="overflow-hidden p-0">
                <div className="panel-h">{t("pages:ingresses.legacyTitle")}</div>
                <div className="max-h-[65vh] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:ingresses.title")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("common:fields.namespace")}</th>
                                <th>{t("pages:ingresses.columns.class")}</th>
                                <th>{t("pages:ingresses.columns.hosts")}</th>
                                <th>{t("pages:ingresses.columns.address")}</th>
                                <th>TLS</th>
                                <th>{t("common:fields.age")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((i) => (
                                <tr key={`${i.namespace}/${i.name}`}>
                                    <td className="name">
                                        <span className="flex gap-2 items-center">
                                            <Waypoints size={14} className="text-ink-3" /> {i.name}
                                        </span>
                                    </td>
                                    <td className="mono">{i.namespace}</td>
                                    <td>{i.class || "—"}</td>
                                    <td>
                                        <div className="flex flex-wrap gap-1">
                                            {(i.hosts || []).map((h) => (
                                                <Badge key={h} variant="outline" className="mono text-[11px] bg-muted/30">
                                                    {h}
                                                </Badge>
                                            ))}
                                        </div>
                                    </td>
                                    <td className="mono num">{i.address || "—"}</td>
                                    <td>
                                        {i.tls ? (
                                            <Lock size={14} className="text-tone-green" />
                                        ) : (
                                            <LockOpen size={14} className="text-ink-3" />
                                        )}
                                    </td>
                                    <td className="mono">{i.age}</td>
                                </tr>
                            ))}
                            {rows.length === 0 && (
                                <tr>
                                    <td colSpan={7}>
                                        <div className="empty">{t("pages:ingresses.empty")}</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>

            <div className="mt-6">
                <GlassCard className="overflow-hidden p-0">
                    <div className="panel-h flex items-center gap-2">
                        <Router size={14} /> {t("pages:ingresses.gatewayTitle")}
                    </div>
                    <div className="max-h-[50vh] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:ingresses.gatewayTitle")}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("common:fields.kind")}</th>
                                    <th>{t("common:fields.name")}</th>
                                    <th>{t("common:fields.namespace")}</th>
                                    <th>{t("pages:ingresses.columns.class")}</th>
                                    <th>{t("pages:ingresses.columns.provider")}</th>
                                    <th>{t("pages:ingresses.columns.hosts")}</th>
                                    <th>{t("pages:ingresses.columns.address")}</th>
                                    <th>{t("common:fields.status")}</th>
                                    <th>{t("common:fields.age")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {gatewayRows.map((g) => (
                                    <tr key={`${g.kind}/${g.namespace || "_"}/${g.name}`}>
                                        <td className="mono">{g.kind}</td>
                                        <td className="name mono">{g.name}</td>
                                        <td className="mono">{g.namespace || "—"}</td>
                                        <td className="mono text-[12px]">{g.class || g.controller || "—"}</td>
                                        <td>
                                            {g.provider ? (
                                                <span className="pill" style={{ "--c": providerTone(g.provider) }}>
                                                    <i aria-hidden="true" />
                                                    {g.provider}
                                                </span>
                                            ) : (
                                                "—"
                                            )}
                                        </td>
                                        <td>
                                            <div className="flex flex-wrap gap-1">
                                                {(g.hosts || []).map((h) => (
                                                    <Badge key={h} variant="outline" className="mono text-[11px] bg-muted/30">
                                                        {h}
                                                    </Badge>
                                                ))}
                                                {(g.hosts || []).length === 0 ? "—" : null}
                                            </div>
                                        </td>
                                        <td className="mono num">{(g.addresses || []).join(", ") || "—"}</td>
                                        <td>{g.status || "—"}</td>
                                        <td className="mono">{g.age}</td>
                                    </tr>
                                ))}
                                {gatewayRows.length === 0 && (
                                    <tr>
                                        <td colSpan={9}>
                                            <div className="empty">
                                                {gateways === null ? t("common:states.loading") : t("pages:ingresses.noGateways")}
                                            </div>
                                        </td>
                                    </tr>
                                )}
                            </tbody>
                        </table>
                    </div>
                </GlassCard>
            </div>
        </div>
    );
}
