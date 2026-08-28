import { useEffect, useMemo, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
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
            <div data-testid={STORAGE.root} className="space-y-4">
                <Skeleton className="h-40 rounded-2xl bg-white/[0.04]" />
                <Skeleton className="h-80 rounded-2xl bg-white/[0.04]" />
            </div>
        );
    }

    return (
        <div data-testid={STORAGE.root} className="space-y-4">
            <div>
                <h1 className="text-3xl font-semibold tracking-tight">Storage</h1>
                <p className="text-sm text-muted-foreground">
                    StorageClasses and PersistentVolumeClaims across all namespaces.
                </p>
            </div>

            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                {classes.map((sc) => (
                    <GlassCard key={sc.name} data-testid={STORAGE.classRow(sc.name)} className="p-4">
                        <div className="flex items-start justify-between gap-2">
                            <div className="grid h-9 w-9 place-items-center rounded-xl bg-[#6ab6f0]/15 text-[#6ab6f0]">
                                <Database className="h-4 w-4" />
                            </div>
                        </div>
                        <div className="mono mt-3 truncate text-sm">{sc.name}</div>
                        <div className="mt-3 flex items-center justify-between text-[11px] text-muted-foreground">
                            <span>Claims</span>
                            <span className="mono text-foreground">{sc.count}</span>
                        </div>
                    </GlassCard>
                ))}
            </div>

            <GlassCard className="p-0 overflow-hidden">
                <div className="border-b border-white/5 px-4 py-3 text-sm font-medium">PersistentVolumeClaims</div>
                <div className="max-h-[60vh] overflow-auto scrollbar-thin">
                    {volumes.map((p) => (
                        <div
                            key={`${p.namespace}/${p.name}`}
                            data-testid={STORAGE.pvcRow(p.name)}
                            className="grid grid-cols-12 items-center gap-3 border-b border-white/5 px-4 py-3 last:border-none hover:bg-white/[0.03]"
                        >
                            <div className="col-span-4 flex items-center gap-3 min-w-0">
                                <HardDrive className="h-4 w-4 text-[#b49bd8] shrink-0" />
                                <div className="min-w-0">
                                    <div className="mono truncate text-sm">{p.name}</div>
                                    <div className="text-[11px] text-muted-foreground">
                                        {p.namespace} · {p.storageClass}
                                    </div>
                                </div>
                            </div>
                            <div className="col-span-2 text-xs text-muted-foreground">
                                <span className="mono text-foreground">{p.capacity}</span>
                            </div>
                            <div className="col-span-3 mono truncate text-xs text-muted-foreground">{p.volumeName}</div>
                            <div className="col-span-2 mono text-xs text-muted-foreground">{p.accessModes}</div>
                            <div className="col-span-1 text-right">
                                <StatusBadge status={p.status} />
                            </div>
                        </div>
                    ))}
                </div>
            </GlassCard>

            <div className="grid gap-4 lg:grid-cols-2">
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
