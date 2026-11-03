import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
import SearchBox from "@/components/SearchBox";
import NamespaceFilter from "@/components/NamespaceFilter";
import SavedViews from "@/components/SavedViews";
import { useUrlState } from "@/lib/urlState";
import { useNamespaceFilter } from "@/lib/namespace";
import { useTranslation } from "react-i18next";

export default function Services() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const [data, setData] = useState(null);
    const [q, setQ] = useUrlState("q", "");
    const [ns, setNs, knownNamespaces, globalNs] = useNamespaceFilter();

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/services").then((r) => alive && setData(r.data));
        load();
        const timer = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    const rows = useMemo(
        () => (data || []).filter((s) => (ns === "all" || s.namespace === ns) && (q === "" || s.name.toLowerCase().includes(q.toLowerCase()))),
        [data, q, ns]
    );
    const namespaces = useMemo(
        () => [...new Set([...knownNamespaces, ...(data || []).map((s) => s.namespace)])].sort(),
        [knownNamespaces, data]
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
                title={t("pages:services.title")}
                description={t("pages:services.description")}
                tools={
                    <>
                        <SearchBox placeholder={t("pages:services.search")} value={q} onChange={setQ} />
                        <NamespaceFilter value={ns} onChange={setNs} namespaces={namespaces} />
                        <SavedViews
                            pageKey="services"
                            filters={{ q, ns }}
                            onApply={(f) => {
                                setQ(f.q ?? "");
                                setNs(f.ns ?? globalNs);
                            }}
                        />
                    </>
                }
            />

            <GlassCard className="overflow-hidden p-0">
                <div className="max-h-[65vh] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:services.title")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("common:fields.namespace")}</th>
                                <th>{t("common:fields.type")}</th>
                                <th>{t("pages:services.columns.clusterIp")}</th>
                                <th>{t("pages:services.columns.ports")}</th>
                                <th>{t("pages:services.columns.endpoints")}</th>
                                <th>{t("common:fields.age")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {rows.map((s) => (
                                <tr
                                    key={`${s.namespace}/${s.name}`}
                                    className="cursor-pointer"
                                    onClick={() => navigate(`/services/${encodeURIComponent(s.namespace)}/${encodeURIComponent(s.name)}`)}
                                >
                                    <td className="name">
                                        <Link
                                            to={`/services/${encodeURIComponent(s.namespace)}/${encodeURIComponent(s.name)}`}
                                            className="hover:underline"
                                            onClick={(e) => e.stopPropagation()}
                                        >
                                            {s.name}
                                        </Link>
                                    </td>
                                    <td className="mono">{s.namespace}</td>
                                    <td>{s.type}</td>
                                    <td className="mono">{s.clusterIP}</td>
                                    <td>
                                        <div className="flex flex-wrap gap-1">
                                            {(s.ports || []).map((p, i) => (
                                                <Badge key={i} variant="outline" className="mono text-[11px] bg-muted/30">
                                                    {p.port}
                                                    {p.targetPort ? `→${p.targetPort}` : ""}/{p.protocol || "TCP"}
                                                </Badge>
                                            ))}
                                        </div>
                                    </td>
                                    <td className="mono num">{(s.endpoints || []).length}</td>
                                    <td className="mono">{s.age}</td>
                                </tr>
                            ))}
                            {rows.length === 0 && (
                                <tr>
                                    <td colSpan={7}>
                                        <div className="empty">{t("pages:services.empty")}</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </GlassCard>
        </div>
    );
}
