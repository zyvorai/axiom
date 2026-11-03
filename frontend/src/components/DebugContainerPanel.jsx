import { useState } from "react";
import { Bug } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PodTerminal from "@/components/PodTerminal";
import { useAuth } from "@/lib/auth";
import { DEBUG } from "@/constants/testIds";

/**
 * Adds an ephemeral debug container (POST /pods/{ns}/{name}/debug) to a running
 * pod and, on a live cluster, attaches a terminal to it. Ephemeral containers
 * cannot be removed from a pod; they disappear when the pod is replaced.
 */
export default function DebugContainerPanel({ namespace, name, containers = [], interactive }) {
    const { isViewer } = useAuth();
    const [target, setTarget] = useState("");
    const [busy, setBusy] = useState(false);
    const [created, setCreated] = useState([]);
    const [last, setLast] = useState(null);
    const [error, setError] = useState("");

    if (isViewer) return null;

    const start = async () => {
        setBusy(true);
        setError("");
        try {
            const { data } = await api.post(`/pods/${namespace}/${encodeURIComponent(name)}/debug`, target ? { target } : {});
            setLast(data);
            setCreated((prev) => [...prev, data.container]);
            toast.success(`Debug container ${data.container} added`, { description: data.message });
        } catch (e) {
            const msg = e?.response?.data?.error || e.message;
            setError(msg);
            toast.error("Could not add a debug container", { description: msg });
        } finally {
            setBusy(false);
        }
    };

    return (
        <GlassCard className="space-y-3 p-5" data-testid={DEBUG.panel}>
            <div className="flex items-center gap-2 text-sm font-medium">
                <Bug className="h-4 w-4" aria-hidden="true" /> Debug container
            </div>
            <p className="text-xs text-muted-foreground">
                Adds a throw-away container with debugging tools to this running pod, sharing its network (and, when a target is chosen, that container&apos;s
                processes). It cannot be removed until the pod is replaced. The action is audited as <span className="mono">pod.debug</span>.
            </p>
            <div className="flex flex-wrap items-center gap-2">
                <select
                    data-testid={DEBUG.target}
                    aria-label="Target container"
                    className="select"
                    value={target}
                    onChange={(e) => setTarget(e.target.value)}
                >
                    <option value="">No target process namespace</option>
                    {containers.map((c) => (
                        <option key={c} value={c}>
                            Target {c}
                        </option>
                    ))}
                </select>
                <button type="button" data-testid={DEBUG.start} className="btn btn-fill" onClick={start} disabled={busy}>
                    {busy ? "Adding…" : "Add debug container"}
                </button>
            </div>
            {error && (
                <div className="notice danger" role="alert" data-testid={DEBUG.error}>
                    {error}
                </div>
            )}
            {last && (
                <div data-testid={DEBUG.result} className="space-y-1 text-sm">
                    <div>
                        <span className="mono text-tone-link">{last.container}</span>
                        <span className="text-muted-foreground"> · {last.image}</span>
                        {last.target && <span className="text-muted-foreground"> · target {last.target}</span>}
                    </div>
                    <div className="text-xs text-muted-foreground" data-testid={DEBUG.message}>
                        {last.message}
                    </div>
                </div>
            )}
            {interactive && created.length > 0 && <PodTerminal namespace={namespace} name={name} containers={created} />}
        </GlassCard>
    );
}
