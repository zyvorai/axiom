import { Search, Command, Zap } from "lucide-react";
import { Button } from "@/components/ui/button";
import { toast } from "sonner";
import { NAV } from "@/constants/testIds";
import UserMenu from "@/components/UserMenu";

// Full-width sticky header, matching zeus-os's WebShell.tsx header shape
// (h-14, border-b, backdrop-blur) rather than the floating glass pill this
// used to be. Axiom keeps a cluster-scoped "Connect kubeconfig" action and
// UserMenu here — zeus-os's header has neither since it isn't multi-tenant.
export default function TopBar() {
    const openPalette = () => window.dispatchEvent(new Event("axiom:palette"));
    return (
        <header className="sticky top-0 z-30 flex h-14 shrink-0 items-center gap-3 border-b border-border bg-background/95 px-4 backdrop-blur">
            <button
                data-testid={NAV.search}
                onClick={openPalette}
                className="flex w-full max-w-md items-center gap-2 rounded-full border border-input bg-foreground/[0.03] px-3 py-1.5 text-left transition-colors hover:bg-foreground/[0.06]"
            >
                <Search className="h-4 w-4 text-muted-foreground" />
                <span className="w-full truncate text-sm text-muted-foreground">Search resources, actions…</span>
                <span className="mono flex items-center gap-1 rounded-md border border-input px-1.5 py-0.5 text-[10px] text-muted-foreground">
                    <Command className="h-3 w-3" />K
                </span>
            </button>

            <div className="ml-auto flex items-center gap-2">
                <Button
                    data-testid={NAV.connect}
                    onClick={() =>
                        toast.info("Sample data mode", {
                            description: "Point KUBECONFIG at this dashboard to switch to a live cluster.",
                        })
                    }
                    variant="outline"
                >
                    <Zap className="mr-1.5 h-4 w-4 text-i17-cosmic" />
                    Connect kubeconfig
                </Button>

                <UserMenu />
            </div>
        </header>
    );
}
