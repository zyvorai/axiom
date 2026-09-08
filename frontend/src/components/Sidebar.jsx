import { useEffect, useState } from "react";
import { NavLink } from "react-router-dom";
import { ChevronDown, ChevronsLeft, ChevronsRight } from "lucide-react";
import ClusterSwitcher from "@/components/ClusterSwitcher";
import { NAV } from "@/constants/testIds";
import { NAV_ITEMS, NAV_GROUP_ORDER, RAIL_PINNED_KEYS, resolveIcon } from "@/constants/navItems";
import { api } from "@/lib/api";
import { useNavCounts } from "@/lib/navCounts";

const COUNT_KEYS = { workloads: "workloads", pods: "pods", alerts: "alerts" };

export default function Sidebar({ collapsed, onToggleCollapsed, mobileOpen, onCloseMobile }) {
    const [plugins, setPlugins] = useState([]);
    const [closedGroups, setClosedGroups] = useState({});
    const counts = useNavCounts();

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

    const countFor = (key) => {
        const k = COUNT_KEYS[key];
        return k ? counts[k] : undefined;
    };

    const toggleGroup = (label) => setClosedGroups((c) => ({ ...c, [label]: !c[label] }));

    return (
        <>
            {mobileOpen && <div className="side-backdrop" onClick={onCloseMobile} aria-hidden="true" />}
            <aside data-testid={NAV.sidebar} className={`side ${collapsed ? "min" : ""} ${mobileOpen ? "open" : ""}`}>
                {!collapsed && (
                    <div className="mb-3 border-b border-[var(--line)] pb-3">
                        <ClusterSwitcher />
                    </div>
                )}
                <div className="side-scroll">
                    {collapsed
                        ? railItems.map(({ to, label, icon: Icon, key }) => (
                              <NavLink
                                  key={to}
                                  to={to}
                                  data-testid={NAV.link(key)}
                                  className="item"
                                  onClick={onCloseMobile}
                              >
                                  <Icon size={16} strokeWidth={1.8} />
                                  <span className="tip">{label}</span>
                              </NavLink>
                          ))
                        : groups.map((group) => (
                              <div key={group.label} className={`grp ${closedGroups[group.label] ? "closed" : ""}`}>
                                  <button type="button" className="grp-h" onClick={() => toggleGroup(group.label)}>
                                      <ChevronDown size={11} />
                                      {group.label}
                                  </button>
                                  {group.items.map(({ to, label, sub, icon: Icon, key }) => {
                                      const cnt = countFor(key);
                                      return (
                                          <NavLink
                                              key={to}
                                              to={to}
                                              data-testid={NAV.link(key)}
                                              className="item"
                                              onClick={onCloseMobile}
                                          >
                                              <Icon size={16} strokeWidth={1.8} />
                                              <span className="lbl">
                                                  {label}
                                                  {sub && <span className="sub"> · {sub}</span>}
                                              </span>
                                              {cnt != null && (
                                                  <span className={`cnt ${key === "alerts" && cnt > 0 ? "warn" : ""}`}>{cnt}</span>
                                              )}
                                          </NavLink>
                                      );
                                  })}
                              </div>
                          ))}
                </div>
                <div className="side-foot">
                    <button
                        type="button"
                        data-testid={NAV.sidebarCollapseToggle}
                        className="item"
                        onClick={onToggleCollapsed}
                        aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
                    >
                        {collapsed ? <ChevronsRight size={16} /> : <ChevronsLeft size={16} />}
                        <span className="lbl">Collapse</span>
                    </button>
                </div>
            </aside>
        </>
    );
}
