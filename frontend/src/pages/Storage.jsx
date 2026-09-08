import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import AutoTable from "@/components/AutoTable";
import { Skeleton } from "@/components/ui/skeleton";
import { Database, HardDrive, Layers } from "lucide-react";
import { STORAGE } from "@/constants/testIds";

export default function Storage() {
    const [volumes, setVolumes] = useState(null);
    const [storageClasses, setStorageClasses] = useState(null);
    const [persistentVolumes, setPersistentVolumes] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/volumes").then((r) => alive && setVolumes(r.data));
        load();
        api.get("/storageclasses").then((r) => alive && setStorageClasses(r.data || [])).catch(() => alive && setStorageClasses([]));
        api.get("/persistentvolumes").then((r) => alive && setPersistentVolumes(r.data || [])).catch(() => alive && setPersistentVolumes([]));
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
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
                title="Storage"
                description="StorageClasses and PersistentVolumeClaims across all namespaces."
            />

            <div className="grid g3">
                {classes.map((sc) => (
                    <div key={sc.name} data-testid={STORAGE.classRow(sc.name)} className="stat">
                        <Database size={18} style={{ color: "var(--blue)" }} />
                        <span className="mono" style={{ fontSize: 14, marginTop: 12, display: "block" }}>
                            {sc.name}
                        </span>
                        <em>Claims · {sc.count}</em>
                    </div>
                ))}
            </div>

            <div className="card overflow-hidden" style={{ marginTop: 14, padding: 0 }}>
                <div style={{ overflowX: "auto" }}>
                    <table className="tbl">
                        <thead>
                            <tr>
                                <th>Claim</th>
                                <th>Namespace</th>
                                <th>Class</th>
                                <th>Size</th>
                                <th>Volume</th>
                                <th>Access</th>
                                <th>Status</th>
                            </tr>
                        </thead>
                        <tbody>
                            {volumes.map((p) => (
                                <tr key={`${p.namespace}/${p.name}`} data-testid={STORAGE.pvcRow(p.name)}>
                                    <td className="name">
                                        <span style={{ display: "flex", gap: 8, alignItems: "center" }}>
                                            <HardDrive size={15} style={{ color: "var(--ink-3)" }} />
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
                        </tbody>
                    </table>
                </div>
            </div>

            <div className="section-h" style={{ marginTop: 40 }}>
                <h2>Raw resources</h2>
            </div>
            <div className="grid g2">
                <AutoTable
                    title="StorageClasses"
                    icon={Layers}
                    items={storageClasses}
                    emptyHint={storageClasses === null ? "Loading…" : "No StorageClasses found."}
                />
                <AutoTable
                    title="PersistentVolumes"
                    icon={HardDrive}
                    items={persistentVolumes}
                    emptyHint={persistentVolumes === null ? "Loading…" : "No PersistentVolumes found."}
                />
            </div>
        </div>
    );
}
