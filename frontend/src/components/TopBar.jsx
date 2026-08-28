import { Search, Command, Zap } from "lucide-react";
import { useCluster } from "@/lib/cluster";
import { Button } from "@/components/ui/button";
import { toast } from "sonner";
import { NAV } from "@/constants/testIds";
import UserMenu from "@/components/UserMenu";

export default function TopBar() {
    const { active } = useCluster();
    const openPalette = () => window.dispatchEvent(new Event("axiom:palette"));
    return (
        <div className="glass specular flex items-center gap-3 rounded-2xl px-4 py-2.5">
            <div className="hidden items-center gap-1 text-xs text-muted-foreground sm:flex">
                <span className="text-foreground/70">Clusters</span>
                <span className="opacity-40">/</span>
                <span className="text-foreground/90">{active?.name ?? "—"}</span>
            </div>

            <button
                data-testid={NAV.search}
                onClick={openPalette}
                className="ml-auto flex flex-1 max-w-md cursor-pointer items-center gap-2 rounded-xl border border-white/10 bg-white/[0.03] px-3 py-1.5 text-left transition-colors hover:bg-white/[0.06]"
            >
                <Search className="h-4 w-4 text-muted-foreground" />
                <span className="w-full text-sm text-muted-foreground">Search pods, services, nodes…</span>
                <span className="mono flex items-center gap-1 rounded-md border border-white/10 px-1.5 py-0.5 text-[10px] text-muted-foreground">
                    <Command className="h-3 w-3" />K
                </span>
            </button>

            <Button
                data-testid={NAV.connect}
                onClick={() =>
                    toast.info("Sample data mode", {
                        description: "Point KUBECONFIG at this dashboard to switch to a live cluster.",
                    })
                }
                variant="outline"
                className="glass-inset specular rounded-xl border-white/10 bg-white/[0.03] hover:bg-white/[0.08]"
            >
                <Zap className="mr-1.5 h-4 w-4 text-[#ff7550]" />
                Connect kubeconfig
            </Button>

            <UserMenu />
        </div>
    );
}
