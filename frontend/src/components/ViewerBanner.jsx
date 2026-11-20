import { Eye } from "lucide-react";
import { Trans } from "react-i18next";
import { useAuth } from "@/lib/auth";

export default function ViewerBanner() {
    const { isViewer } = useAuth();
    if (!isViewer) return null;

    return (
        <div className="flex items-center gap-2 rounded-2xl border border-soft-info-border bg-soft-info px-4 py-2.5 text-xs text-muted-foreground">
            <Eye className="h-3.5 w-3.5 shrink-0 text-tone-link" />
            <Trans i18nKey="auth:viewerBanner" components={{ role: <span className="font-medium text-foreground" /> }} />
        </div>
    );
}
