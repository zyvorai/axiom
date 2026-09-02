import { useEffect, useState } from "react";
import { NavLink } from "react-router-dom";
import { ChevronsLeft, ChevronsRight } from "lucide-react";
import ClusterSwitcher from "@/components/ClusterSwitcher";
import ThemeToggle from "@/components/ThemeToggle";
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
        // ignore (private browsing, storage disabled, etc.)
    }
}

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
                "hidden h-full shrink-0 flex-col border-r border-border bg-background p-3 transition-[width] duration-200 ease-out lg:flex",
                collapsed ? "w-[68px]" : "w-64"
            )}
        >
            {/* Brand lives in TopBar — sidebar starts with cluster / nav. */}
            {!collapsed && (
                <div className="px-1 pb-2">
                    <ClusterSwitcher />
                </div>
            )}

            <nav
                id="axiom-primary-nav"
                aria-label="Primary"
                className={cn("scrollbar-thin flex-1 overflow-y-auto", collapsed ? "space-y-0.5" : "mt-1 space-y-3")}
            >
                {collapsed ? (
                    // Collapsed rail: a plain flat list of a few pinned, one-per-domain
                    // destinations — no group headers or dividers — mirroring zeus-os's
                    // own minimal rail instead of cramming every item into icon form.
                    <ul className="space-y-0.5">
                        {railItems.map(({ to, label, icon: Icon, key }) => (
                            <li key={to}>
                                <NavLink
                                    to={to}
                                    data-testid={NAV.link(key)}
                                    title={label}
                                    className={({ isActive }) =>
                                        cn(
                                            "group flex items-center justify-center rounded-xl px-2 py-2 text-sm font-medium transition-colors duration-200",
                                            isActive
                                                ? "bg-primary/12 text-foreground"
                                                : "text-muted-foreground hover:bg-muted hover:text-foreground"
                                        )
                                    }
                                >
                                    {({ isActive }) => (
                                        <Icon
                                            className={cn(
                                                "h-4 w-4 shrink-0 transition-colors",
                                                isActive ? "text-primary" : "text-muted-foreground group-hover:text-foreground"
                                            )}
                                        />
                                    )}
                                </NavLink>
                            </li>
                        ))}
                    </ul>
                ) : (
                    groups.map((group) => (
                        <div key={group.label}>
                            <p className="px-3 py-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
                                {group.label}
                            </p>
                            <ul className="space-y-0.5">
                                {group.items.map(({ to, label, icon: Icon, key }) => (
                                    <li key={to}>
                                        <NavLink
                                            to={to}
                                            data-testid={NAV.link(key)}
                                            className={({ isActive }) =>
                                                cn(
                                                    "group flex items-center gap-3 rounded-xl px-3 py-2 text-sm font-medium transition-colors duration-200",
                                                    isActive
                                                        ? "bg-primary/12 text-foreground"
                                                        : "text-muted-foreground hover:bg-muted hover:text-foreground"
                                                )
                                            }
                                        >
                                            {({ isActive }) => (
                                                <>
                                                    <Icon
                                                        className={cn(
                                                            "h-4 w-4 shrink-0 transition-colors",
                                                            isActive ? "text-primary" : "text-muted-foreground group-hover:text-foreground"
                                                        )}
                                                    />
                                                    <span className="truncate">{label}</span>
                                                    {isActive && <span className="ml-auto h-1.5 w-1.5 rounded-full bg-primary" />}
                                                </>
                                            )}
                                        </NavLink>
                                    </li>
                                ))}
                            </ul>
                        </div>
                    ))
                )}
            </nav>

            {!collapsed && (
                <div className="mt-2 flex items-center justify-between rounded-2xl border border-border bg-muted/40 px-3 py-2">
                    <div className="text-xs text-muted-foreground">Appearance</div>
                    <ThemeToggle />
                </div>
            )}

            <button
                data-testid={NAV.sidebarCollapseToggle}
                onClick={toggleCollapsed}
                aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
                className={cn(
                    "mt-2 flex items-center justify-center gap-2 rounded-xl border border-border bg-muted/40 py-2 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground",
                    collapsed ? "px-2" : "px-3"
                )}
            >
                {collapsed ? <ChevronsRight className="h-4 w-4" /> : <ChevronsLeft className="h-4 w-4" />}
                {!collapsed && <span>Collapse</span>}
            </button>
        </aside>
    );
}
