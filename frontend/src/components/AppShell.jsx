import { Suspense, useEffect, useRef, useState } from "react";
import { Outlet, useLocation } from "react-router-dom";
import "@/axiom-shell.css";
import Sidebar from "@/components/Sidebar";
import TopBar from "@/components/TopBar";
import CommandPalette from "@/components/CommandPalette";
import ViewerBanner from "@/components/ViewerBanner";
import { ConfirmProvider } from "@/components/ConfirmDialog";
import { Skeleton } from "@/components/ui/skeleton";
import ErrorBoundary from "@/components/ErrorBoundary";
import LoadErrorBanner from "@/components/LoadErrorBanner";
import { clearApiFailures } from "@/lib/apiHealth";
import { loadSidebarCollapsed, saveSidebarCollapsed } from "@/lib/sidebar";

const MOBILE_BREAKPOINT = 900;

export default function AppShell() {
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
                            <Outlet />
                        </Suspense>
                        </ErrorBoundary>
                    </main>
                </div>
            </div>
        </ConfirmProvider>
    );
}
