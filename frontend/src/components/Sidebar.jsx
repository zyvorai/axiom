import { useEffect, useState } from "react";
import { NavLink } from "react-router-dom";
import { ChevronsLeft, ChevronsRight } from "lucide-react";
import ClusterSwitcher from "@/components/ClusterSwitcher";
import { NAV } from "@/constants/testIds";
import { NAV_ITEMS, NAV_GROUP_ORDER, RAIL_PINNED_KEYS, resolveIcon } from "@/constants/navItems";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";

const COLLAPSE_KEY = "axiom:sidebar-collapsed";

function loadCollapsed() {
    try {
        return localStorage.getItem(COLLAPSE_KEY) === "1";
    } catch {
        return false;
    }
}

function saveCollapsed(value) {
    try {
        localStorage.setItem(COLLAPSE_KEY, value ? "1" : "0");
    } catch {
        // ignore
    }
}

// Apple.com-flat rail: same canvas as the page, text color only for state —
// no card chrome, no hover pills, no active fill.
const linkIdle = "text-[#6e6e73] dark:text-[#8b949e]";
const linkHover = "hover:text-[#1d1d1f] dark:hover:text-[#e6edf3]";
const linkActive = "text-[#1d1d1f] dark:text-[#e6edf3]";

export default function Sidebar() {
    const [plugins, setPlugins] = useState([]);
    const [collapsed, setCollapsed] = useState(loadCollapsed);

    useEffect(() => {
        api.get("/plugins").then((r) => setPlugins(r.data || [])).catch(() => setPlugins([]));
    }, []);

    const ITEMS = [
        ...NAV_ITEMS,
        ...plugins.map((p) => ({
            to: `/plugins/${p.name}`,
            label: p.displayName,
            icon: resolveIcon(p.icon),
            key: `plugin-${p.name}`,
            group: "Extensions",
        })),
    ];

    const groups = NAV_GROUP_ORDER.map((label) => ({
        label,
        items: ITEMS.filter((i) => (i.group || "Extensions") === label),
    })).filter((g) => g.items.length > 0);

    const railItems = RAIL_PINNED_KEYS.map((key) => ITEMS.find((i) => i.key === key)).filter(Boolean);

    const toggleCollapsed = () => {
        setCollapsed((v) => {
            saveCollapsed(!v);
            return !v;
        });
    };

    return (
        <aside
            data-testid={NAV.sidebar}
            className={cn(
                "rail-apple hidden h-full shrink-0 flex-col border-r border-black/[0.06] bg-transparent py-5 transition-[width] duration-200 ease-out dark:border-white/[0.06] lg:flex",
                collapsed ? "w-11 px-0" : "w-48 px-5"
            )}
        >
            {!collapsed && (
                <div className="mb-6 border-b border-black/[0.06] pb-4 dark:border-white/[0.06]">
                    <ClusterSwitcher />
                </div>
            )}

            <nav
                id="axiom-primary-nav"
                aria-label="Primary"
                className={cn("scrollbar-thin flex-1 overflow-y-auto", collapsed ? "space-y-1" : "space-y-5")}
            >
                {collapsed ? (
                    <ul className="space-y-1">
                        {railItems.map(({ to, label, icon: Icon, key }) => (
                            <li key={to}>
                                <NavLink
                                    to={to}
                                    data-testid={NAV.link(key)}
                                    title={label}
                                    className={({ isActive }) =>
                                        cn("flex items-center justify-center py-2 transition-colors duration-150", linkHover, isActive ? linkActive : linkIdle)
                                    }
                                >
                                    <Icon className="h-4 w-4 shrink-0" strokeWidth={1.5} />
                                </NavLink>
                            </li>
                        ))}
                    </ul>
                ) : (
                    groups.map((group) => (
                        <div key={group.label}>
                            <p className="rail-apple-group mb-1.5 text-[#86868b] dark:text-[#6e7681]">{group.label}</p>
                            <ul className="space-y-0.5">
                                {group.items.map(({ to, label, icon: Icon, key }) => (
                                    <li key={to}>
                                        <NavLink
                                            to={to}
                                            data-testid={NAV.link(key)}
                                            className={({ isActive }) =>
                                                cn(
                                                    "rail-apple-link group flex items-center gap-2.5 py-1 transition-colors duration-150",
                                                    linkHover,
                                                    isActive ? cn(linkActive, "is-active") : linkIdle
                                                )
                                            }
                                        >
                                            <Icon className="h-3.5 w-3.5 shrink-0 opacity-70" strokeWidth={1.5} />
                                            <span className="truncate">{label}</span>
                                        </NavLink>
                                    </li>
                                ))}
                            </ul>
                        </div>
                    ))
                )}
            </nav>

            <button
                data-testid={NAV.sidebarCollapseToggle}
                type="button"
                onClick={toggleCollapsed}
                aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
                className={cn(
                    "rail-apple-meta mt-4 flex items-center gap-1.5 py-1 transition-colors duration-150",
                    linkIdle,
                    linkHover,
                    collapsed && "justify-center"
                )}
            >
                {collapsed ? <ChevronsRight className="h-3.5 w-3.5" strokeWidth={1.5} /> : <ChevronsLeft className="h-3.5 w-3.5" strokeWidth={1.5} />}
                {!collapsed && <span>Collapse</span>}
            </button>
        </aside>
    );
}
