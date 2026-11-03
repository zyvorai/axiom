import { Link } from "react-router-dom";
import { Search, Command, Zap, PanelLeft, Sun, Moon, Monitor } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { NAV, NS_SELECT } from "@/constants/testIds";
import { useCluster } from "@/lib/cluster";
import { nextPreference, useTheme } from "@/lib/theme";
import { ALL_NAMESPACES, useNamespace } from "@/lib/namespace";
import NotificationInbox from "@/components/NotificationInbox";
import UserMenu from "@/components/UserMenu";
import ZyvorMark from "@/components/ZyvorMark";
import Breadcrumbs from "@/components/Breadcrumbs";

export default function TopBar({ onToggleSidebar }) {
    const { t } = useTranslation();
    const { preference, setTheme } = useTheme();
    const themeLabel = t(`nav:theme.${["light", "dark", "system"].includes(preference) ? preference : "dark"}`);
    const { active, clusters } = useCluster();
    const { namespace, namespaces, setNamespace } = useNamespace();
    const ThemeIcon = { light: Sun, dark: Moon, system: Monitor }[preference] || Moon;
    // keep the current choice selectable even if it is not (or no longer) in the list
    const nsOptions = [...new Set([...namespaces, ...(namespace !== ALL_NAMESPACES ? [namespace] : [])])].sort();
    const openPalette = () => window.dispatchEvent(new Event("axiom:palette"));

    return (
        <header className="bar">
            <button type="button" className="ic" onClick={onToggleSidebar} aria-label={t("nav:topbar.toggleSidebar")}>
                <PanelLeft size={17} />
            </button>

            <Link to="/overview" className="brand" aria-label={t("nav:topbar.home")}>
                <ZyvorMark size={22} idPrefix="axiom-topbar" className="brand-mark" />
                <span className="brand-name">
                    Axiom<span>{t("nav:topbar.byZyvor")}</span>
                </span>
            </Link>

            {active && (
                <Link
                    to={clusters.length > 1 ? "/fleet" : "/settings#clusters"}
                    className="cluster"
                    data-testid={NAV.clusterSwitcher}
                    data-state={active.state || "unknown"}
                    title={
                        active.state && active.state !== "healthy"
                            ? t(active.error ? "nav:topbar.clusterStateError" : "nav:topbar.clusterState", {
                                  name: active.label || active.name,
                                  state: active.state,
                                  error: active.error,
                              })
                            : undefined
                    }
                >
                    <i aria-hidden="true" />
                    {active.label || active.name}
                </Link>
            )}

            <label className="sr-only" htmlFor="topbar-namespace-select">
                {t("nav:topbar.namespace")}
            </label>
            <select
                id="topbar-namespace-select"
                data-testid={NS_SELECT.global}
                className="ns-switch"
                value={namespace}
                onChange={(e) => setNamespace(e.target.value)}
                title={t("nav:topbar.namespaceScope")}
            >
                <option value={ALL_NAMESPACES}>{t("nav:topbar.allNamespaces")}</option>
                {nsOptions.map((n) => (
                    <option key={n} value={n}>
                        {n}
                    </option>
                ))}
            </select>

            <Breadcrumbs />

            <div className="bar-r">
                <button type="button" data-testid={NAV.search} onClick={openPalette} className="search" title={t("nav:topbar.searchTitle")}>
                    <Search size={14} />
                    <span className="hidden sm:inline">{t("nav:topbar.search")}</span>
                    <kbd>
                        <Command size={10} className="inline align-[-1px]" />K
                    </kbd>
                </button>

                <button
                    type="button"
                    data-testid={NAV.connect}
                    className="btn btn-line h-[30px]"
                    onClick={() =>
                        toast.info(t("nav:topbar.sampleMode"), {
                            description: t("nav:topbar.sampleModeHint"),
                        })
                    }
                >
                    <Zap size={14} />
                    {t("nav:topbar.connect")}
                </button>

                <NotificationInbox />

                <button
                    type="button"
                    data-testid={NAV.themeToggle}
                    className="ic"
                    aria-label={t("nav:topbar.toggleTheme", { theme: themeLabel })}
                    title={t("nav:topbar.themeTitle", { theme: themeLabel })}
                    onClick={() => setTheme(nextPreference(preference))}
                >
                    <ThemeIcon size={16} aria-hidden="true" />
                </button>

                <UserMenu />
            </div>
        </header>
    );
}
