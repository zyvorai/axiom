import { Link } from "react-router-dom";
import { Search, Command, Zap, PanelLeft, Bell, Sun, Moon } from "lucide-react";
import { toast } from "sonner";
import { NAV } from "@/constants/testIds";
import { useCluster } from "@/lib/cluster";
import { useTheme } from "@/lib/theme";
import { useAlertsCount } from "@/lib/alerts";
import UserMenu from "@/components/UserMenu";
import ZyvorMark from "@/components/ZyvorMark";
import Breadcrumbs from "@/components/Breadcrumbs";

export default function TopBar({ onToggleSidebar }) {
    const { theme, setTheme } = useTheme();
    const { active } = useCluster();
    const alertCount = useAlertsCount();
    const openPalette = () => window.dispatchEvent(new Event("axiom:palette"));

    return (
        <header className="bar">
            <button type="button" className="ic" onClick={onToggleSidebar} aria-label="Toggle sidebar">
                <PanelLeft size={17} />
            </button>

            <Link to="/overview" className="brand" aria-label="Axiom home">
                <ZyvorMark size={22} idPrefix="axiom-topbar" className="brand-mark" />
                <span className="brand-name">
                    Axiom<span>by Zyvor</span>
                </span>
            </Link>

            {active && (
                <Link to="/settings#clusters" className="cluster" data-testid={NAV.clusterSwitcher}>
                    <i aria-hidden="true" />
                    {active.label || active.name}
                </Link>
            )}

            <Breadcrumbs />

            <div className="bar-r">
                <button type="button" data-testid={NAV.search} onClick={openPalette} className="search" title="Search (⌘K)">
                    <Search size={14} />
                    <span className="hidden sm:inline">Search</span>
                    <kbd>
                        <Command size={10} style={{ display: "inline", verticalAlign: "-1px" }} />K
                    </kbd>
                </button>

                <button
                    type="button"
                    data-testid={NAV.connect}
                    className="btn btn-line"
                    style={{ height: 30 }}
                    onClick={() =>
                        toast.info("Sample data mode", {
                            description: "Point KUBECONFIG at this dashboard to switch to a live cluster.",
                        })
                    }
                >
                    <Zap size={14} />
                    Connect
                </button>

                <Link to="/alerts" className="ic" aria-label="Alerts">
                    <Bell size={16} />
                    {alertCount > 0 && <span className="dot" />}
                </Link>

                <button
                    type="button"
                    data-testid={NAV.themeToggle}
                    className="ic"
                    aria-label="Toggle theme"
                    onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
                >
                    {theme === "dark" ? <Sun size={16} /> : <Moon size={16} />}
                </button>

                <UserMenu />
            </div>
        </header>
    );
}
