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
    FileCog,
    Gauge,
    CalendarClock,
    Users as UsersIcon,
    TrendingUp,
    Layers3,
    Snowflake,
    Puzzle,
    History,
    Stethoscope,
    ShieldAlert,
} from "lucide-react";

// Fixed name -> lucide-react component lookup for plugin-declared icons.
// Deliberately not a dynamic import: a plugin manifest can only select
// from this list, never cause arbitrary code to load.
const PLUGIN_ICON_MAP = {
    Snowflake,
    Puzzle,
    Layers3,
    Boxes,
    Database,
    Archive,
};

export function resolveIcon(name) {
    return PLUGIN_ICON_MAP[name] || Puzzle;
}

// Single source of truth for every top-level route, shared by the Sidebar
// and the command palette so the two can't drift out of sync again.
// `group` mirrors zeus-os's sidebar grouping convention (Core/Cluster/
// Observe/Run/Security/Cost/More), mapped onto Axiom's own domain.
export const NAV_ITEMS = [
    { to: "/getting-started", label: "Getting Started", icon: Rocket, key: "getting-started", group: "Core" },
    { to: "/overview", label: "Overview", icon: LayoutDashboard, key: "overview", group: "Core" },
    { to: "/nodes", label: "Nodes", icon: Server, key: "nodes", group: "Cluster" },
    { to: "/workloads", label: "Workloads", icon: Boxes, key: "workloads", group: "Cluster" },
    { to: "/jobs", label: "Jobs & CronJobs", icon: CalendarClock, key: "jobs", group: "Cluster" },
    { to: "/pods", label: "Pods", icon: Component, key: "pods", group: "Cluster" },
    { to: "/namespaces", label: "Namespaces", icon: FolderTree, key: "namespaces", group: "Cluster" },
    { to: "/platform-intents", label: "Platform Intents", icon: Sparkles, key: "platform-intents", group: "Cluster" },
    { to: "/networking", label: "Networking · Cilium", icon: Network, key: "networking", group: "Network" },
    { to: "/services", label: "Services", icon: Waypoints, key: "services", group: "Network" },
    { to: "/ingresses", label: "Ingresses", icon: Waypoints, key: "ingresses", group: "Network" },
    { to: "/vms", label: "Virtual Machines", icon: MonitorSmartphone, key: "vms", group: "Storage" },
    { to: "/storage", label: "Storage", icon: Database, key: "storage", group: "Storage" },
    { to: "/atlas", label: "Atlas", icon: Cloudy, key: "atlas", group: "Storage" },
    { to: "/backups", label: "Backups", icon: Archive, key: "backups", group: "Storage" },
    { to: "/configmaps", label: "ConfigMaps", icon: FileCog, key: "configmaps", group: "Config" },
    { to: "/secrets", label: "Secrets", icon: KeySquare, key: "secrets", group: "Config" },
    { to: "/quotas", label: "Quotas & Limits", icon: Gauge, key: "quotas", group: "Config" },
    { to: "/autoscaling", label: "Autoscaling", icon: TrendingUp, key: "autoscaling", group: "Config" },
    { to: "/custom-resources", label: "Custom Resources", icon: Layers3, key: "custom-resources", group: "Config" },
    { to: "/logs", label: "Pod Logs", icon: ScrollText, key: "logs", group: "Observe" },
    { to: "/events", label: "Events", icon: BellRing, key: "events", group: "Observe" },
    { to: "/timeline", label: "Timeline", icon: History, key: "timeline", group: "Observe" },
    { to: "/health-check", label: "Health check", icon: Stethoscope, key: "health-check", group: "Observe" },
    { to: "/alerts", label: "Alerts", icon: AlertTriangle, key: "alerts", group: "Observe" },
    { to: "/ops", label: "Ops", icon: Layers, key: "ops", group: "Observe" },
    { to: "/security", label: "Security", icon: ShieldAlert, key: "security", group: "Observe" },
    { to: "/users", label: "Users", icon: UsersIcon, key: "users", group: "Admin" },
    { to: "/settings", label: "Settings", icon: SlidersHorizontal, key: "settings", group: "Admin" },
];

// Render order for the grouped sidebar; any item (e.g. a plugin) without
// a matching group falls into "Extensions" at the end.
export const NAV_GROUP_ORDER = ["Core", "Cluster", "Network", "Storage", "Config", "Observe", "Admin", "Extensions"];

// Curated set for the collapsed "rail" sidebar — mirrors zeus-os's own rail
// (RAIL_PINNED_ITEMS in its navigation config): a handful of top-level
// destinations, one per domain, rendered as a plain flat icon list with no
// group headers or dividers — instead of cramming every nav item into
// icon-only form under 8 divided groups.
export const RAIL_PINNED_KEYS = ["overview", "workloads", "networking", "storage", "ops", "settings"];
