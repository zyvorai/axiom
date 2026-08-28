import { Outlet } from "react-router-dom";
import AmbientBackdrop from "@/components/AmbientBackdrop";
import Sidebar from "@/components/Sidebar";
import TopBar from "@/components/TopBar";
import CommandPalette from "@/components/CommandPalette";

export default function AppShell() {
    return (
        <div className="relative min-h-screen">
            <AmbientBackdrop />
            <CommandPalette />
            <div className="mx-auto flex max-w-[1600px] gap-4 p-4">
                <Sidebar />
                <main className="min-w-0 flex-1 space-y-4">
                    <TopBar />
                    <div className="scrollbar-thin">
                        <Outlet />
                    </div>
                </main>
            </div>
        </div>
    );
}
