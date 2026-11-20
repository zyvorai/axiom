import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import AutoTable from "@/components/AutoTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Database, HardDrive, Layers } from "lucide-react";
import { STORAGE } from "@/constants/testIds";
import SearchBox from "@/components/SearchBox";
import NamespaceFilter from "@/components/NamespaceFilter";
import SavedViews from "@/components/SavedViews";
import { useUrlState } from "@/lib/urlState";
import { useNamespaceFilter } from "@/lib/namespace";

export default function Storage() {
    const { t } = useTranslation();
    const [volumes, setVolumes] = useState(null);
    const [storageClasses, setStorageClasses] = useState(null);
    const [persistentVolumes, setPersistentVolumes] = useState(null);
    const [q, setQ] = useUrlState("q", "");
    const [ns, setNs, knownNamespaces, globalNs] = useNamespaceFilter();

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/volumes").then((r) => alive && setVolumes(r.data));
        load();
        api.get("/storageclasses").then((r) => alive && setStorageClasses(r.data || [])).catch(() => alive && setStorageClasses([]));
        api.get("/persistentvolumes").then((r) => alive && setPersistentVolumes(r.data || [])).catch(() => alive && setPersistentVolumes([]));
        const timer = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    const classes = useMemo(() => {
        if (!volumes) return [];
        const byClass = new Map();
        for (const v of volumes) {
            const key = v.storageClass || "—";
            if (!byClass.has(key)) byClass.set(key, { name: key, count: 0 });
            byClass.get(key).count++;
        }
        return [...byClass.values()];
    }, [volumes]);

    const claims = useMemo(
        () =>
            (volumes || []).filter(
                (p) => (ns === "all" || p.namespace === ns) && (q === "" || p.name.toLowerCase().includes(q.toLowerCase()))
            ),
        [volumes, q, ns]
    );
    const namespaces = useMemo(
        () => [...new Set([...knownNamespaces, ...(volumes || []).map((p) => p.namespace)])].sort(),
        [knownNamespaces, volumes]
    );

    if (!volumes) {
        return (
            <div data-testid={STORAGE.root} className="page">
                <Skeleton className="h-40 rounded-2xl bg-muted" />
                <Skeleton className="mt-4 h-80 rounded-2xl bg-muted" />
            </div>
        );
    }

    return (
        <div data-testid={STORAGE.root} className="page">
            <PageHeader
                title={t("pages:storage.title")}
                description={t("pages:storage.description")}
                tools={
                    <>
                        <SearchBox placeholder={t("pages:storage.searchPlaceholder")} value={q} onChange={setQ} />
                        <NamespaceFilter value={ns} onChange={setNs} namespaces={namespaces} />
                        <SavedViews
                            pageKey="storage"
                            filters={{ q, ns }}
                            onApply={(f) => {
                                setQ(f.q ?? "");
                                setNs(f.ns ?? globalNs);
                            }}
                        />
                    </>
                }
            />

            <div className="grid g3">
                {classes.map((sc) => (
                    <div key={sc.name} data-testid={STORAGE.classRow(sc.name)} className="stat">
                        <b>{sc.count}</b>
                        <span>
                            <Database size={14} className="text-tone-link" aria-hidden="true" />
                            <span className="mono">{sc.name}</span>
                        </span>
                        <em>{t("pages:storage.claimUnit", { count: sc.count })}</em>
                    </div>
                ))}
            </div>

            <div className="card overflow-hidden mt-3.5 p-0">
                <div className="overflow-x-auto" tabIndex={0} role="region" aria-label={t("pages:storage.claimsRegion")}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>{t("pages:storage.columns.claim")}</th>
                                <th>{t("common:fields.namespace")}</th>
                                <th>{t("pages:storage.columns.class")}</th>
                                <th>{t("pages:storage.columns.size")}</th>
                                <th>{t("pages:storage.columns.volume")}</th>
                                <th>{t("pages:storage.columns.access")}</th>
                                <th>{t("common:fields.status")}</th>
                            </tr>
                        </thead>
                        <tbody>
                            {claims.map((p) => (
                                <tr key={`${p.namespace}/${p.name}`} data-testid={STORAGE.pvcRow(p.name)}>
                                    <td className="name">
                                        <span className="flex gap-2 items-center">
                                            <HardDrive size={15} className="text-ink-3" />
                                            {p.name}
                                        </span>
                                    </td>
                                    <td className="mono">{p.namespace}</td>
                                    <td className="mono">{p.storageClass}</td>
                                    <td className="num">{p.capacity}</td>
                                    <td className="mono num">{p.volumeName || "—"}</td>
                                    <td className="num">{p.accessModes}</td>
                                    <td>
                                        <StatusBadge status={p.status} />
                                    </td>
                                </tr>
                            ))}
                            {claims.length === 0 && (
                                <tr>
                                    <td colSpan={7}>
                                        <div className="empty">{t("pages:storage.empty")}</div>
                                    </td>
                                </tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </div>

            <div className="section-h mt-10">
                <h2>{t("pages:storage.rawResources")}</h2>
            </div>
            <div className="grid g2">
                <AutoTable
                    title="StorageClasses"
                    icon={Layers}
                    items={storageClasses}
                    emptyHint={storageClasses === null ? t("common:states.loading") : t("pages:storage.noStorageClasses")}
                />
                <AutoTable
                    title="PersistentVolumes"
                    icon={HardDrive}
                    items={persistentVolumes}
                    emptyHint={persistentVolumes === null ? t("common:states.loading") : t("pages:storage.noPersistentVolumes")}
                />
            </div>
        </div>
    );
}
