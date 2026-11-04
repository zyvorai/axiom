import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { useTranslation } from "react-i18next";

export default function PlatformIntents() {
    const { t } = useTranslation();
    const [items, setItems] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/platform-intents").then((r) => alive && setItems(r.data));
        load();
        const timer = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    if (!items) {
        return (
            <div className="page">
                <Skeleton className="h-16 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div className="page">
            <PageHeader
                title={t("pages:platformIntents.title")}
                description={t("pages:platformIntents.description")}
            />

            <GlassCard className="overflow-hidden p-0">
                <div className="max-h-[65vh] overflow-y-auto scrollbar-thin overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:platformIntents.tableAria")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("common:fields.name")}</th>
                                <th>{t("common:fields.namespace")}</th>
                                <th>{t("common:fields.kind")}</th>
                                <th>{t("common:fields.image")}</th>
                                <th>{t("pages:platformIntents.columns.resources")}</th>
                                <th>{t("common:fields.status")}</th>
                                <th>{t("common:fields.age")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {items.map((it, i) => (
                                <tr key={i}>
                                    <td className="name mono">
                                        {it.name}
                                        {it.message && <small>{it.message}</small>}
                                    </td>
                                    <td className="mono">{it.namespace}</td>
                                    <td>{it.workloadKind}</td>
                                    <td className="mono">{it.image}</td>
                                    <td className="num">
                                        {it.replicas ? `${it.replicas}x ` : ""}
                                        {it.cpu} {it.memory}
                                    </td>
                                    <td>
                                        <StatusBadge status={it.phase} />
                                    </td>
                                    <td className="num">{it.age}</td>
                                </tr>
                            ))}
                            {items.length === 0 && (
                                <tr>
                                    <td colSpan={7}>
                                        <div className="empty">{t("pages:platformIntents.empty")}</div>
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
