import { useEffect, useState } from "react";
import { AlertTriangle, Terminal } from "lucide-react";
import { api } from "@/lib/api";
import GlassCard from "@/components/GlassCard";
import PodTerminal from "@/components/PodTerminal";
import { useAuth } from "@/lib/auth";
import { NODESHELL } from "@/constants/testIds";
import { useTranslation, Trans } from "react-i18next";

/**
 * Root shell on a node. Off by default: the server only offers it when an
 * administrator set AXIOM_ENABLE_NODE_SHELL=true, and only admins may open it
 * (the server enforces both; this panel just explains the state).
 */
export default function NodeShellPanel({ node }) {
    const { t } = useTranslation();
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
                <Terminal className="h-4 w-4" aria-hidden="true" /> {t("pages:nodeShell.title")}
            </div>
            {!caps && <p className="text-xs text-muted-foreground">{t("pages:nodeShell.checking")}</p>}
            {caps && !caps.nodeShell && (
                <p className="text-xs text-muted-foreground" data-testid={NODESHELL.disabled}>
                    <Trans i18nKey="pages:nodeShell.disabled" components={{ code: <span className="mono" /> }} />
                </p>
            )}
            {caps?.nodeShell && (
                <>
                    <div className="notice danger" data-testid={NODESHELL.warning}>
                        <AlertTriangle size={16} aria-hidden="true" />
                        <span>
                            <Trans
                                i18nKey="pages:nodeShell.warning"
                                values={{ namespace: caps.nodeShellNamespace, minutes: Math.round((caps.nodeShellTimeoutSeconds || 900) / 60) }}
                                components={{ b: <b />, code: <span className="mono" /> }}
                            />
                        </span>
                    </div>
                    {!caps.interactive ? (
                        <p className="text-xs text-muted-foreground">{t("pages:nodeShell.needsLive")}</p>
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
                            {t("pages:nodeShell.open", { node })}
                        </button>
                    )}
                </>
            )}
        </GlassCard>
    );
}
