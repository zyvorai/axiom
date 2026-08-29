import { useCluster } from "@/lib/cluster";
import StatusBadge from "@/components/StatusBadge";
import { ChevronsUpDown, Check, Cloud } from "lucide-react";
import {
    DropdownMenu,
    DropdownMenuContent,
    DropdownMenuItem,
    DropdownMenuLabel,
    DropdownMenuSeparator,
    DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { NAV } from "@/constants/testIds";

export default function ClusterSwitcher() {
    const { clusters, active, activate } = useCluster();
    if (!active) return null;

    const single = clusters.length <= 1;

    const trigger = (
        <button
            data-testid={NAV.clusterSwitcher}
            disabled={single}
            className="glass-inset specular w-full rounded-2xl px-3 py-2.5 text-left transition hover:bg-white/[0.06] disabled:cursor-default disabled:hover:bg-transparent"
        >
            <div className="flex items-center gap-3">
                <div className="grid h-9 w-9 place-items-center rounded-xl bg-gradient-to-br from-i17-cosmic/30 to-i17-sky/20 border border-white/10">
                    <Cloud className="h-4 w-4 text-i17-cosmic" />
                </div>
                <div className="min-w-0 flex-1">
                    <div className="truncate text-sm font-medium">{active.label || active.name}</div>
                    <div className="mt-0.5 flex items-center gap-2 text-[11px] text-muted-foreground">
                        <StatusBadge status={active.healthy ? "Healthy" : "NotReady"} />
                        <span>·</span>
                        <span className="truncate">Kubernetes</span>
                    </div>
                </div>
                {!single && <ChevronsUpDown className="h-4 w-4 text-muted-foreground shrink-0" />}
            </div>
        </button>
    );

    if (single) return trigger;

    return (
        <DropdownMenu>
            <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-72">
                <DropdownMenuLabel className="text-xs uppercase tracking-widest text-muted-foreground">
                    Clusters
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                {clusters.map((c) => (
                    <DropdownMenuItem
                        key={c.name}
                        data-testid={NAV.clusterOption(c.name)}
                        onSelect={() => activate(c.name)}
                        className="flex items-start gap-2 py-2.5"
                    >
                        <Check className={`mt-0.5 h-4 w-4 ${c.name === active.name ? "opacity-100" : "opacity-0"}`} />
                        <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-2 text-sm">
                                <span className="truncate font-medium">{c.label || c.name}</span>
                                <StatusBadge status={c.healthy ? "Healthy" : "NotReady"} />
                            </div>
                            <div className="mt-0.5 truncate text-xs text-muted-foreground">{c.server || c.name}</div>
                        </div>
                    </DropdownMenuItem>
                ))}
            </DropdownMenuContent>
        </DropdownMenu>
    );
}
