import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "@/lib/api";
import PageHeader from "@/components/PageHeader";
import StatusBadge from "@/components/StatusBadge";
import { Skeleton } from "@/components/ui/skeleton";
import { Cpu, MemoryStick, Boxes, Server, Ban, CheckCircle2, LogOut } from "lucide-react";
import { NODES } from "@/constants/testIds";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";

// Toast texts per node action (static keys so every string is checked).
function nodeActionRequested(t, action, name) {
    if (action === "cordon") return t("toasts:nodes.cordonRequested", { name });
    if (action === "uncordon") return t("toasts:nodes.uncordonRequested", { name });
    return t("toasts:nodes.drainRequested", { name });
}

function nodeActionFailed(t, action) {
    if (action === "cordon") return t("toasts:nodes.cordonFailed");
    if (action === "uncordon") return t("toasts:nodes.uncordonFailed");
    return t("toasts:nodes.drainFailed");
}

export default function Nodes() {
    const { t } = useTranslation();
    const [nodes, setNodes] = useState(null);
    const [busy, setBusy] = useState("");

    const load = () => api.get("/nodes").then((r) => setNodes(r.data));

    useEffect(() => {
        let alive = true;
        const tick = () => api.get("/nodes").then((r) => alive && setNodes(r.data));
        tick();
        const timer = setInterval(tick, 10000);
        return () => {
            alive = false;
            clearInterval(timer);
        };
    }, []);

    const act = async (name, action, path) => {
        setBusy(`${name}:${action}`);
        try {
            await api.post(`/nodes/${encodeURIComponent(name)}/${path}`);
            toast.success(nodeActionRequested(t, action, name));
            await load();
        } catch (e) {
            toast.error(nodeActionFailed(t, action), { description: e?.response?.data?.error || e.message });
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
                title={t("pages:nodes.title")}
                description={t("pages:nodes.description")}
                action={<span className="text-[13px] text-ink-3">{t("pages:nodes.total", { count: nodes.length })}</span>}
            />

            <div className="grid g3">
                {nodes.map((n) => (
                    <div key={n.name} data-testid={NODES.row(n.name)} className="tile">
                        <div className="tile-h">
                            <Link to={`/nodes/${encodeURIComponent(n.name)}`} className="mono hover:underline text-ink">
                                <b>{n.name}</b>
                            </Link>
                            <StatusBadge status={n.status} />
                        </div>
                        <div className="-mt-2 mb-3.5 text-[12.5px] text-ink-3">
                            {n.role} · {n.internalIp}
                        </div>

                        <div className="tile-specs c2">
                            <div>
                                <Cpu size={13} /> {t("common:fields.cpu")}
                                <b>{n.allocatableCpu}</b>
                            </div>
                            <div>
                                <MemoryStick size={13} /> {t("common:fields.memory")}
                                <b>{n.allocatableMemory}</b>
                            </div>
                            <div>
                                <Boxes size={13} /> {t("pages:nodes.specs.pods")}
                                <b>
                                    {n.pods}
                                    <small>/{n.allocatablePods}</small>
                                </b>
                            </div>
                            <div>
                                <Server size={13} /> {t("common:fields.version")}
                                <b className="truncate text-[14px]">
                                    {n.version}
                                </b>
                            </div>
                        </div>

                        <div className="tile-facts">
                            <div>
                                {t("pages:nodes.facts.os")}
                                <div className="mt-0.5 text-ink">{n.osImage}</div>
                            </div>
                            <div>
                                {t("pages:nodes.facts.kernel")}
                                <div className="mono mt-0.5 text-ink">
                                    {n.kernelVersion}
                                </div>
                            </div>
                            <div className="wide">
                                {t("pages:nodes.facts.runtime")}
                                <div className="mono truncate mt-0.5 text-ink">
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
                                    <CheckCircle2 size={13} /> {t("pages:nodes.actions.uncordon")}
                                </button>
                            ) : (
                                <button
                                    type="button"
                                    className="btn btn-line btn-sm"
                                    disabled={busy === `${n.name}:cordon`}
                                    onClick={() => act(n.name, "cordon", "cordon")}
                                >
                                    <Ban size={13} /> {t("pages:nodes.actions.cordon")}
                                </button>
                            )}
                            <button
                                type="button"
                                className="btn btn-danger btn-sm"
                                disabled={busy === `${n.name}:drain`}
                                onClick={() => act(n.name, "drain", "drain")}
                            >
                                <LogOut size={13} /> {t("pages:nodes.actions.drain")}
                            </button>
                        </div>
                    </div>
                ))}
            </div>
        </div>
    );
}
