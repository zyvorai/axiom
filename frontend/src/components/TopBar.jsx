import { Link } from "react-router-dom";
import { Search, Command, Zap } from "lucide-react";
import { Button } from "@/components/ui/button";
import { toast } from "sonner";
import { NAV } from "@/constants/testIds";
import UserMenu from "@/components/UserMenu";
import ZyvorMark from "@/components/ZyvorMark";

// Apple-style top chrome: brand mark flush left, compact actions flush right.
// Search is a small chip on the right (opens the command palette) — not a
// wide centered field that dominates the bar.
export default function TopBar() {
    const openPalette = () => window.dispatchEvent(new Event("axiom:palette"));
    return (
        <header className="sticky top-0 z-30 flex h-12 shrink-0 items-center gap-3 border-b border-border bg-background/95 px-4 backdrop-blur">
            <Link to="/overview" className="flex shrink-0 items-center gap-2" aria-label="Axiom home">
                <ZyvorMark size={28} idPrefix="axiom-topbar" className="shrink-0" />
                <span className="hidden text-[13px] font-semibold tracking-[-0.02em] sm:inline">
                    <span className="lowercase">zyvor</span>
                    <span className="ml-1.5 text-[10px] font-semibold uppercase tracking-[0.2em] text-i17-cosmic/90">Axiom</span>
                </span>
            </Link>

            <div className="ml-auto flex items-center gap-1.5">
                <button
                    data-testid={NAV.search}
                    onClick={openPalette}
                    title="Search (⌘K)"
                    className="flex h-8 items-center gap-1.5 rounded-full border border-input bg-foreground/[0.03] px-2.5 text-muted-foreground transition-colors hover:bg-foreground/[0.06] hover:text-foreground"
                >
                    <Search className="h-3.5 w-3.5" />
                    <span className="hidden text-xs sm:inline">Search</span>
                    <span className="mono hidden items-center gap-0.5 rounded border border-input px-1 py-px text-[9px] sm:flex">
                        <Command className="h-2.5 w-2.5" />K
                    </span>
                </button>

                <Button
                    data-testid={NAV.connect}
                    size="sm"
                    onClick={() =>
                        toast.info("Sample data mode", {
                            description: "Point KUBECONFIG at this dashboard to switch to a live cluster.",
                        })
                    }
                    variant="ghost"
                    className="hidden h-8 px-2.5 text-xs text-muted-foreground hover:text-foreground md:inline-flex"
                >
                    <Zap className="mr-1 h-3.5 w-3.5 text-i17-cosmic" />
                    Connect
                </Button>

                <UserMenu />
            </div>
        </header>
    );
}
