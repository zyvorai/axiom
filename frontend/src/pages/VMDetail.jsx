import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import DetailHeader from "@/components/DetailHeader";
import SpecTable from "@/components/SpecTable";
import StatusBadge from "@/components/StatusBadge";
import FeatureTile from "@/components/FeatureTile";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import {
    Dialog,
    DialogContent,
    DialogHeader,
    DialogTitle,
} from "@/components/ui/dialog";
import { ArrowLeft, ArrowRightLeft, Check, Copy, Cpu, MemoryStick, Play, Square, Terminal } from "lucide-react";
import { toast } from "sonner";
import { VMS } from "@/constants/testIds";

function ConsolePanel({ vm }) {
    const [info, setInfo] = useState(null);
    const [copied, setCopied] = useState(false);

    useEffect(() => {
        let alive = true;
        api
            .get(`/vms/${vm.namespace}/${encodeURIComponent(vm.name)}/console`)
            .then((r) => alive && setInfo(r.data))
            .catch(() => alive && setInfo(false));
        return () => {
            alive = false;
        };
    }, [vm]);

    if (info === null) return <Skeleton className="h-40 rounded-xl bg-muted" />;
    if (info === false) return <div className="rounded-xl border border-border p-6 text-center text-sm text-muted-foreground">Console info unavailable.</div>;

    const copy = () => {
        navigator.clipboard?.writeText(info.virtctl);
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
    };

    return (
        <div className="space-y-4 rounded-xl border border-border bg-muted p-6">
            <div className="text-sm font-medium">{info.ready ? "VM is ready for console access" : "VM is not ready"}</div>
            <div>
                <div className="mb-1 text-[10px] uppercase tracking-widest text-muted-foreground">Run locally (kubeconfig required)</div>
                <div className="flex items-center gap-2 rounded-lg border border-border bg-background/40 px-3 py-2">
                    <code className="mono flex-1 truncate text-xs">{info.virtctl}</code>
                    <button type="button" onClick={copy} className="text-muted-foreground hover:text-foreground">
                        {copied ? <Check className="h-3.5 w-3.5 text-emerald-400" /> : <Copy className="h-3.5 w-3.5" />}
                    </button>
                </div>
            </div>
            {info.vncPath && (
                <div>
                    <div className="mb-1 text-[10px] uppercase tracking-widest text-muted-foreground">Kubernetes API subresource</div>
                    <code className="mono block truncate text-[11px] text-muted-foreground">{info.vncPath}</code>
                </div>
            )}
        </div>
    );
}

