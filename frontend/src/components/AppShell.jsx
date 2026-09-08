import { useState } from "react";
import { Outlet } from "react-router-dom";
import "@/axiom-shell.css";
import Sidebar from "@/components/Sidebar";
import TopBar from "@/components/TopBar";
import CommandPalette from "@/components/CommandPalette";
import ViewerBanner from "@/components/ViewerBanner";
import { ConfirmProvider } from "@/components/ConfirmDialog";
import { loadSidebarCollapsed, saveSidebarCollapsed } from "@/lib/sidebar";

const MOBILE_BREAKPOINT = 900;

export default function AppShell() {
    const [collapsed, setCollapsed] = useState(loadSidebarCollapsed);
    const [mobileOpen, setMobileOpen] = useState(false);

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
                <CommandPalette />
                <TopBar onToggleSidebar={handleToggleSidebar} />
                <div className="frame">
                    <Sidebar
                        collapsed={collapsed}
                        onToggleCollapsed={toggleCollapsed}
                        mobileOpen={mobileOpen}
                        onCloseMobile={() => setMobileOpen(false)}
                    />
                    <main className="main">
                        <ViewerBanner />
                        <Outlet />
                    </main>
                </div>
            </div>
        </ConfirmProvider>
    );
}
