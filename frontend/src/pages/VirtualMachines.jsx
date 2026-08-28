import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { MonitorPlay, Cpu, MemoryStick, X } from "lucide-react";
import { VMS } from "@/constants/testIds";

/** noVNC-style animated console emulator. Draws a Linux boot / tty on canvas. */
function VncCanvas({ vm }) {
    const canvasRef = useRef(null);
    const rafRef = useRef();

    useEffect(() => {
        const cv = canvasRef.current;
        if (!cv) return;
        const ctx = cv.getContext("2d");
        const dpr = window.devicePixelRatio || 1;
        const resize = () => {
            const rect = cv.getBoundingClientRect();
            cv.width = rect.width * dpr;
            cv.height = rect.height * dpr;
            ctx.scale(dpr, dpr);
        };
        resize();

        const linesWin = [
            "Starting Windows Server 2022...",
            "[  OK  ] Loaded kernel modules",
            "[  OK  ] Mounted C:\\ (NTFS)",
            "[  OK  ] Reached target Network",
            "[  OK  ] Started Task Scheduler",
            "PS C:\\Users\\Administrator>  Get-Service | Where Status -eq 'Running' | Select -First 5",
            "Status   Name               DisplayName",
            "------   ----               -----------",
            "Running  BITS                Background Intelligent Transfer",
            "Running  Dnscache            DNS Client",
            "Running  EventLog            Windows Event Log",
            "Running  LanmanServer        Server",
            "PS C:\\Users\\Administrator>  _",
        ];
        const linesLinux = [
            "[  OK  ] Started KubeVirt virt-launcher",
            "[  OK  ] Reached target Multi-User System",
            "Ubuntu 22.04.4 LTS  ttyS0",
            "",
            `${vm.name} login: root`,
            "Password: ****",
            "Welcome to Ubuntu 22.04.4 LTS (GNU/Linux 6.5.0-1023 x86_64)",
            "",
            " * Documentation:  https://help.ubuntu.com",
            " * Management:     https://landscape.canonical.com",
            "",
            "  System information as of " + new Date().toUTCString(),
            "",
            "  System load:  0.42               Users logged in:       1",
            "  Usage of /:   " + (Math.random() * 40 + 20).toFixed(1) + "% of 40.00GB",
            "  Memory usage: " + Math.round(Math.random() * 30 + 40) + "%",
            "  Swap usage:   0%",
            "",
            "Last login: " + new Date().toString().slice(0, 24),
            `root@${vm.name}:~# systemctl status kubelet`,
            "● kubelet.service - kubernetes Kubelet",
            "     Loaded: loaded (/etc/systemd/system/kubelet.service; enabled)",
            "     Active: active (running) since " + new Date().toDateString(),
            "   Main PID: 1834 (kubelet)",
            `root@${vm.name}:~# _`,
        ];
        const linesOpn = [
            "OPNsense 24.1 boot sequence",
            "[  OK  ] Interfaces wan0, lan0 up",
            "[  OK  ] pf.ko loaded",
            "[  OK  ] Firewall rules applied: 342",
            "[  OK  ] IPsec tunnel eu-west-1 established",
            "[  OK  ] DHCPv6 leases: 128",
            `opnsense# ipfw show | wc -l  →  342`,
            `opnsense# _`,
        ];
        const os = (vm.os || "").toLowerCase();
        const lines = os.startsWith("windows") ? linesWin : os.startsWith("opnsense") ? linesOpn : linesLinux;

        let shown = 0;
        let charIdx = 0;
        let blink = 0;
        let last = 0;

        const draw = (t) => {
            const rect = cv.getBoundingClientRect();
            ctx.fillStyle = "#050506";
            ctx.fillRect(0, 0, rect.width, rect.height);
            // scanlines
            ctx.fillStyle = "rgba(255,255,255,0.02)";
            for (let y = 0; y < rect.height; y += 3) ctx.fillRect(0, y, rect.width, 1);

            ctx.font = "13px 'SF Mono', Menlo, monospace";
            ctx.fillStyle = "#7FE7A6";
            let y = 20;
            for (let i = 0; i < shown && i < lines.length; i++) {
                const full = lines[i];
                const text = i === shown - 1 ? full.slice(0, charIdx) : full;
                if (text.startsWith("[  OK  ]")) ctx.fillStyle = "#7FE7A6";
                else if (text.toLowerCase().includes("error") || text.toLowerCase().includes("fail")) ctx.fillStyle = "#FF6B6B";
                else if (text.startsWith("PS ") || text.startsWith("opnsense#") || text.startsWith("root@")) ctx.fillStyle = "#8AB4FF";
                else ctx.fillStyle = "#D6D6D6";
                ctx.fillText(text, 16, y);
                y += 18;
            }
            // cursor blink
            if (shown >= lines.length && Math.floor(blink) % 2 === 0) {
                const lastLine = lines[lines.length - 1];
                const width = ctx.measureText(lastLine.replace(/_$/, "")).width;
                ctx.fillStyle = "#8AB4FF";
                ctx.fillRect(16 + width, y - 30, 8, 14);
            }

            if (t - last > 22) {
                charIdx += 2;
                last = t;
            }
            if (shown < lines.length && charIdx >= lines[shown].length) {
                shown++;
                charIdx = 0;
            }
            blink += 0.05;
            rafRef.current = requestAnimationFrame(draw);
        };

        rafRef.current = requestAnimationFrame(draw);
        return () => cancelAnimationFrame(rafRef.current);
    }, [vm]);

    return (
        <canvas
            ref={canvasRef}
            data-testid={VMS.consoleCanvas}
            className="h-[70vh] w-full rounded-xl border border-white/10 bg-black"
        />
    );
}

