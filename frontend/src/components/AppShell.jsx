import { Suspense, useEffect, useLayoutEffect, useRef, useState } from "react";
import { Outlet, useLocation } from "react-router-dom";
import "@/axiom-shell.css";
import Sidebar from "@/components/Sidebar";
import TopBar from "@/components/TopBar";
import CommandPalette from "@/components/CommandPalette";
import ShortcutsDialog from "@/components/ShortcutsDialog";
import { NamespaceProvider } from "@/lib/namespace";
import { useShortcuts } from "@/lib/shortcuts";
import ViewerBanner from "@/components/ViewerBanner";
import { ConfirmProvider } from "@/components/ConfirmDialog";
import { AiAssistantProvider } from "@/components/AiAssistantDrawer";
import { Skeleton } from "@/components/ui/skeleton";
import ErrorBoundary from "@/components/ErrorBoundary";
import MfaEnrollmentGate from "@/components/MfaEnrollmentGate";
import { useAuth } from "@/lib/auth";
import LoadErrorBanner from "@/components/LoadErrorBanner";
import { clearApiFailures } from "@/lib/apiHealth";
import { defaultTitle } from "@/lib/pageTitle";
import { NAV_ITEMS } from "@/constants/navItems";
import { loadSidebarCollapsed, saveSidebarCollapsed } from "@/lib/sidebar";

const MOBILE_BREAKPOINT = 900;

export default function AppShell() {
    return (
        <NamespaceProvider>
            <Shell />
        </NamespaceProvider>
    );
}

function Shell() {
    const { user } = useAuth();
    // AXIOM_REQUIRE_MFA covers this account but it has not enrolled: the API
    // refuses everything except enrolment, so show only the enrolment screen.
    const mfaGated = !!(user?.mfa?.required && !user.mfa.enrolled);
    useShortcuts();
    const [collapsed, setCollapsed] = useState(loadSidebarCollapsed);
    const [mobileOpen, setMobileOpen] = useState(false);
    const mainRef = useRef(null);
    const { pathname } = useLocation();
    const firstRender = useRef(true);
    const [retryKey, setRetryKey] = useState(0);
    const retry = () => {
        clearApiFailures();
        setRetryKey((k) => k + 1); // remounts the page, so it refetches
    };
    // Reset to a route-based title on navigation. A layout effect runs before every passive effect,
    // so a page's own usePageTitle still wins; this only covers states with no header
    // (loading, "not found", a deleted job) that would otherwise keep a stale/generic title.
    useLayoutEffect(() => {
        document.title = defaultTitle(pathname, NAV_ITEMS);
    }, [pathname]);
    // failures belong to the page that caused them
    useEffect(() => clearApiFailures(), [pathname]);

    // SPA navigation doesn't move focus, so keyboard/screen-reader users are left on the
    // sidebar link they activated. After a route change, move focus to <main> (not on the
    // initial load, and only when nothing inside the page already took it).
    useEffect(() => {
        if (firstRender.current) {
            firstRender.current = false;
            return;
        }
        const main = mainRef.current;
        if (main && !main.contains(document.activeElement)) main.focus({ preventScroll: true });
    }, [pathname]);

    const toggleCollapsed = () => {
        setCollapsed((v) => {
            saveSidebarCollapsed(!v);
            return !v;
        });
    };

    const handleToggleSidebar = () => {
        if (window.innerWidth < MOBILE_BREAKPOINT) {
            setMobileOpen((v) => !v);
        } else {
            toggleCollapsed();
        }
    };

    return (
        <ConfirmProvider>
            <AiAssistantProvider>
            <div className="ax">
                <a
                    href="#main-content"
                    className="skip-link"
                    onClick={(e) => {
                        e.preventDefault();
                        mainRef.current?.focus();
                    }}
                >
                    Skip to content
                </a>
                <CommandPalette />
                <ShortcutsDialog />
                <TopBar onToggleSidebar={handleToggleSidebar} />
                <div className="frame">
                    <Sidebar
                        collapsed={collapsed}
                        onToggleCollapsed={toggleCollapsed}
                        mobileOpen={mobileOpen}
                        onCloseMobile={() => setMobileOpen(false)}
                    />
                    <main className="main" id="main-content" tabIndex={-1} ref={mainRef}>
                        <ViewerBanner />
                        <LoadErrorBanner onRetry={retry} />
                        <ErrorBoundary resetKey={`${pathname}:${retryKey}`} onRetry={retry}>
                        <Suspense
                            key={retryKey}
                            fallback={
                                <div className="page" aria-busy="true">
                                    <Skeleton className="h-16 w-1/3" />
                                    <Skeleton className="mt-6 h-64 w-full rounded-3xl" />
                                </div>
                            }
                        >
                            {mfaGated ? <MfaEnrollmentGate /> : <Outlet />}
                        </Suspense>
                        </ErrorBoundary>
                    </main>
                </div>
            </div>
            </AiAssistantProvider>
        </ConfirmProvider>
    );
}
