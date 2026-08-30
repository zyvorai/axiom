import { useEffect, useState } from "react";
import { NavLink } from "react-router-dom";
import { ChevronsLeft, ChevronsRight } from "lucide-react";
import ClusterSwitcher from "@/components/ClusterSwitcher";
import ThemeToggle from "@/components/ThemeToggle";
import { NAV } from "@/constants/testIds";
import { NAV_ITEMS, NAV_GROUP_ORDER, resolveIcon } from "@/constants/navItems";
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
                "glass specular hidden h-full shrink-0 flex-col p-3 transition-[width] duration-200 ease-out lg:flex",
                collapsed ? "w-[68px]" : "w-64"
            )}
        >
            <div className={cn("flex items-center gap-2.5 px-2 pt-1 pb-3", collapsed && "justify-center px-0")}>
                <div className="relative grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-i17-cosmic via-i17-cosmic to-i17-deep-blue shadow-lg shadow-i17-cosmic/30 ring-1 ring-white/10">
                    <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
                        <path
                            d="M2 2h14L6.6 16H16"
                            fill="none"
                            stroke="white"
                            strokeWidth="2.6"
                            strokeLinejoin="round"
                            strokeLinecap="round"
                        />
                    </svg>
                </div>
                {!collapsed && (
                    <div className="min-w-0 leading-tight">
                        <div className="flex items-baseline gap-1.5">
                            <span className="text-[15px] font-semibold tracking-[-0.02em] lowercase">zyvor</span>
                            <span className="text-[10px] uppercase tracking-[0.22em] text-i17-cosmic/90">Axiom</span>
                        </div>
                        <div className="truncate text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                            Multi-cluster Console
                        </div>
                    </div>
                )}
            </div>

            {!collapsed && <ClusterSwitcher />}

            <nav
                id="axiom-primary-nav"
                aria-label="Primary"
                className="scrollbar-thin mt-4 flex-1 space-y-3 overflow-y-auto"
            >
                {groups.map((group) => (
                    <div key={group.label}>
                        {collapsed ? (
                            <div className="my-2 h-px bg-white/5" aria-hidden="true" />
                        ) : (
                            <p className="px-3 py-1 text-[11px] font-semibold uppercase tracking-wider text-muted-foreground/70">
                                {group.label}
                            </p>
                        )}
                        <ul className="space-y-0.5">
                            {group.items.map(({ to, label, icon: Icon, key }) => (
                                <li key={to}>
                                    <NavLink
                                        to={to}
                                        data-testid={NAV.link(key)}
                                        title={collapsed ? label : undefined}
                                        className={({ isActive }) =>
                                            cn(
                                                "group flex items-center gap-3 rounded-xl px-3 py-2 text-sm transition-colors duration-200",
                                                collapsed && "justify-center px-2",
                                                isActive
                                                    ? "bg-white/10 text-foreground shadow-inner shadow-white/5"
                                                    : "text-muted-foreground hover:bg-white/[0.05] hover:text-foreground"
                                            )
                                        }
                                    >
                                        {({ isActive }) => (
                                            <>
                                                <Icon
                                                    className={cn(
                                                        "h-4 w-4 shrink-0 transition-colors",
                                                        isActive ? "text-i17-cosmic" : "text-muted-foreground group-hover:text-foreground"
                                                    )}
                                                />
                                                {!collapsed && <span className="truncate">{label}</span>}
                                                {!collapsed && isActive && (
                                                    <span className="ml-auto h-1.5 w-1.5 rounded-full bg-i17-cosmic" />
                                                )}
                                            </>
                                        )}
                                    </NavLink>
                                </li>
                            ))}
                        </ul>
                    </div>
                ))}
            </nav>

            {!collapsed && (
                <div className="mt-2 flex items-center justify-between rounded-2xl border border-white/5 bg-white/[0.02] px-3 py-2">
                    <div className="text-xs text-muted-foreground">Appearance</div>
                    <ThemeToggle />
                </div>
            )}

            <button
                data-testid={NAV.sidebarCollapseToggle}
                onClick={toggleCollapsed}
                aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
                className={cn(
                    "mt-2 flex items-center justify-center gap-2 rounded-xl border border-white/5 bg-white/[0.02] py-2 text-xs text-muted-foreground transition-colors hover:bg-white/[0.05] hover:text-foreground",
                    collapsed ? "px-2" : "px-3"
                )}
            >
                {collapsed ? <ChevronsRight className="h-4 w-4" /> : <ChevronsLeft className="h-4 w-4" />}
                {!collapsed && <span>Collapse</span>}
            </button>
        </aside>
    );
}
