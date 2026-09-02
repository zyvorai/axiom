import { Outlet, useLocation } from "react-router-dom";
import AmbientBackdrop from "@/components/AmbientBackdrop";
import Sidebar from "@/components/Sidebar";
import TopBar from "@/components/TopBar";
import Breadcrumbs from "@/components/Breadcrumbs";
import CommandPalette from "@/components/CommandPalette";
import ViewerBanner from "@/components/ViewerBanner";
import { ConfirmProvider } from "@/components/ConfirmDialog";

// Mirrors zeus-os's WebShell.tsx shape: a full-width sticky header spanning
// the top, with a flex row of [sidebar, content] below it — replacing the
// old layout where everything (including the header) lived inside one
// centered max-width row.
export default function AppShell() {
    const { pathname } = useLocation();
    // Overview is the product home — hide breadcrumbs so the first viewport
    // reads as an apple.com-style marketing band rather than an ops trail.
    const showBreadcrumbs = pathname !== "/overview" && pathname !== "/";

    return (
        <ConfirmProvider>
            <div className="relative flex h-screen min-h-screen flex-col overflow-hidden bg-background">
                <AmbientBackdrop />
                <CommandPalette />
                <TopBar />
                <div className="flex min-h-0 flex-1 gap-0 px-0 pt-0 lg:gap-0">
                    <Sidebar />
                    <main className="scrollbar-thin min-w-0 flex-1 overflow-y-auto px-4 py-4 lg:px-6">
                        <div className="mx-auto w-full max-w-[1600px] space-y-4">
                            <ViewerBanner />
                            {showBreadcrumbs && <Breadcrumbs />}
                            <Outlet />
                        </div>
                    </main>
                </div>
            </div>
        </ConfirmProvider>
    );
}
