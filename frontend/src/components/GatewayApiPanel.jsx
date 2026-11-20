import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import { Skeleton } from "@/components/ui/skeleton";
import { DoorOpen } from "lucide-react";
import { NET } from "@/constants/testIds";
import { useTranslation } from "react-i18next";

// Condition values (True/False/Unknown) are Kubernetes API data and shown as sent.
function Cond({ value }) {
    if (!value) return <span className="text-ink-3">—</span>;
    const tone = value === "True" ? "ok" : value === "False" ? "err" : "warn";
    return (
        <span className="flex items-center gap-1.5">
            <span className={`status-dot ${tone}`} aria-hidden="true" />
            {value}
        </span>
    );
}

const gwAnchor = (ns, name) => `gw-${ns}-${name}`;

export default function GatewayApiPanel() {
    const { t } = useTranslation();
    const [inv, setInv] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () =>
            api
                .get("/gateway-api")
                .then((r) => alive && setInv(r.data))
                .catch(() => alive && setInv({ available: false, gateways: [], routes: [], referenceGrants: [] }));
        load();
        const timer = setInterval(load, 15000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    if (!inv) return <Skeleton className="h-40 rounded-2xl bg-muted" />;

    const gateways = inv.gateways || [];
    const routes = inv.routes || [];
    const grants = inv.referenceGrants || [];
    const known = new Set(gateways.map((g) => `${g.namespace}/${g.name}`));
    const goto = (ns, name) => {
        const el = document.getElementById(gwAnchor(ns, name));
        if (el) {
            el.scrollIntoView({ behavior: "smooth", block: "center" });
            el.focus();
        }
    };
    const routeCount = (g) =>
        routes.filter((r) => r.parentRefs.some((p) => p.namespace === g.namespace && p.name === g.name)).length;

    return (
        <GlassCard className="overflow-hidden" data-testid={NET.gwPanel}>
            <div className="card-h">
                <h2 className="flex items-center gap-2 text-[15px]">
                    <DoorOpen size={16} className="text-tone-link" /> {t("pages:networking.gatewayApi.title")}
                </h2>
            </div>
            <p className="text-[13px] text-ink-2 mb-3.5">{t("pages:networking.gatewayApi.description")}</p>
            {!inv.available ? (
                <div className="empty">{t("pages:networking.gatewayApi.unavailable")}</div>
            ) : (
                <>
                    <div className="overflow-x-auto scrollbar-thin" tabIndex={0} role="region" aria-label={t("pages:networking.gatewayApi.gatewaysRegion")}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>Gateway</th>
                                    <th>{t("pages:networking.gatewayApi.columns.class")}</th>
                                    <th>{t("pages:networking.gatewayApi.columns.listeners")}</th>
                                    <th>{t("pages:networking.gatewayApi.columns.addresses")}</th>
                                    <th>{t("pages:networking.gatewayApi.columns.accepted")}</th>
                                    <th>{t("pages:networking.gatewayApi.columns.programmed")}</th>
                                    <th>{t("pages:networking.gatewayApi.columns.routes")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {gateways.length === 0 && (
                                    <tr>
                                        <td colSpan={7} className="empty">{t("pages:networking.gatewayApi.noGateways")}</td>
                                    </tr>
                                )}
                                {gateways.map((g) => (
                                    <tr
                                        key={`${g.namespace}/${g.name}`}
                                        id={gwAnchor(g.namespace, g.name)}
                                        tabIndex={-1}
                                        data-testid={NET.gwGatewayRow(g.namespace, g.name)}
                                    >
                                        <td className="name">
                                            {g.name}
                                            <small>{g.namespace}</small>
                                        </td>
                                        <td className="mono">{g.class || "—"}</td>
                                        <td className="mono">
                                            {(g.listeners || []).map((l) => (
                                                <div key={l.name} className="flex items-center gap-1.5">
                                                    <span className={`status-dot ${l.programmed === "True" ? "ok" : l.programmed === "False" ? "err" : "warn"}`} aria-hidden="true" />
                                                    {l.name} {l.protocol}/{l.port}
                                                    {l.hostname ? ` ${l.hostname}` : ""}
                                                    <small> ({l.attachedRoutes})</small>
                                                </div>
                                            ))}
                                        </td>
                                        <td className="mono">{(g.addresses || []).join(", ") || "—"}</td>
                                        <td><Cond value={g.accepted} /></td>
                                        <td><Cond value={g.programmed} /></td>
                                        <td className="num">{routeCount(g)}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>

                    <div className="card-h mt-3.5">
                        <h3 className="text-[14px]">{t("pages:networking.gatewayApi.routesTitle")}</h3>
                    </div>
                    <div className="overflow-x-auto scrollbar-thin" tabIndex={0} role="region" aria-label={t("pages:networking.gatewayApi.routesRegion")}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>{t("pages:networking.gatewayApi.columns.kind")}</th>
                                    <th>{t("pages:networking.gatewayApi.columns.hostnames")}</th>
                                    <th>{t("pages:networking.gatewayApi.columns.parents")}</th>
                                    <th>{t("pages:networking.gatewayApi.columns.backends")}</th>
                                    <th>{t("pages:networking.gatewayApi.columns.accepted")}</th>
                                    <th>{t("pages:networking.gatewayApi.columns.resolvedRefs")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {routes.length === 0 && (
                                    <tr>
                                        <td colSpan={6} className="empty">{t("pages:networking.gatewayApi.noRoutes")}</td>
                                    </tr>
                                )}
                                {routes.map((r) => (
                                    <tr key={`${r.kind}/${r.namespace}/${r.name}`} data-testid={NET.gwRouteRow(r.kind, r.namespace, r.name)}>
                                        <td className="name">
                                            {r.name}
                                            <small>{r.kind} · {r.namespace}</small>
                                        </td>
                                        <td className="mono wrap">{(r.hostnames || []).join(", ") || "—"}</td>
                                        <td>
                                            {(r.parentRefs || []).map((p, i) => {
                                                const label = `${p.namespace}/${p.name}${p.sectionName ? `:${p.sectionName}` : ""}`;
                                                return known.has(`${p.namespace}/${p.name}`) ? (
                                                    <div key={i}>
                                                        <button
                                                            type="button"
                                                            className="mono underline"
                                                            aria-label={t("pages:networking.gatewayApi.gotoGateway", { name: p.name })}
                                                            data-testid={NET.gwParentLink(p.namespace, p.name)}
                                                            onClick={() => goto(p.namespace, p.name)}
                                                        >
                                                            {label}
                                                        </button>
                                                    </div>
                                                ) : (
                                                    <div key={i} className="mono">{label}</div>
                                                );
                                            })}
                                        </td>
                                        <td className="mono wrap">
                                            {(r.backendRefs || []).map((b, i) => (
                                                <div key={i}>
                                                    {b.kind}/{b.namespace}/{b.name}
                                                    {b.port ? `:${b.port}` : ""}
                                                </div>
                                            ))}
                                        </td>
                                        <td><Cond value={r.accepted} /></td>
                                        <td><Cond value={r.resolvedRefs} /></td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>

                    <div className="card-h mt-3.5">
                        <h3 className="text-[14px]">ReferenceGrants</h3>
                    </div>
                    <div className="overflow-x-auto scrollbar-thin" tabIndex={0} role="region" aria-label={t("pages:networking.gatewayApi.grantsRegion")}>
                        <table className="tbl">
                            <thead>
                                <tr>
                                    <th>ReferenceGrant</th>
                                    <th>{t("pages:networking.gatewayApi.columns.from")}</th>
                                    <th>{t("pages:networking.gatewayApi.columns.to")}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {grants.length === 0 && (
                                    <tr>
                                        <td colSpan={3} className="empty">{t("pages:networking.gatewayApi.noGrants")}</td>
                                    </tr>
                                )}
                                {grants.map((g) => (
                                    <tr key={`${g.namespace}/${g.name}`} data-testid={NET.gwGrantRow(g.namespace, g.name)}>
                                        <td className="name">
                                            {g.name}
                                            <small>{g.namespace}</small>
                                        </td>
                                        <td className="mono wrap">{(g.from || []).join(", ")}</td>
                                        <td className="mono wrap">{(g.to || []).join(", ")}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </>
            )}
        </GlassCard>
    );
}
