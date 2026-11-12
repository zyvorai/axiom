import { useState } from "react";
import { Bot } from "lucide-react";
import { toast } from "sonner";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api";

/**
 * Creates a ProposeOnly agent run for a cluster object (pod / event / finding).
 * target: { namespace, kind, name, goal?, reason? }
 */
export function RunAgentButton({ target, className = "", testId }) {
    const { t } = useTranslation();
    const [busy, setBusy] = useState(false);
    if (!target?.namespace || !target?.name) return null;

    const run = async () => {
        setBusy(true);
        try {
            const kind = target.kind === "event" ? "Pod" : (target.kind || "Pod");
            const name = target.kind === "event" && String(target.name).includes("/")
                ? String(target.name).split("/").pop()
                : target.name;
            await api.post("/agent-runs", {
                namespace: target.namespace,
                goal: target.goal || target.reason || `Diagnose and propose a safe fix for ${kind}/${name}`,
                dryRun: true,
                source: "health",
                target: { apiVersion: "v1", kind: kind === "pod" ? "Pod" : kind, namespace: target.namespace, name },
                extraContext: target.label || target.reason || "",
            });
            toast.success(t("toasts:ops.agents.created"));
        } catch (e) {
            toast.error(t("toasts:ops.agents.createFailed"), { description: e?.response?.data?.error || e.message });
        } finally {
            setBusy(false);
        }
    };

    return (
        <button
            type="button"
            data-testid={testId}
            className={`btn btn-line btn-sm ${className}`}
            disabled={busy}
            onClick={run}
            title={t("pages:ops.agents.runAgent")}
        >
            <Bot size={13} /> {busy ? t("common:actions.saving") : t("pages:ops.agents.runAgent")}
        </button>
    );
}
