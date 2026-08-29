import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { SunMoon, LogOut, Layers, Box, Boxes as BoxesIcon, FolderTree } from "lucide-react";
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
import { api } from "@/lib/api";
import { NAV_ITEMS, resolveIcon } from "@/constants/navItems";
import { PALETTE } from "@/constants/testIds";

export default function CommandPalette() {
    const [open, setOpen] = useState(false);
    const [pods, setPods] = useState([]);
    const [workloads, setWorkloads] = useState([]);
    const [namespaces, setNamespaces] = useState([]);
    const [plugins, setPlugins] = useState([]);
    const [loadedResources, setLoadedResources] = useState(false);
    const navigate = useNavigate();
    const { clusters, activate } = useCluster();
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

    useEffect(() => {
        if (!open || loadedResources) return;
        setLoadedResources(true);
        Promise.all([
            api.get("/pods").catch(() => ({ data: [] })),
            api.get("/workloads").catch(() => ({ data: [] })),
            api.get("/namespaces").catch(() => ({ data: [] })),
            api.get("/plugins").catch(() => ({ data: [] })),
        ]).then(([p, w, n, pl]) => {
            setPods(p.data || []);
            setWorkloads(w.data || []);
            setNamespaces(n.data || []);
            setPlugins(pl.data || []);
        });
    }, [open, loadedResources]);

    const navItems = [
        ...NAV_ITEMS,
        ...plugins.map((p) => ({ to: `/plugins/${p.name}`, label: p.displayName, icon: resolveIcon(p.icon) })),
    ];

    const run = (fn) => {
        setOpen(false);
        fn();
    };

    return (
        <CommandDialog open={open} onOpenChange={setOpen} data-testid={PALETTE.dialog}>
            <CommandInput data-testid={PALETTE.input} placeholder="Jump to a page, pod, workload, namespace, cluster, or action…" />
            <CommandList className="scrollbar-thin">
                <CommandEmpty>Nothing found.</CommandEmpty>
                <CommandGroup heading="Navigate">
                    {navItems.map(({ to, label, icon: Icon }) => (
                        <CommandItem
                            key={to}
                            value={label}
                            data-testid={PALETTE.item(to.slice(1))}
                            onSelect={() => run(() => navigate(to))}
                        >
                            <Icon className="mr-2 h-4 w-4 text-[#ff7550]" />
                            {label}
                        </CommandItem>
                    ))}
                </CommandGroup>
                {pods.length > 0 && (
                    <>
                        <CommandSeparator />
                        <CommandGroup heading="Pods">
                            {pods.map((p) => (
                                <CommandItem
                                    key={`${p.namespace}/${p.name}`}
                                    value={`pod ${p.name} ${p.namespace}`}
                                    onSelect={() => run(() => navigate(`/pods/${p.namespace}/${encodeURIComponent(p.name)}`))}
                                >
                                    <Box className="mr-2 h-4 w-4 text-[#93c88f]" />
                                    {p.name}
                                    <span className="ml-auto text-xs text-muted-foreground">{p.namespace}</span>
                                </CommandItem>
                            ))}
                        </CommandGroup>
                    </>
                )}
                {workloads.length > 0 && (
                    <>
                        <CommandSeparator />
                        <CommandGroup heading="Workloads">
                            {workloads.map((w) => (
                                <CommandItem
                                    key={`${w.namespace}/${w.kind}/${w.name}`}
                                    value={`workload ${w.name} ${w.namespace} ${w.kind}`}
                                    onSelect={() => run(() => navigate(`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}`))}
                                >
                                    <BoxesIcon className="mr-2 h-4 w-4 text-[#e0c996]" />
                                    {w.name}
                                    <span className="ml-auto text-xs text-muted-foreground">
                                        {w.namespace} · {w.kind}
                                    </span>
                                </CommandItem>
                            ))}
                        </CommandGroup>
                    </>
                )}
                {namespaces.length > 0 && (
                    <>
                        <CommandSeparator />
                        <CommandGroup heading="Namespaces">
                            {namespaces.map((n) => (
                                <CommandItem
                                    key={n.name}
                                    value={`namespace ${n.name}`}
                                    onSelect={() => run(() => navigate("/namespaces"))}
                                >
                                    <FolderTree className="mr-2 h-4 w-4 text-[#6ab6f0]" />
                                    {n.name}
                                </CommandItem>
                            ))}
                        </CommandGroup>
                    </>
                )}
                <CommandSeparator />
                <CommandGroup heading="Switch cluster">
                    {clusters.map((c) => (
                        <CommandItem
                            key={c.name}
                            value={`cluster ${c.label || c.name}`}
                            data-testid={PALETTE.item(`cluster-${c.name}`)}
                            onSelect={() => run(() => activate(c.name))}
                        >
                            <Layers className="mr-2 h-4 w-4 text-[#7ab6dd]" />
                            {c.label || c.name}
                            <span className="ml-auto text-xs text-muted-foreground">{c.current ? "current" : ""}</span>
                        </CommandItem>
                    ))}
                </CommandGroup>
                <CommandSeparator />
                <CommandGroup heading="Actions">
                    <CommandItem
                        value="toggle theme"
                        data-testid={PALETTE.item("theme")}
                        onSelect={() => run(() => setTheme(theme === "dark" ? "light" : "dark"))}
                    >
                        <SunMoon className="mr-2 h-4 w-4 text-[#e0c996]" />
                        Toggle {theme === "dark" ? "light" : "dark"} mode
                    </CommandItem>
                    <CommandItem
                        value="sign out"
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
