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
    const { clusters, active, setActiveId } = useCluster();
    if (!active) return null;

    return (
        <DropdownMenu>
            <DropdownMenuTrigger asChild>
                <button
                    data-testid={NAV.clusterSwitcher}
                    className="glass-inset specular w-full rounded-2xl px-3 py-2.5 text-left transition hover:bg-white/[0.06]"
                >
                    <div className="flex items-center gap-3">
                        <div className="grid h-9 w-9 place-items-center rounded-xl bg-gradient-to-br from-[#ff7550]/30 to-[#6ab6f0]/20 border border-white/10">
                            <Cloud className="h-4 w-4 text-[#ff7550]" />
                        </div>
                        <div className="min-w-0 flex-1">
                            <div className="truncate text-sm font-medium">{active.name}</div>
                            <div className="mt-0.5 flex items-center gap-2 text-[11px] text-muted-foreground">
                                <StatusBadge status={active.status} />
                                <span>·</span>
                                <span className="truncate">{active.provider}</span>
                            </div>
                        </div>
                        <ChevronsUpDown className="h-4 w-4 text-muted-foreground shrink-0" />
                    </div>
                </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" className="w-72">
                <DropdownMenuLabel className="text-xs uppercase tracking-widest text-muted-foreground">
                    Clusters & Tenants
                </DropdownMenuLabel>
                <DropdownMenuSeparator />
                {clusters.map((c) => (
                    <DropdownMenuItem
                        key={c.id}
                        data-testid={NAV.clusterOption(c.id)}
                        onSelect={() => setActiveId(c.id)}
                        className="flex items-start gap-2 py-2.5"
                    >
                        <Check className={`mt-0.5 h-4 w-4 ${c.id === active.id ? "opacity-100" : "opacity-0"}`} />
                        <div className="min-w-0 flex-1">
                            <div className="flex items-center gap-2 text-sm">
                                <span className="truncate font-medium">{c.name}</span>
                                <StatusBadge status={c.status} />
                            </div>
                            <div className="mt-0.5 truncate text-xs text-muted-foreground">
                                {c.provider} · {c.region}
                            </div>
                        </div>
                    </DropdownMenuItem>
                ))}
            </DropdownMenuContent>
        </DropdownMenu>
    );
}
