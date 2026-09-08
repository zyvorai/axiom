import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Cpu, MemoryStick, Boxes, Server, Ban, CheckCircle2, LogOut } from "lucide-react";
import { NODES } from "@/constants/testIds";
import { toast } from "sonner";

export default function Nodes() {
    const [nodes, setNodes] = useState(null);
    const [busy, setBusy] = useState("");

    const load = () => api.get("/nodes").then((r) => setNodes(r.data));

    useEffect(() => {
        let alive = true;
        const tick = () => api.get("/nodes").then((r) => alive && setNodes(r.data));
        tick();
        const t = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(t);
        };
    }, []);

    const act = async (name, action, path) => {
        setBusy(`${name}:${action}`);
        try {
            await api.post(`/nodes/${encodeURIComponent(name)}/${path}`);
            toast.success(`${name}: ${action} requested`);
            await load();
        } catch (e) {
            toast.error(`${action} failed`, { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy("");
        }
    };

    if (!nodes) {
        return (
            <div data-testid={NODES.root} className="page">
                <div className="grid g3">
                    {[...Array(6)].map((_, i) => (
                        <Skeleton key={i} className="h-64 rounded-2xl bg-muted" />
                    ))}
                </div>
            </div>
        );
    }

    return (
        <div data-testid={NODES.root} className="page">
            <PageHeader
                title="Nodes"
                description="Compute topology, allocatable capacity and kubelet status."
                action={<span style={{ fontSize: 13, color: "var(--ink-3)" }}>{nodes.length} nodes total</span>}
            />

            <div className="grid g3">
                {nodes.map((n) => (
                    <div key={n.name} data-testid={NODES.row(n.name)} className="tile">
                        <div className="tile-h">
                            <Link to={`/nodes/${encodeURIComponent(n.name)}`} className="mono hover:underline" style={{ color: "var(--ink)" }}>
                                <b>{n.name}</b>
                            </Link>
                            <StatusBadge status={n.status} />
                        </div>
                        <div style={{ marginTop: -8, marginBottom: 14, fontSize: 12.5, color: "var(--ink-3)" }}>
                            {n.role} · {n.internalIp}
                        </div>

                        <div className="tile-specs">
                            <div>
                                <Cpu size={13} /> CPU
                                <b>{n.allocatableCpu}</b>
                            </div>
                            <div>
                                <MemoryStick size={13} /> Memory
                                <b>{n.allocatableMemory}</b>
                            </div>
                            <div>
                                <Boxes size={13} /> Pods
                                <b>
                                    {n.pods}
                                    <small>/{n.allocatablePods}</small>
                                </b>
                            </div>
                            <div>
                                <Server size={13} /> Version
                                <b className="truncate" style={{ fontSize: 14 }}>
                                    {n.version}
                                </b>
                            </div>
                        </div>

                        <div
                            style={{
                                display: "grid",
                                gridTemplateColumns: "1fr 1fr",
                                gap: 12,
                                marginBottom: 18,
                                paddingTop: 14,
                                borderTop: "1px solid var(--line)",
                                fontSize: 12,
                                color: "var(--ink-3)",
                            }}
                        >
                            <div>
                                OS
                                <div style={{ marginTop: 2, color: "var(--ink)" }}>{n.osImage}</div>
                            </div>
                            <div>
                                Kernel
                                <div className="mono" style={{ marginTop: 2, color: "var(--ink)" }}>
                                    {n.kernelVersion}
                                </div>
                            </div>
                            <div style={{ gridColumn: "1 / -1" }}>
                                Runtime
                                <div className="mono truncate" style={{ marginTop: 2, color: "var(--ink)" }}>
                                    {n.containerRuntime}
                                </div>
                            </div>
                        </div>

                        <div className="tile-acts">
                            {n.status === "SchedulingDisabled" ? (
                                <button
                                    type="button"
                                    className="btn btn-line btn-sm"
                                    disabled={busy === `${n.name}:uncordon`}
                                    onClick={() => act(n.name, "uncordon", "uncordon")}
                                >
                                    <CheckCircle2 size={13} /> Uncordon
                                </button>
                            ) : (
                                <button
                                    type="button"
                                    className="btn btn-line btn-sm"
                                    disabled={busy === `${n.name}:cordon`}
                                    onClick={() => act(n.name, "cordon", "cordon")}
                                >
                                    <Ban size={13} /> Cordon
                                </button>
                            )}
                            <button
                                type="button"
                                className="btn btn-danger btn-sm"
                                disabled={busy === `${n.name}:drain`}
                                onClick={() => act(n.name, "drain", "drain")}
                            >
                                <LogOut size={13} /> Drain
                            </button>
                        </div>
                    </div>
                ))}
            </div>
        </div>
    );
}
