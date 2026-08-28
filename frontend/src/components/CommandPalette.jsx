import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
    LayoutDashboard,
    Server,
    Boxes,
    Network,
    MonitorSmartphone,
    Database,
    ScrollText,
    BellRing,
    SunMoon,
    LogOut,
    Layers,
} from "lucide-react";
import {
    CommandDialog,
    CommandEmpty,
    CommandGroup,
    CommandInput,
    CommandItem,
    CommandList,
    CommandSeparator,
} from "@/components/ui/command";
import { useCluster } from "@/lib/cluster";
import { useTheme } from "@/lib/theme";
import { useAuth } from "@/lib/auth";
import { PALETTE } from "@/constants/testIds";

const PAGES = [
    { to: "/overview", label: "Overview", icon: LayoutDashboard },
    { to: "/nodes", label: "Nodes", icon: Server },
    { to: "/workloads", label: "Workloads", icon: Boxes },
    { to: "/networking", label: "Networking · Cilium", icon: Network },
    { to: "/vms", label: "Virtual Machines", icon: MonitorSmartphone },
    { to: "/storage", label: "Storage", icon: Database },
    { to: "/logs", label: "Pod Logs", icon: ScrollText },
    { to: "/events", label: "Events", icon: BellRing },
];

export default function CommandPalette() {
    const [open, setOpen] = useState(false);
    const navigate = useNavigate();
    const { clusters, setActiveId } = useCluster();
    const { theme, setTheme } = useTheme();
    const { signOut } = useAuth();

    useEffect(() => {
        const onKey = (e) => {
            if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
                e.preventDefault();
                setOpen((o) => !o);
            }
        };
        const onOpen = () => setOpen(true);
        window.addEventListener("keydown", onKey);
        window.addEventListener("axiom:palette", onOpen);
        return () => {
            window.removeEventListener("keydown", onKey);
            window.removeEventListener("axiom:palette", onOpen);
        };
    }, []);

    const run = (fn) => {
        setOpen(false);
        fn();
    };

    return (
        <CommandDialog open={open} onOpenChange={setOpen} data-testid={PALETTE.dialog}>
            <CommandInput data-testid={PALETTE.input} placeholder="Jump to a page, cluster, or action…" />
            <CommandList className="scrollbar-thin">
                <CommandEmpty>Nothing found.</CommandEmpty>
                <CommandGroup heading="Navigate">
                    {PAGES.map(({ to, label, icon: Icon }) => (
                        <CommandItem
                            key={to}
                            data-testid={PALETTE.item(to.slice(1))}
                            onSelect={() => run(() => navigate(to))}
                        >
                            <Icon className="mr-2 h-4 w-4 text-[#ff7550]" />
                            {label}
                        </CommandItem>
                    ))}
                </CommandGroup>
                <CommandSeparator />
                <CommandGroup heading="Switch cluster">
                    {clusters.map((c) => (
                        <CommandItem
                            key={c.id}
                            data-testid={PALETTE.item(`cluster-${c.id}`)}
                            onSelect={() => run(() => setActiveId(c.id))}
                        >
                            <Layers className="mr-2 h-4 w-4 text-[#7ab6dd]" />
                            {c.name}
                            <span className="ml-auto text-xs text-muted-foreground">{c.provider}</span>
                        </CommandItem>
                    ))}
                </CommandGroup>
                <CommandSeparator />
                <CommandGroup heading="Actions">
                    <CommandItem
                        data-testid={PALETTE.item("theme")}
                        onSelect={() => run(() => setTheme(theme === "dark" ? "light" : "dark"))}
                    >
                        <SunMoon className="mr-2 h-4 w-4 text-[#e0c996]" />
                        Toggle {theme === "dark" ? "light" : "dark"} mode
                    </CommandItem>
                    <CommandItem
                        data-testid={PALETTE.item("signout")}
                        onSelect={() =>
                            run(() => {
                                signOut();
                                navigate("/login", { replace: true });
                            })
                        }
                    >
                        <LogOut className="mr-2 h-4 w-4 text-[#ff7550]" />
                        Sign out
                    </CommandItem>
                </CommandGroup>
            </CommandList>
        </CommandDialog>
    );
}
