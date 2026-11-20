import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
    SunMoon,
    Keyboard,
    LogOut,
    Layers,
    Box,
    Boxes as BoxesIcon,
    FolderTree,
    FileCog,
    KeySquare,
    Network,
    Monitor,
    CalendarClock,
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
import { SHORTCUTS_EVENT } from "@/lib/shortcuts";
import { nextPreference, useTheme } from "@/lib/theme";
import { useAuth } from "@/lib/auth";
import { api } from "@/lib/api";
import { NAV_ITEMS, resolveIcon, navLabel } from "@/constants/navItems";
import { PALETTE } from "@/constants/testIds";

export default function CommandPalette() {
    const [open, setOpen] = useState(false);
    const [pods, setPods] = useState([]);
    const [workloads, setWorkloads] = useState([]);
    const [namespaces, setNamespaces] = useState([]);
    const [configmaps, setConfigmaps] = useState([]);
    const [secrets, setSecrets] = useState([]);
    const [services, setServices] = useState([]);
    const [vms, setVms] = useState([]);
    const [jobs, setJobs] = useState([]);
    const [cronJobs, setCronJobs] = useState([]);
    const [plugins, setPlugins] = useState([]);
    const [loadedResources, setLoadedResources] = useState(false);
    const navigate = useNavigate();
    const { t } = useTranslation();
    const { clusters, activate } = useCluster();
    const { preference, setTheme } = useTheme();
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
            api.get("/configmaps").catch(() => ({ data: [] })),
            api.get("/secrets").catch(() => ({ data: [] })),
            api.get("/services").catch(() => ({ data: [] })),
            api.get("/vms").catch(() => ({ data: [] })),
            api.get("/jobs").catch(() => ({ data: [] })),
            api.get("/cronjobs").catch(() => ({ data: [] })),
            api.get("/plugins").catch(() => ({ data: [] })),
        ]).then(([p, w, n, cm, sec, svc, vm, j, cj, pl]) => {
            setPods(p.data || []);
            setWorkloads(w.data || []);
            setNamespaces(n.data || []);
            setConfigmaps((cm.data || []).slice(0, 80));
            setSecrets((sec.data || []).slice(0, 80));
            setServices((svc.data || []).slice(0, 80));
            setVms(vm.data || []);
            setJobs((j.data || []).slice(0, 80));
            setCronJobs(cj.data || []);
            setPlugins(pl.data || []);
        });
    }, [open, loadedResources]);

    const navItems = [
        ...NAV_ITEMS.map((n) => ({ ...n, label: navLabel(t, n), english: n.label })),
        ...plugins.map((p) => ({ to: `/plugins/${p.name}`, label: p.displayName, icon: resolveIcon(p.icon) })),
    ];

    const run = (fn) => {
        setOpen(false);
        fn();
    };

    return (
        <CommandDialog open={open} onOpenChange={setOpen} data-testid={PALETTE.dialog}>
            <CommandInput data-testid={PALETTE.input} placeholder={t("nav:palette.placeholder")} />
            <CommandList className="scrollbar-thin">
                <CommandEmpty>{t("nav:palette.empty")}</CommandEmpty>
                <CommandGroup heading={t("nav:palette.navigate")}>
                    {navItems.map(({ to, label, english, icon: Icon }) => (
                        <CommandItem
                            key={to}
                            value={label}
                            keywords={english && english !== label ? [english] : undefined}
                            data-testid={PALETTE.item(to.slice(1))}
                            onSelect={() => run(() => navigate(to))}
                        >
                            <Icon className="mr-2 h-4 w-4 text-tone-link" />
                            {label}
                        </CommandItem>
                    ))}
                </CommandGroup>
                {pods.length > 0 && (
                    <>
                        <CommandSeparator />
                        <CommandGroup heading={t("nav:palette.pods")}>
                            {pods.map((p) => (
                                <CommandItem
                                    key={`${p.namespace}/${p.name}`}
                                    value={`pod ${p.name} ${p.namespace}`}
                                    onSelect={() => run(() => navigate(`/pods/${p.namespace}/${encodeURIComponent(p.name)}`))}
                                >
                                    <Box className="mr-2 h-4 w-4 text-tone-green" />
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
                        <CommandGroup heading={t("nav:palette.workloads")}>
                            {workloads.map((w) => (
                                <CommandItem
                                    key={`${w.namespace}/${w.kind}/${w.name}`}
                                    value={`workload ${w.name} ${w.namespace} ${w.kind}`}
                                    onSelect={() => run(() => navigate(`/workloads/${w.namespace}/${w.kind}/${encodeURIComponent(w.name)}`))}
                                >
                                    <BoxesIcon className="mr-2 h-4 w-4 text-tone-amber" />
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
                        <CommandGroup heading={t("nav:palette.namespaces")}>
                            {namespaces.map((n) => (
                                <CommandItem
                                    key={n.name}
                                    value={`namespace ${n.name}`}
                                    onSelect={() => run(() => navigate(`/namespaces/${encodeURIComponent(n.name)}`))}
                                >
                                    <FolderTree className="mr-2 h-4 w-4 text-tone-link" />
                                    {n.name}
                                    <span className="ml-auto text-xs text-muted-foreground">{t("nav:palette.nsPods", { count: n.pods ?? 0 })}</span>
                                </CommandItem>
                            ))}
                        </CommandGroup>
                    </>
                )}
                {services.length > 0 && (
                    <>
                        <CommandSeparator />
                        <CommandGroup heading={t("nav:palette.services")}>
                            {services.map((s) => (
                                <CommandItem
                                    key={`${s.namespace}/${s.name}`}
                                    value={`service ${s.name} ${s.namespace}`}
                                    onSelect={() => run(() => navigate(`/services/${encodeURIComponent(s.namespace)}/${encodeURIComponent(s.name)}`))}
                                >
                                    <Network className="mr-2 h-4 w-4 text-tone-link" />
                                    {s.name}
                                    <span className="ml-auto text-xs text-muted-foreground">{s.namespace}</span>
                                </CommandItem>
                            ))}
                        </CommandGroup>
                    </>
                )}
                {configmaps.length > 0 && (
                    <>
                        <CommandSeparator />
                        <CommandGroup heading={t("nav:palette.configmaps")}>
                            {configmaps.map((c) => (
                                <CommandItem
                                    key={`${c.namespace}/${c.name}`}
                                    value={`configmap ${c.name} ${c.namespace}`}
                                    onSelect={() => run(() => navigate(`/configmaps/${encodeURIComponent(c.namespace)}/${encodeURIComponent(c.name)}`))}
                                >
                                    <FileCog className="mr-2 h-4 w-4 text-tone-link" />
                                    {c.name}
                                    <span className="ml-auto text-xs text-muted-foreground">{c.namespace}</span>
                                </CommandItem>
                            ))}
                        </CommandGroup>
                    </>
                )}
                {secrets.length > 0 && (
                    <>
                        <CommandSeparator />
                        <CommandGroup heading={t("nav:palette.secrets")}>
                            {secrets.map((s) => (
                                <CommandItem
                                    key={`${s.namespace}/${s.name}`}
                                    value={`secret ${s.name} ${s.namespace}`}
                                    onSelect={() => run(() => navigate(`/secrets/${encodeURIComponent(s.namespace)}/${encodeURIComponent(s.name)}`))}
                                >
                                    <KeySquare className="mr-2 h-4 w-4 text-tone-purple" />
                                    {s.name}
                                    <span className="ml-auto text-xs text-muted-foreground">{s.namespace}</span>
                                </CommandItem>
                            ))}
                        </CommandGroup>
                    </>
                )}
                {vms.length > 0 && (
                    <>
                        <CommandSeparator />
                        <CommandGroup heading={t("nav:palette.vms")}>
                            {vms.map((v) => (
                                <CommandItem
                                    key={`${v.namespace}/${v.name}`}
                                    value={`vm ${v.name} ${v.namespace}`}
                                    onSelect={() => run(() => navigate(`/vms/${encodeURIComponent(v.namespace)}/${encodeURIComponent(v.name)}`))}
                                >
                                    <Monitor className="mr-2 h-4 w-4 text-tone-amber" />
                                    {v.name}
                                    <span className="ml-auto text-xs text-muted-foreground">{v.namespace}</span>
                                </CommandItem>
                            ))}
                        </CommandGroup>
                    </>
                )}
                {(jobs.length > 0 || cronJobs.length > 0) && (
                    <>
                        <CommandSeparator />
                        <CommandGroup heading={t("nav:palette.jobs")}>
                            {cronJobs.map((c) => (
                                <CommandItem
                                    key={`cron/${c.namespace}/${c.name}`}
                                    value={`cronjob ${c.name} ${c.namespace}`}
                                    onSelect={() => run(() => navigate(`/cronjobs/${encodeURIComponent(c.namespace)}/${encodeURIComponent(c.name)}`))}
                                >
                                    <CalendarClock className="mr-2 h-4 w-4 text-tone-link" />
                                    {c.name}
                                    <span className="ml-auto text-xs text-muted-foreground">{t("nav:palette.cron", { namespace: c.namespace })}</span>
                                </CommandItem>
                            ))}
                            {jobs.map((j) => (
                                <CommandItem
                                    key={`job/${j.namespace}/${j.name}`}
                                    value={`job ${j.name} ${j.namespace}`}
                                    onSelect={() => run(() => navigate(`/jobs/${encodeURIComponent(j.namespace)}/${encodeURIComponent(j.name)}`))}
                                >
                                    <BoxesIcon className="mr-2 h-4 w-4 text-tone-green" />
                                    {j.name}
                                    <span className="ml-auto text-xs text-muted-foreground">{j.namespace}</span>
                                </CommandItem>
                            ))}
                        </CommandGroup>
                    </>
                )}
                <CommandSeparator />
                <CommandGroup heading={t("nav:palette.switchCluster")}>
                    {clusters.map((c) => (
                        <CommandItem
                            key={c.name}
                            value={`cluster ${c.label || c.name}`}
                            data-testid={PALETTE.item(`cluster-${c.name}`)}
                            onSelect={() => run(() => activate(c.name))}
                        >
                            <Layers className="mr-2 h-4 w-4 text-tone-link" />
                            {c.label || c.name}
                            <span className="ml-auto text-xs text-muted-foreground">{c.current ? t("nav:palette.current") : ""}</span>
                        </CommandItem>
                    ))}
                </CommandGroup>
                <CommandSeparator />
                <CommandGroup heading={t("nav:palette.actions")}>
                    <CommandItem
                        value="toggle theme"
                        data-testid={PALETTE.item("theme")}
                        onSelect={() => run(() => setTheme(nextPreference(preference)))}
                    >
                        <SunMoon className="mr-2 h-4 w-4 text-tone-amber" />
                        {t(`nav:palette.switchTheme.${nextPreference(preference)}`)}
                    </CommandItem>
                    <CommandItem
                        value="keyboard shortcuts help"
                        data-testid={PALETTE.item("shortcuts")}
                        onSelect={() => run(() => window.dispatchEvent(new Event(SHORTCUTS_EVENT)))}
                    >
                        <Keyboard className="mr-2 h-4 w-4 text-tone-link" />
                        {t("nav:palette.shortcuts")}
                        <span className="ml-auto text-xs text-muted-foreground">?</span>
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
                        <LogOut className="mr-2 h-4 w-4 text-tone-link" />
                        {t("nav:palette.signOut")}
                    </CommandItem>
                </CommandGroup>
            </CommandList>
        </CommandDialog>
    );
}
