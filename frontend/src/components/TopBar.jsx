import { Link } from "react-router-dom";
import { Search, Command, Zap, PanelLeft, Sun, Moon, Monitor } from "lucide-react";
import { toast } from "sonner";
import { NAV, NS_SELECT } from "@/constants/testIds";
import { useCluster } from "@/lib/cluster";
import { THEME_LABELS, nextPreference, useTheme } from "@/lib/theme";
import { ALL_NAMESPACES, useNamespace } from "@/lib/namespace";
import NotificationInbox from "@/components/NotificationInbox";
import UserMenu from "@/components/UserMenu";
import ZyvorMark from "@/components/ZyvorMark";
import Breadcrumbs from "@/components/Breadcrumbs";

export default function TopBar({ onToggleSidebar }) {
    const { preference, setTheme } = useTheme();
    const { active } = useCluster();
    const { namespace, namespaces, setNamespace } = useNamespace();
    const ThemeIcon = { light: Sun, dark: Moon, system: Monitor }[preference] || Moon;
    // keep the current choice selectable even if it is not (or no longer) in the list
    const nsOptions = [...new Set([...namespaces, ...(namespace !== ALL_NAMESPACES ? [namespace] : [])])].sort();
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

            <label className="sr-only" htmlFor="topbar-namespace-select">
                Namespace
            </label>
            <select
                id="topbar-namespace-select"
                data-testid={NS_SELECT.global}
                className="ns-switch"
                value={namespace}
                onChange={(e) => setNamespace(e.target.value)}
                title="Namespace scope for all pages"
            >
                <option value={ALL_NAMESPACES}>All namespaces</option>
                {nsOptions.map((n) => (
                    <option key={n} value={n}>
                        {n}
                    </option>
                ))}
            </select>

            <Breadcrumbs />

            <div className="bar-r">
                <button type="button" data-testid={NAV.search} onClick={openPalette} className="search" title="Search (⌘K)">
                    <Search size={14} />
                    <span className="hidden sm:inline">Search</span>
                    <kbd>
                        <Command size={10} className="inline align-[-1px]" />K
                    </kbd>
                </button>

                <button
                    type="button"
                    data-testid={NAV.connect}
                    className="btn btn-line h-[30px]"
                    onClick={() =>
                        toast.info("Sample data mode", {
                            description: "Point KUBECONFIG at this dashboard to switch to a live cluster.",
                        })
                    }
                >
                    <Zap size={14} />
                    Connect
                </button>

                <NotificationInbox />

                <button
                    type="button"
                    data-testid={NAV.themeToggle}
                    className="ic"
                    aria-label={`Toggle theme (currently ${THEME_LABELS[preference] || "Dark"})`}
                    title={`Theme: ${THEME_LABELS[preference] || "Dark"}`}
                    onClick={() => setTheme(nextPreference(preference))}
                >
                    <ThemeIcon size={16} aria-hidden="true" />
                </button>

                <UserMenu />
            </div>
        </header>
    );
}