export default function VMDetail() {
    const { namespace, name } = useParams();
    const navigate = useNavigate();
    const ns = decodeURIComponent(namespace || "");
    const nm = decodeURIComponent(name || "");
    const [vm, setVm] = useState(null);
    const [busy, setBusy] = useState("");
    const [consoleOpen, setConsoleOpen] = useState(false);

    const load = () =>
        api
            .get(`/vms?namespace=${encodeURIComponent(ns)}`)
            .then((r) => {
                const found = (r.data || []).find((v) => v.name === nm);
                setVm(found || false);
            })
            .catch(() => setVm(false));

    useEffect(() => {
        let alive = true;
        api
            .get(`/vms?namespace=${encodeURIComponent(ns)}`)
            .then((r) => {
                if (!alive) return;
                const found = (r.data || []).find((v) => v.name === nm);
                setVm(found || false);
            })
            .catch(() => alive && setVm(false));
        return () => {
            alive = false;
        };
    }, [ns, nm]);

    const act = async (action, path) => {
        setBusy(action);
        try {
            await api.post(`/vms/${encodeURIComponent(ns)}/${encodeURIComponent(nm)}/${path}`);
            toast.success(`${nm}: ${action} requested`);
            await load();
        } catch (e) {
            toast.error(`${action} failed`, { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    if (vm === null) {
        return (
            <div className="space-y-4">
                <Skeleton className="h-20 rounded-2xl bg-muted" />
                <Skeleton className="h-64 rounded-2xl bg-muted" />
            </div>
        );
    }

    if (vm === false) {
        return (
            <div className="space-y-4">
                <Button variant="ghost" size="sm" onClick={() => navigate("/vms")} className="gap-1.5">
                    <ArrowLeft className="h-3.5 w-3.5" /> Virtual Machines
                </Button>
                <GlassCard className="p-8 text-center text-sm text-muted-foreground">
                    VM <span className="mono text-foreground">{ns}/{nm}</span> was not found.
                </GlassCard>
            </div>
        );
    }

    const running = vm.running || vm.status === "Running";

    return (
        <div className="space-y-6" data-testid={VMS.root}>
            <DetailHeader
                eyebrow="Virtual Machine"
                name={nm}
                meta={
                    <>
                        <Link to={`/namespaces/${encodeURIComponent(ns)}`} className="mono text-primary hover:underline">
                            {ns}
                        </Link>
                        <span>·</span>
                        <StatusBadge status={vm.status} />
                        <span>·</span>
                        <span>{vm.os || "—"}</span>
                        <span>·</span>
                        <span>{vm.age || "—"}</span>
                    </>
                }
                action={
                    <div className="flex flex-wrap gap-2">
                        {running ? (
                            <Button size="sm" variant="outline" disabled={busy === "stop"} onClick={() => act("stop", "stop")} className="rounded-xl">
                                <Square className="mr-1 h-3.5 w-3.5" /> Stop
                            </Button>
                        ) : (
                            <Button size="sm" variant="outline" disabled={busy === "start"} onClick={() => act("start", "start")} className="rounded-xl">
                                <Play className="mr-1 h-3.5 w-3.5" /> Start
                            </Button>
                        )}
                        <Button
                            size="sm"
                            variant="outline"
                            disabled={!running || busy === "migrate"}
                            onClick={() => act("migrate", "migrate")}
                            className="rounded-xl"
                        >
                            <ArrowRightLeft className="mr-1 h-3.5 w-3.5" /> Migrate
                        </Button>
                        <Button size="sm" onClick={() => setConsoleOpen(true)} className="rounded-xl">
                            <Terminal className="mr-1 h-3.5 w-3.5" /> Console
                        </Button>
                        <Button variant="outline" size="sm" onClick={() => navigate("/vms")} className="rounded-xl gap-1.5">
                            <ArrowLeft className="h-3.5 w-3.5" /> All VMs
                        </Button>
                    </div>
                }
            />

            <div className="grid gap-3 sm:grid-cols-2">
                <FeatureTile nested size="compact" mono icon={Cpu} label="vCPU" value={vm.cpu || "—"} className="p-3" />
                <FeatureTile nested size="compact" mono icon={MemoryStick} label="RAM" value={vm.memory || "—"} className="p-3" />
            </div>

            <SpecTable
                title="Placement"
                rows={[
                    {
                        label: "Node",
                        value: vm.node ? (
                            <Link to={`/nodes/${encodeURIComponent(vm.node)}`} className="text-primary hover:underline">
                                {vm.node}
                            </Link>
                        ) : (
                            "—"
                        ),
                        mono: true,
                    },
                    { label: "Ready", value: vm.ready ? "Yes" : "No" },
                    { label: "Running", value: running ? "Yes" : "No" },
                ]}
            />

            <Dialog open={consoleOpen} onOpenChange={setConsoleOpen}>
                <DialogContent data-testid={VMS.consoleModal} className="glass max-w-lg border-border bg-card p-4 backdrop-blur-2xl">
                    <DialogHeader>
                        <DialogTitle className="mono text-sm">
                            {nm} <span className="text-muted-foreground">· console</span>
                        </DialogTitle>
                    </DialogHeader>
                    <ConsolePanel vm={vm} />
                    <Button size="sm" variant="outline" onClick={() => setConsoleOpen(false)} data-testid={VMS.consoleClose} className="mt-2 rounded-xl">
                        Close
                    </Button>
                </DialogContent>
            </Dialog>
        </div>
    );
}
