import { useEffect, useState } from "react";
import { AlertTriangle, Terminal } from "lucide-react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PodTerminal from "@/components/PodTerminal";
import { useAuth } from "@/lib/auth";
import { NODESHELL } from "@/constants/testIds";

/**
 * Root shell on a node. Off by default: the server only offers it when an
 * administrator set AXIOM_ENABLE_NODE_SHELL=true, and only admins may open it
 * (the server enforces both; this panel just explains the state).
 */
export default function NodeShellPanel({ node }) {
    const { isViewer } = useAuth();
    const [caps, setCaps] = useState(null);
    const [open, setOpen] = useState(false);

    useEffect(() => {
        let alive = true;
        api
            .get("/debug/capabilities")
            .then((r) => alive && setCaps(r.data))
            .catch(() => alive && setCaps({ nodeShell: false }));
        return () => {
            alive = false;
        };
    }, []);

    if (isViewer) return null;

    return (
        <GlassCard className="space-y-3 p-5" data-testid={NODESHELL.panel}>
            <div className="flex items-center gap-2 text-sm font-medium">
                <Terminal className="h-4 w-4" aria-hidden="true" /> Node shell
            </div>
            {!caps && <p className="text-xs text-muted-foreground">Checking…</p>}
            {caps && !caps.nodeShell && (
                <p className="text-xs text-muted-foreground" data-testid={NODESHELL.disabled}>
                    The node shell is disabled on this server. An administrator can enable it by setting <span className="mono">AXIOM_ENABLE_NODE_SHELL=true</span> on the
                    Axiom deployment. It is admin-only and every session is audited.
                </p>
            )}
            {caps?.nodeShell && (
                <>
                    <div className="notice danger" data-testid={NODESHELL.warning}>
                        <AlertTriangle size={16} aria-hidden="true" />
                        <span>
                            This opens a <b>root shell on the node</b> through a short-lived privileged pod in the <span className="mono">{caps.nodeShellNamespace}</span>{" "}
                            namespace (admins only, audited as <span className="mono">node.shell</span>). The pod is deleted when you disconnect or after{" "}
                            {Math.round((caps.nodeShellTimeoutSeconds || 900) / 60)} minutes.
                        </span>
                    </div>
                    {!caps.interactive ? (
                        <p className="text-xs text-muted-foreground">An interactive shell needs a live Kubernetes connection.</p>
                    ) : open ? (
                        <PodTerminal
                            namespace="node"
                            name={node}
                            containers={[]}
                            wsPath={`/api/v1/nodes/${encodeURIComponent(node)}/shell`}
                            label={`root@${node}`}
                        />
                    ) : (
                        <button type="button" className="btn btn-line" onClick={() => setOpen(true)}>
                            Open root shell on {node}
                        </button>
                    )}
                </>
            )}
        </GlassCard>
    );
}