export default function VirtualMachines() {
    const [vms, setVms] = useState(null);
    const [openVm, setOpenVm] = useState(null);

    useEffect(() => {
        let alive = true;
        const load = () => api.get("/vms").then((r) => alive && setVms(r.data));
        load();
        const t = setInterval(load, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    if (!vms) {
        return (
            <div data-testid={VMS.root} className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {[...Array(6)].map((_, i) => (
                    <Skeleton key={i} className="h-64 rounded-2xl bg-white/[0.04]" />
                ))}
            </div>
        );
    }

    return (
        <div data-testid={VMS.root} className="space-y-4">
            <div className="flex items-end justify-between">
                <div>
                    <h1 className="text-3xl font-semibold tracking-tight">Virtual Machines</h1>
                    <p className="text-sm text-muted-foreground">
                        KubeVirt-managed VMs with live-migration and noVNC console access.
                    </p>
                </div>
                <div className="text-xs text-muted-foreground">{vms.length} VMs</div>
            </div>

            <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                {vms.map((vm) => (
                    <GlassCard key={`${vm.namespace}/${vm.name}`} data-testid={VMS.card(vm.name)} className="p-5">
                        <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                                <div className="mono truncate text-sm">{vm.name}</div>
                                <div className="mt-1 text-xs text-muted-foreground">{vm.os}</div>
                            </div>
                            <StatusBadge status={vm.status} />
                        </div>

                        <div className="mt-4 grid grid-cols-2 gap-3 text-xs">
                            <div className="rounded-lg border border-white/5 bg-white/[0.02] p-2.5">
                                <div className="flex items-center gap-1 text-[10px] uppercase tracking-widest text-muted-foreground">
                                    <Cpu className="h-3 w-3" /> vCPU
                                </div>
                                <div className="mono mt-1 text-sm">{vm.cpu}</div>
                            </div>
                            <div className="rounded-lg border border-white/5 bg-white/[0.02] p-2.5">
                                <div className="flex items-center gap-1 text-[10px] uppercase tracking-widest text-muted-foreground">
                                    <MemoryStick className="h-3 w-3" /> RAM
                                </div>
                                <div className="mono mt-1 text-sm">{vm.memory}</div>
                            </div>
                        </div>

                        <div className="mt-3 flex items-center gap-2 text-xs text-muted-foreground">
                            <span className="mono">{vm.node}</span>
                            <span className="ml-auto">{vm.age}</span>
                        </div>

                        <div className="mt-4">
                            <Button
                                data-testid={VMS.consoleBtn(vm.name)}
                                onClick={() => setOpenVm(vm)}
                                className="w-full rounded-xl bg-gradient-to-r from-[#ff7550] via-[#f26b3a] to-[#6ab6f0] text-white shadow-lg shadow-[#f26b3a]/25 hover:brightness-110"
                            >
                                <MonitorPlay className="mr-2 h-4 w-4" />
                                Open console (noVNC)
                            </Button>
                        </div>
                    </GlassCard>
                ))}
            </div>

            <Dialog open={!!openVm} onOpenChange={(o) => !o && setOpenVm(null)}>
                <DialogContent
                    data-testid={VMS.consoleModal}
                    className="glass max-w-5xl border-white/10 bg-black/70 p-4 backdrop-blur-2xl"
                >
                    <DialogHeader className="flex-row items-center justify-between">
                        <div>
                            <DialogTitle className="mono text-sm">
                                {openVm?.name} <span className="text-muted-foreground">· {openVm?.os}</span>
                            </DialogTitle>
                            <div className="mt-1 flex items-center gap-3 text-[11px] text-muted-foreground">
                                <span className="flex items-center gap-1.5">
                                    <span className="status-dot ok animate-pulse" /> VNC connected
                                </span>
                                <span>{openVm?.node}</span>
                            </div>
                        </div>
                        <button
                            data-testid={VMS.consoleClose}
                            onClick={() => setOpenVm(null)}
                            className="rounded-lg border border-white/10 bg-white/[0.03] p-1.5 text-muted-foreground hover:bg-white/[0.08]"
                        >
                            <X className="h-4 w-4" />
                        </button>
                    </DialogHeader>
                    {openVm && <VncCanvas vm={openVm} />}
                </DialogContent>
            </Dialog>
        </div>
    );
}
