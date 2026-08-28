import { NavLink } from "react-router-dom";
import {
    LayoutDashboard,
    Server,
    Boxes,
    Component,
    Network,
    MonitorSmartphone,
    Database,
    ScrollText,
    BellRing,
    KeySquare,
    Archive,
    AlertTriangle,
    Layers,
    SlidersHorizontal,
    Cloudy,
    Waypoints,
    FolderTree,
    Sparkles,
    Rocket,
} from "lucide-react";
import ClusterSwitcher from "@/components/ClusterSwitcher";
import ThemeToggle from "@/components/ThemeToggle";
import { NAV } from "@/constants/testIds";
import { cn } from "@/lib/utils";

const ITEMS = [
    { to: "/getting-started", label: "Getting Started", icon: Rocket, key: "getting-started" },
    { to: "/overview", label: "Overview", icon: LayoutDashboard, key: "overview" },
    { to: "/nodes", label: "Nodes", icon: Server, key: "nodes" },
    { to: "/workloads", label: "Workloads", icon: Boxes, key: "workloads" },
    { to: "/pods", label: "Pods", icon: Component, key: "pods" },
    { to: "/platform-intents", label: "Platform Intents", icon: Sparkles, key: "platform-intents" },
    { to: "/networking", label: "Networking · Cilium", icon: Network, key: "networking" },
    { to: "/services", label: "Services", icon: Waypoints, key: "services" },
    { to: "/namespaces", label: "Namespaces", icon: FolderTree, key: "namespaces" },
    { to: "/vms", label: "Virtual Machines", icon: MonitorSmartphone, key: "vms" },
    { to: "/storage", label: "Storage", icon: Database, key: "storage" },
    { to: "/atlas", label: "Atlas", icon: Cloudy, key: "atlas" },
    { to: "/logs", label: "Pod Logs", icon: ScrollText, key: "logs" },
    { to: "/events", label: "Events", icon: BellRing, key: "events" },
    { to: "/secrets", label: "Secrets", icon: KeySquare, key: "secrets" },
    { to: "/backups", label: "Backups", icon: Archive, key: "backups" },
    { to: "/alerts", label: "Alerts", icon: AlertTriangle, key: "alerts" },
    { to: "/ops", label: "Ops", icon: Layers, key: "ops" },
    { to: "/settings", label: "Settings", icon: SlidersHorizontal, key: "settings" },
];

export default function Sidebar() {
    return (
        <aside
            data-testid={NAV.sidebar}
            className="glass specular sticky top-4 hidden h-[calc(100vh-2rem)] w-72 shrink-0 flex-col p-3 lg:flex"
        >
            <div className="flex items-center gap-2.5 px-2 pt-1 pb-3">
                <div className="relative grid h-9 w-9 place-items-center rounded-xl bg-gradient-to-br from-[#ff7550] via-[#f26b3a] to-[#c8452a] shadow-lg shadow-[#f26b3a]/30 ring-1 ring-white/10">
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
                <div className="leading-tight">
                    <div className="flex items-baseline gap-1.5">
                        <span className="text-[15px] font-semibold tracking-tight lowercase">zyvor</span>
                        <span className="text-[10px] uppercase tracking-[0.22em] text-[#ff7550]/90">Axiom</span>
                    </div>
                    <div className="text-[10px] uppercase tracking-[0.18em] text-muted-foreground">
                        Multi-cluster Console
                    </div>
                </div>
            </div>

            <ClusterSwitcher />

            <nav className="scrollbar-thin mt-4 flex-1 space-y-1 overflow-y-auto">
                {ITEMS.map(({ to, label, icon: Icon, key }) => (
                    <NavLink
                        key={to}
                        to={to}
                        data-testid={NAV.link(key)}
                        className={({ isActive }) =>
                            cn(
                                "group flex items-center gap-3 rounded-xl px-3 py-2 text-sm transition-all",
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
                                        "h-4 w-4 transition-colors",
                                        isActive ? "text-[#ff7550]" : "text-muted-foreground group-hover:text-foreground"
                                    )}
                                />
                                <span className="truncate">{label}</span>
                                {isActive && (
                                    <span className="ml-auto h-1.5 w-1.5 rounded-full bg-[#ff7550] shadow-[0_0_10px_#ff7550]" />
                                )}
                            </>
                        )}
                    </NavLink>
                ))}
            </nav>

            <div className="mt-2 flex items-center justify-between rounded-2xl border border-white/5 bg-white/[0.02] px-3 py-2">
                <div className="text-xs text-muted-foreground">Appearance</div>
                <ThemeToggle />
            </div>
        </aside>
    );
}
